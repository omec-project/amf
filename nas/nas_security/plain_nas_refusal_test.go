// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package nas_security_test

import (
	"testing"

	"github.com/omec-project/amf/context"
	"github.com/omec-project/amf/logger"
	"github.com/omec-project/amf/nas/nas_security"
	"github.com/omec-project/nas/v2"
	"github.com/omec-project/nas/v2/nasMessage"
	"github.com/omec-project/openapi/v2/models"
)

// plainServiceRequest is a decodable plain NAS message that TS 24.501 subclause 4.4.4.3 withholds
// until NAS security is up: it is not one of the seven types a UE holding a security context may
// send unprotected. Decodable matters -- a malformed payload would be refused by the decoder
// itself, and the test would pass whatever the guard did.
func plainServiceRequest(t *testing.T) []byte {
	t.Helper()

	m := nas.NewMessage()
	m.GmmMessage = nas.NewGmmMessage()
	m.GmmHeader.SetMessageType(nas.MsgTypeServiceRequest)

	serviceRequest := nasMessage.NewServiceRequest(0)
	serviceRequest.SetExtendedProtocolDiscriminator(nasMessage.Epd5GSMobilityManagementMessage)
	serviceRequest.SetSecurityHeaderType(nas.SecurityHeaderTypePlainNas)
	serviceRequest.SetSpareHalfOctet(0)
	serviceRequest.SetMessageType(nas.MsgTypeServiceRequest)
	serviceRequest.SetServiceTypeValue(nasMessage.ServiceTypeMobileTerminatedServices)
	m.ServiceRequest = serviceRequest

	payload, err := m.PlainNasEncode()
	if err != nil {
		t.Fatalf("encoding the plain service request: %v", err)
	}

	return payload
}

// ueHoldingASecurityContext is the state this guard is about: NAS security is up, so plain NAS is
// restricted. Built field by field rather than through NewAmfUe, which allocates a GUTI and needs
// a configured PLMN this package does not have.
func ueHoldingASecurityContext() *context.AmfUe {
	return &context.AmfUe{
		NASLog:                   logger.NasLog,
		RanUe:                    make(map[models.AccessType]*context.RanUe),
		SecurityContextAvailable: true,
	}
}

// A UE that holds a security context may send plain NAS only for an emergency connection, and the
// RRC establishment cause on its RAN association is how the AMF knows. When the association has
// been released the cause cannot be read at all -- so the message cannot be placed, and it is
// refused.
//
// Reading the missing association as an emergency instead accepted every message type from a UE
// with a security context, which is what the restriction exists to prevent: a Service Request,
// unauthenticated and unprotected, would have been handed to the 5GMM entity. A release that has
// already happened is not evidence of an emergency.
func TestPlainNasIsRefusedWhenTheAssociationIsGone(t *testing.T) {
	ue := ueHoldingASecurityContext()

	msg, err := nas_security.Decode(ue, models.ACCESSTYPE__3_GPP_ACCESS, plainServiceRequest(t))
	if err == nil {
		t.Fatalf("Decode() accepted plain NAS from a UE with a security context and no RAN association, returning %v", msg)
	}

	if msg != nil {
		t.Errorf("Decode() = %v, want no message alongside the refusal", msg)
	}

	// The non-emergency branch clears the context on a plain NAS message. Reaching it on a message
	// this cannot place would let one unauthenticated packet cost a UE its security context.
	if !ue.SecurityContextAvailable {
		t.Error("the refused message cleared the UE's security context")
	}
}

// The same message from a UE with an association that is not an emergency connection is refused
// too, but by the restriction itself rather than by the missing association -- so the guard above
// is not standing in for a check that was already there.
func TestPlainNasFromANonEmergencyConnectionIsStillRefused(t *testing.T) {
	ue := ueHoldingASecurityContext()
	// Attached directly: AttachRanUe wants a RAN and the logger plumbing that goes with a real
	// association, and what this case needs is only an establishment cause to read.
	ue.RanUe[models.ACCESSTYPE__3_GPP_ACCESS] = &context.RanUe{RRCEstablishmentCause: "1"}

	if _, err := nas_security.Decode(ue, models.ACCESSTYPE__3_GPP_ACCESS, plainServiceRequest(t)); err == nil {
		t.Error("Decode() accepted an unprotected service request from a non-emergency connection")
	}
}
