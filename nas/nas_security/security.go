// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

package nas_security

import (
	"encoding/hex"
	"fmt"
	"reflect"
	"sync"

	"github.com/omec-project/amf/context"
	"github.com/omec-project/amf/logger"
	"github.com/omec-project/nas"
	"github.com/omec-project/nas/nasConvert"
	"github.com/omec-project/nas/nasMessage"
	"github.com/omec-project/nas/security"
	"github.com/omec-project/openapi/models"
)

var mutex sync.Mutex

func Encode(ue *context.AmfUe, msg *nas.Message) ([]byte, error) {
	if ue == nil {
		return nil, fmt.Errorf("amfUe is nil")
	}
	if msg == nil {
		return nil, fmt.Errorf("Nas Message is empty")
	}

	// Plain NAS message
	if !ue.SecurityContextAvailable {
		return msg.PlainNasEncode()
	} else {
		// Security protected NAS Message
		// a security protected NAS message must be integrity protected, and ciphering is optional
		needCiphering := false
		switch msg.SecurityHeader.SecurityHeaderType {
		case nas.SecurityHeaderTypeIntegrityProtected:
			ue.NASLog.Debugln("Security header type: Integrity Protected")
		case nas.SecurityHeaderTypeIntegrityProtectedAndCiphered:
			ue.NASLog.Debugln("Security header type: Integrity Protected And Ciphered")
			needCiphering = true
		case nas.SecurityHeaderTypeIntegrityProtectedWithNew5gNasSecurityContext:
			ue.NASLog.Debugln("Security header type: Integrity Protected With New 5G Security Context")
			ue.ULCount.Set(0, 0)
			ue.DLCount.Set(0, 0)
		default:
			return nil, fmt.Errorf("Wrong security header type: 0x%0x", msg.SecurityHeader.SecurityHeaderType)
		}

		// encode plain nas first
		payload, err := msg.PlainNasEncode()
		if err != nil {
			return nil, fmt.Errorf("Plain NAS encode error: %+v", err)
		}

		ue.NASLog.Tracef("plain payload:\n%+v", hex.Dump(payload))

		if needCiphering {
			ue.NASLog.Debugf("Encrypt NAS message (algorithm: %+v, DLCount: 0x%0x)", ue.CipheringAlg, ue.DLCount.Get())
			ue.NASLog.Tracef("NAS ciphering key: %0x", ue.KnasEnc)
			if err = security.NASEncrypt(ue.CipheringAlg, ue.KnasEnc, ue.DLCount.Get(), security.Bearer3GPP,
				security.DirectionDownlink, payload); err != nil {
				return nil, fmt.Errorf("Encrypt error: %+v", err)
			}
		}

		// add sequece number
		payload = append([]byte{ue.DLCount.SQN()}, payload[:]...)

		ue.NASLog.Debugf("Calculate NAS MAC (algorithm: %+v, DLCount: 0x%0x)", ue.IntegrityAlg, ue.DLCount.Get())
		ue.NASLog.Debugf("NAS integrity key: %0x", ue.KnasInt)
		mutex.Lock()
		defer mutex.Unlock()
		mac32, err := security.NASMacCalculate(ue.IntegrityAlg, ue.KnasInt, ue.DLCount.Get(), security.Bearer3GPP,
			security.DirectionDownlink, payload)
		if err != nil {
			return nil, fmt.Errorf("MAC calcuate error: %+v", err)
		}
		// Add mac value
		ue.NASLog.Tracef("MAC: 0x%08x", mac32)
		payload = append(mac32, payload[:]...)

		// Add EPD and Security Type
		msgSecurityHeader := []byte{msg.SecurityHeader.ProtocolDiscriminator, msg.SecurityHeader.SecurityHeaderType}
		payload = append(msgSecurityHeader, payload[:]...)

		// Increase DL Count
		ue.DLCount.AddOne()
		return payload, nil
	}
}

func StmsiToGuti(buf [7]byte) (guti string) {
	amfSelf := context.AMF_Self()
	servedGuami := amfSelf.ServedGuamiList[0]

	tmpReginID := servedGuami.AmfId[:2]
	amfID := hex.EncodeToString(buf[1:3])
	tmsi5G := hex.EncodeToString(buf[3:])

	guti = servedGuami.PlmnId.Mcc + servedGuami.PlmnId.Mnc + tmpReginID + amfID + tmsi5G

	return
}

/*
fetch Guti if present incase of integrity protected Nas Message
*/
func FetchUeContextWithMobileIdentity(payload []byte) *context.AmfUe {
	if payload == nil {
		return nil
	}

	msg := new(nas.Message)
	msg.SecurityHeaderType = nas.GetSecurityHeaderType(payload) & 0x0f
	logger.CommLog.Debugf("securityHeaderType is %v", msg.SecurityHeaderType)
	switch msg.SecurityHeaderType {
	case nas.SecurityHeaderTypeIntegrityProtected:
		logger.CommLog.Infof("Security header type: Integrity Protected")
		p := payload[7:]
		if err := msg.PlainNasDecode(&p); err != nil {
			return nil
		}
	case nas.SecurityHeaderTypePlainNas:
		logger.CommLog.Infof("Security header type: PlainNas Message")
		if err := msg.PlainNasDecode(&payload); err != nil {
			return nil
		}
	default:
		logger.CommLog.Infof("Security header type is not plain or integrity protected")
		return nil
	}
	var ue *context.AmfUe = nil
	var guti string
	if msg.GmmHeader.GetMessageType() == nas.MsgTypeRegistrationRequest {
		mobileIdentity5GSContents := msg.RegistrationRequest.MobileIdentity5GS.GetMobileIdentity5GSContents()
		if nasMessage.MobileIdentity5GSType5gGuti == nasConvert.GetTypeOfIdentity(mobileIdentity5GSContents[0]) {
			_, guti = nasConvert.GutiToString(mobileIdentity5GSContents)
			logger.CommLog.Debugf("Guti received in Registraion Request Message: %v", guti)
		} else if nasMessage.MobileIdentity5GSTypeSuci == nasConvert.GetTypeOfIdentity(mobileIdentity5GSContents[0]) {
			suci, _ := nasConvert.SuciToString(mobileIdentity5GSContents)
			/* UeContext found based on SUCI which means context is exist in Network(AMF) but not
			   present in UE. Hence, AMF clear the existing context
			*/
			ue, _ = context.AMF_Self().AmfUeFindBySuci(suci)
			if ue != nil {
				ue.NASLog.Infof("UE Context derived from Suci: %v", suci)
				ue.SecurityContextAvailable = false
			}
			return ue
		}
	} else if msg.GmmHeader.GetMessageType() == nas.MsgTypeServiceRequest {
		mobileIdentity5GSContents := msg.ServiceRequest.TMSI5GS.Octet
		if nasMessage.MobileIdentity5GSType5gSTmsi == nasConvert.GetTypeOfIdentity(mobileIdentity5GSContents[0]) {
			guti = StmsiToGuti(mobileIdentity5GSContents)
			logger.CommLog.Debugf("Guti derived from Service Request Message: %v", guti)
		}
	} else if msg.GmmHeader.GetMessageType() == nas.MsgTypeDeregistrationRequestUEOriginatingDeregistration {
		mobileIdentity5GSContents := msg.DeregistrationRequestUEOriginatingDeregistration.MobileIdentity5GS.GetMobileIdentity5GSContents()
		if nasMessage.MobileIdentity5GSType5gGuti == nasConvert.GetTypeOfIdentity(mobileIdentity5GSContents[0]) {
			_, guti = nasConvert.GutiToString(mobileIdentity5GSContents)
			logger.CommLog.Debugf("Guti received in Deregistraion Request Message: %v", guti)
		}
	}
	if guti != "" {
		ue, _ = context.AMF_Self().AmfUeFindByGuti(guti)
		if ue != nil {
			if msg.SecurityHeaderType == nas.SecurityHeaderTypePlainNas {
				ue.NASLog.Infof("UE Context derived from Guti but received in plain nas: %v", guti)
				return nil
			}
			ue.NASLog.Infof("UE Context derived from Guti: %v", guti)
			return ue
		} else {
			logger.CommLog.Warnf("UE Context not fround from Guti: %v", guti)
		}
	}

	return nil
}

/*
payload either a security protected 5GS NAS message or a plain 5GS NAS message which
format is followed TS 24.501 9.1.1
*/
func Decode(ue *context.AmfUe, accessType models.AccessType, payload []byte) (*nas.Message, error) {
	if ue == nil {
		return nil, fmt.Errorf("amfUe is nil")
	}
	if payload == nil {
		return nil, fmt.Errorf("Nas payload is empty")
	}

	msg := new(nas.Message)
	msg.SecurityHeaderType = nas.GetSecurityHeaderType(payload) & 0x0f
	ue.NASLog.Traceln("securityHeaderType is ", msg.SecurityHeaderType)
	if msg.SecurityHeaderType == nas.SecurityHeaderTypePlainNas {
		// RRCEstablishmentCause 0 is for emergency service
		if ue.SecurityContextAvailable && ue.RanUe[accessType].RRCEstablishmentCause != "0" {
			ue.NASLog.Warnln("Received Plain NAS message")
			ue.MacFailed = false
			if err := msg.PlainNasDecode(&payload); err != nil {
				return nil, err
			}

			if msg.GmmMessage == nil {
				return nil, fmt.Errorf("Gmm Message is nil")
			}

			// TS 24.501 4.4.4.3: Except the messages listed below, no NAS signalling messages shall be processed
			// by the receiving 5GMM entity in the AMF or forwarded to the 5GSM entity, unless the secure exchange
			// of NAS messages has been established for the NAS signalling connection
			switch msg.GmmHeader.GetMessageType() {
			case nas.MsgTypeRegistrationRequest:
				return msg, nil
			case nas.MsgTypeIdentityResponse:
				return msg, nil
			case nas.MsgTypeAuthenticationResponse:
				return msg, nil
			case nas.MsgTypeAuthenticationFailure:
				return msg, nil
			case nas.MsgTypeSecurityModeReject:
				return msg, nil
			case nas.MsgTypeDeregistrationRequestUEOriginatingDeregistration:
				return msg, nil
			case nas.MsgTypeDeregistrationAcceptUETerminatedDeregistration:
				return msg, nil
			default:
				return nil, fmt.Errorf(
					"UE can not send plain nas for non-emergency service when there is a valid security context")
			}
		} else {
			ue.MacFailed = false
			err := msg.PlainNasDecode(&payload)
			return msg, err
		}
	} else { // Security protected NAS message
		securityHeader := payload[0:6]
		ue.NASLog.Traceln("securityHeader is ", securityHeader)
		sequenceNumber := payload[6]
		ue.NASLog.Traceln("sequenceNumber", sequenceNumber)

		receivedMac32 := securityHeader[2:]
		// remove security Header except for sequece Number
		payload = payload[6:]

		// a security protected NAS message must be integrity protected, and ciphering is optional
		ciphered := false
		switch msg.SecurityHeaderType {
		case nas.SecurityHeaderTypeIntegrityProtected:
			ue.NASLog.Debugln("Security header type: Integrity Protected")
		case nas.SecurityHeaderTypeIntegrityProtectedAndCiphered:
			ue.NASLog.Debugln("Security header type: Integrity Protected And Ciphered")
			ciphered = true
		case nas.SecurityHeaderTypeIntegrityProtectedAndCipheredWithNew5gNasSecurityContext:
			ue.NASLog.Debugln("Security header type: Integrity Protected And Ciphered With New 5G Security Context")
			ciphered = true
			ue.ULCount.Set(0, 0)
		default:
			return nil, fmt.Errorf("Wrong security header type: 0x%0x", msg.SecurityHeader.SecurityHeaderType)
		}

		if ue.ULCount.SQN() > sequenceNumber {
			ue.NASLog.Debugf("set ULCount overflow")
			ue.ULCount.SetOverflow(ue.ULCount.Overflow() + 1)
		}
		ue.ULCount.SetSQN(sequenceNumber)

		ue.NASLog.Debugf("Calculate NAS MAC (algorithm: %+v, ULCount: 0x%0x)", ue.IntegrityAlg, ue.ULCount.Get())
		ue.NASLog.Debugf("NAS integrity key0x: %0x", ue.KnasInt)
		mutex.Lock()
		defer mutex.Unlock()
		mac32, err := security.NASMacCalculate(ue.IntegrityAlg, ue.KnasInt, ue.ULCount.Get(), security.Bearer3GPP,
			security.DirectionUplink, payload)
		if err != nil {
			return nil, fmt.Errorf("MAC calcuate error: %+v", err)
		}

		if !reflect.DeepEqual(mac32, receivedMac32) {
			ue.NASLog.Warnf("NAS MAC verification failed(received: 0x%08x, expected: 0x%08x)", receivedMac32, mac32)
			ue.MacFailed = true
		} else {
			ue.NASLog.Tracef("cmac value: 0x%08x", mac32)
			ue.MacFailed = false
		}

		if ciphered {
			ue.NASLog.Debugf("Decrypt NAS message (algorithm: %+v, ULCount: 0x%0x)", ue.CipheringAlg, ue.ULCount.Get())
			ue.NASLog.Tracef("NAS ciphering key: %0x", ue.KnasEnc)
			// decrypt payload without sequence number (payload[1])
			if err = security.NASEncrypt(ue.CipheringAlg, ue.KnasEnc, ue.ULCount.Get(), security.Bearer3GPP,
				security.DirectionUplink, payload[1:]); err != nil {
				return nil, fmt.Errorf("Encrypt error: %+v", err)
			}
		}

		// remove sequece Number
		payload = payload[1:]
		err = msg.PlainNasDecode(&payload)

		/*
			integrity check failed, as per spec 24501 section 4.4.4.3 AMF shouldnt process or forward to SMF
			except below message types
		*/
		if err == nil && ue.MacFailed {
			switch msg.GmmHeader.GetMessageType() {
			case nas.MsgTypeRegistrationRequest:
				return msg, nil
			case nas.MsgTypeIdentityResponse:
				return msg, nil
			case nas.MsgTypeAuthenticationResponse:
				return msg, nil
			case nas.MsgTypeAuthenticationFailure:
				return msg, nil
			case nas.MsgTypeSecurityModeReject:
				return msg, nil
			case nas.MsgTypeServiceRequest:
				return msg, nil
			case nas.MsgTypeDeregistrationRequestUEOriginatingDeregistration:
				return msg, nil
			case nas.MsgTypeDeregistrationAcceptUETerminatedDeregistration:
				return msg, nil
			default:
				return nil, fmt.Errorf("Mac Verification for the nas message [%v] failed", msg.GmmHeader.GetMessageType())
			}
		}

		return msg, err
	}
}
