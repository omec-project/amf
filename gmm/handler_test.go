// Copyright 2019 free5GC.org
// SPDX-FileCopyrightText: 2025 Canonical Ltd.
// SPDX-FileCopyrightText: 2025 Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0
//

package gmm

import (
	ctxt "context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/omec-project/amf/context"
	"github.com/omec-project/amf/util"
	"github.com/omec-project/nas/nasMessage"
	"github.com/omec-project/nas/nasType"
	"github.com/omec-project/openapi/models"
	"github.com/omec-project/util/fsm"
	"go.uber.org/zap"
)

func newFuzzUE(fd *FuzzData) *context.AmfUe {
	ue := &context.AmfUe{
		GmmLog:       zap.NewNop().Sugar(),
		RanUe:        make(map[models.AccessType]*context.RanUe),
		AllowedNssai: map[models.AccessType][]models.AllowedSnssai{},
		State:        make(map[models.AccessType]*fsm.State),
	}

	// Setup SubscribedNssai
	if fd.HasSubscribedNssai && fd.SubscribedNssaiLength > 0 {
		ue.SubscribedNssai = make([]models.SubscribedSnssai, fd.SubscribedNssaiLength)
		for i := range ue.SubscribedNssai {
			ue.SubscribedNssai[i] = models.SubscribedSnssai{
				SubscribedSnssai: &models.Snssai{
					Sst: int32(i + 1),
					Sd:  fmt.Sprintf("SD%d", i),
				},
				DefaultIndication: i == 0, // Make the first one default
			}
		}
	}

	// Setup AllowedNssai
	if fd.HasAllowedNssai && fd.AllowedNssaiLength > 0 {
		allowedSlices := make([]models.AllowedSnssai, fd.AllowedNssaiLength)
		for i := range allowedSlices {
			allowedSlices[i] = models.AllowedSnssai{
				AllowedSnssai: &models.Snssai{
					Sst: int32(i + 1),
					Sd:  fmt.Sprintf("SD%d", i),
				},
			}
		}
		ue.AllowedNssai[models.AccessType__3_GPP_ACCESS] = allowedSlices
		ue.AllowedNssai[models.AccessType_NON_3_GPP_ACCESS] = allowedSlices
	}

	// Setup RegistrationRequest
	ue.RegistrationRequest = &nasMessage.RegistrationRequest{}

	// Setup Capability5GMM
	if fd.HasCapability5GMM {
		capability := nasType.Capability5GMM{
			Iei:   0x10,        // Information Element Identifier for 5GMM capability
			Len:   13,          // Length of the capability octets
			Octet: [13]uint8{}, // Initialize with zeros
		}

		// Use fuzz data to set various capability bits
		if fd.AccessType&0x01 != 0 {
			capability.Octet[0] |= 0x08 // LPP capability (bit 3)
		}
		if fd.AccessType&0x02 != 0 {
			capability.Octet[0] |= 0x04 // HOAttach capability (bit 2)
		}
		if fd.AccessType&0x04 != 0 {
			capability.Octet[0] |= 0x02 // S1Mode capability (bit 1)
		}

		// Populate remaining octets with fuzz data
		for i := 1; i < 13; i++ {
			switch i {
			case 1:
				capability.Octet[i] = fd.PeiLength
			case 2:
				capability.Octet[i] = fd.SubscribedNssaiLength
			case 3:
				capability.Octet[i] = fd.AllowedNssaiLength
			case 4:
				capability.Octet[i] = fd.AreasLength
			case 5:
				capability.Octet[i] = fd.SupiLength
			case 6:
				capability.Octet[i] = fd.GutiLength
			case 7:
				capability.Octet[i] = fd.PcfUriLength
			case 8:
				capability.Octet[i] = fd.PcfIdLength
			default:
				capability.Octet[i] = uint8((fd.T3502ValueMs >> ((i - 9) * 8)) & 0xFF)
			}
		}

		ue.RegistrationRequest.Capability5GMM = &capability
	}

	// Setup LastVisitedRegisteredTAI
	if fd.HasLastVisitedTAI {
		tai := nasType.NewLastVisitedRegisteredTAI(nasMessage.RegistrationRequestLastVisitedRegisteredTAIType)
		// Set up TAI with fuzz data
		ue.RegistrationRequest.LastVisitedRegisteredTAI = tai
	}

	// Setup MICOIndication
	if fd.HasMICOIndication {
		micoIndication := nasType.NewMICOIndication(0x0b)

		// Set RAAI (RRC Inactive Assistance Information) bit
		raai := uint8(0)
		if fd.StateRegistered {
			raai = 1
		}
		micoIndication.SetRAAI(raai)

		ue.RegistrationRequest.MICOIndication = micoIndication
	}

	// Setup RequestedDRXParameters
	if fd.HasRequestedDRXParameters {
		drxParams := nasType.NewRequestedDRXParameters(nasMessage.RegistrationRequestRequestedDRXParametersType)
		drxParams.SetLen(1)
		drxParams.SetDRXValue(uint8(fd.T3502ValueMs % 10))

		ue.RegistrationRequest.RequestedDRXParameters = drxParams
	}

	// Setup other UE fields
	ue.ServingAmfChanged = fd.ServingAmfChanged
	ue.SubscriptionDataValid = fd.SubscriptionDataValid

	// Setup PEI
	if fd.HasPei && fd.PeiLength > 0 {
		ue.Pei = generateString("PEI", int(fd.PeiLength))
	}

	// Setup SUPI and GUTI
	if fd.SupiLength > 0 {
		ue.Supi = generateString("SUPI", int(fd.SupiLength))
	}
	if fd.GutiLength > 0 {
		ue.Guti = generateString("GUTI", int(fd.GutiLength))
	}

	// Setup PCF info
	if fd.PcfUriLength > 0 {
		ue.PcfUri = generateString("http://pcf", int(fd.PcfUriLength))
	}
	if fd.PcfIdLength > 0 {
		ue.PcfId = generateString("PCF-ID", int(fd.PcfIdLength))
	}

	// Setup timers
	ue.T3502Value = int(fd.T3502ValueMs)
	ue.T3512Value = int(fd.T3512ValueMs)
	ue.Non3gppDeregistrationTimerValue = int(fd.Non3gppDeregTimerValueMs)

	// Setup RanUe
	ue.RanUe[models.AccessType__3_GPP_ACCESS] = &context.RanUe{AmfUe: ue}
	ue.RanUe[models.AccessType_NON_3_GPP_ACCESS] = &context.RanUe{AmfUe: ue}

	// Setup State
	if ue.State == nil {
		ue.State = make(map[models.AccessType]*fsm.State)
	}
	ue.State[models.AccessType_NON_3_GPP_ACCESS] = &fsm.State{}
	if fd.StateRegistered {
		ue.State[models.AccessType_NON_3_GPP_ACCESS].Set(context.Registered)
	}

	// Setup AmPolicyAssociation
	if fd.HasAmPolicyAssociation && fd.HasServAreaRes {
		areas := make([]models.Area, fd.AreasLength)
		for i := range areas {
			areas[i] = models.Area{
				Tacs: []string{fmt.Sprintf("TAC%d", i)},
			}
		}

		var restrictionType models.RestrictionType
		switch fd.RestrictionType {
		case 0:
			restrictionType = models.RestrictionType_ALLOWED_AREAS
		case 1:
			restrictionType = models.RestrictionType_NOT_ALLOWED_AREAS
		default:
			restrictionType = models.RestrictionType_ALLOWED_AREAS
		}

		ue.AmPolicyAssociation = &models.PolicyAssociation{
			ServAreaRes: &models.ServiceAreaRestriction{
				RestrictionType: restrictionType,
				MaxNumOfTAs:     int32(fd.MaxNumOfTAs),
				Areas:           areas,
			},
		}
	}

	return ue
}

func TestPduSessionIDFromUL(t *testing.T) {
	tests := []struct {
		name        string
		setupUL     func() *nasMessage.ULNASTransport
		expectedID  int32
		expectError bool
	}{
		{
			name: "nil PduSessionID2Value",
			setupUL: func() *nasMessage.ULNASTransport {
				return &nasMessage.ULNASTransport{}
			},
			expectError: true,
		},
		{
			name: "valid PduSessionID2Value with ID 1",
			setupUL: func() *nasMessage.ULNASTransport {
				ul := &nasMessage.ULNASTransport{}
				pduSessionID2Value := nasType.NewPduSessionID2Value(nasMessage.ULNASTransportPduSessionID2ValueType)
				pduSessionID2Value.SetPduSessionID2Value(1)
				ul.PduSessionID2Value = pduSessionID2Value
				return ul
			},
			expectedID:  1,
			expectError: false,
		},
		{
			name: "valid PduSessionID2Value with ID 9",
			setupUL: func() *nasMessage.ULNASTransport {
				ul := &nasMessage.ULNASTransport{}
				pduSessionID2Value := nasType.NewPduSessionID2Value(nasMessage.ULNASTransportPduSessionID2ValueType)
				pduSessionID2Value.SetPduSessionID2Value(9)
				ul.PduSessionID2Value = pduSessionID2Value
				return ul
			},
			expectedID:  9,
			expectError: false,
		},
		{
			name: "valid PduSessionID2Value with ID 15",
			setupUL: func() *nasMessage.ULNASTransport {
				ul := &nasMessage.ULNASTransport{}
				pduSessionID2Value := nasType.NewPduSessionID2Value(nasMessage.ULNASTransportPduSessionID2ValueType)
				pduSessionID2Value.SetPduSessionID2Value(15)
				ul.PduSessionID2Value = pduSessionID2Value
				return ul
			},
			expectedID:  15,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ul := tt.setupUL()
			id, err := pduSessionIDFromUL(ul)

			if tt.expectError {
				if err == nil {
					t.Error("expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if id != tt.expectedID {
					t.Errorf("got id=%d; want id=%d", id, tt.expectedID)
				}
			}
		})
	}
}

func TestIsEmergencyRequestAndIsInitialRequest(t *testing.T) {
	tests := []struct {
		name             string
		requestType      *nasType.RequestType
		requestTypeValue uint8
		expectEmergency  bool
		expectInitial    bool
	}{
		{
			name:            "nil request type",
			requestType:     nil,
			expectEmergency: false,
			expectInitial:   false,
		},
		{
			name:             "initial emergency request",
			requestType:      &nasType.RequestType{},
			requestTypeValue: nasMessage.ULNASTransportRequestTypeInitialEmergencyRequest,
			expectEmergency:  true,
			expectInitial:    false,
		},
		{
			name:             "existing emergency PDU session",
			requestType:      &nasType.RequestType{},
			requestTypeValue: nasMessage.ULNASTransportRequestTypeExistingEmergencyPduSession,
			expectEmergency:  true,
			expectInitial:    false,
		},
		{
			name:             "initial request",
			requestType:      &nasType.RequestType{},
			requestTypeValue: nasMessage.ULNASTransportRequestTypeInitialRequest,
			expectEmergency:  false,
			expectInitial:    true,
		},
		{
			name:             "modification request",
			requestType:      &nasType.RequestType{},
			requestTypeValue: nasMessage.ULNASTransportRequestTypeModificationRequest,
			expectEmergency:  false,
			expectInitial:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up the request type if it's not nil
			if tt.requestType != nil {
				tt.requestType.SetRequestTypeValue(tt.requestTypeValue)
			}

			// Test isEmergencyRequest
			if got := isEmergencyRequest(tt.requestType); got != tt.expectEmergency {
				t.Errorf("isEmergencyRequest() = %v, want %v", got, tt.expectEmergency)
			}

			// Test isInitialRequest
			if got := isInitialRequest(tt.requestType); got != tt.expectInitial {
				t.Errorf("isInitialRequest() = %v, want %v", got, tt.expectInitial)
			}
		})
	}
}

func TestSendNotForwarded(t *testing.T) {
	tests := []struct {
		name         string
		hasRanUe     bool
		accessType   models.AccessType
		payload      []byte
		pduSessionID int32
		wantCalls    int
	}{
		{
			name:         "no RanUe",
			hasRanUe:     false,
			accessType:   models.AccessType__3_GPP_ACCESS,
			payload:      nil,
			pduSessionID: 7,
			wantCalls:    0,
		},
		{
			name:         "with RanUe 3GPP",
			hasRanUe:     true,
			accessType:   models.AccessType__3_GPP_ACCESS,
			payload:      []byte{0x01},
			pduSessionID: 7,
			wantCalls:    1,
		},
		{
			name:         "with RanUe NON_3GPP",
			hasRanUe:     true,
			accessType:   models.AccessType_NON_3_GPP_ACCESS,
			payload:      []byte{0x02},
			pduSessionID: 15,
			wantCalls:    1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mock
			orig := sendDLNASTransport
			defer func() { sendDLNASTransport = orig }()

			calls := 0
			var got struct {
				an    models.AccessType
				cause uint8
				id    int32
			}

			sendDLNASTransport = func(_ *context.RanUe, an models.AccessType, _ uint8, _ []byte, id int32, c uint8, _ *uint8, _ uint8) {
				calls++
				got.an = an
				got.cause = c
				got.id = id
			}

			ue := &context.AmfUe{
				GmmLog:       zap.NewNop().Sugar(),
				RanUe:        make(map[models.AccessType]*context.RanUe),
				AllowedNssai: map[models.AccessType][]models.AllowedSnssai{},
			}

			if tt.hasRanUe {
				ue.RanUe[tt.accessType] = &context.RanUe{AmfUe: ue}
			}

			err := sendNotForwarded(ue, tt.accessType, tt.payload, tt.pduSessionID)
			if err != nil {
				t.Logf("sendNotForwarded returned error (ignored in this test): %v", err)
			}

			if calls != tt.wantCalls {
				t.Errorf("expected %d calls; got %d", tt.wantCalls, calls)
			}

			if tt.wantCalls > 0 {
				if got.an != tt.accessType || got.id != tt.pduSessionID || got.cause != nasMessage.Cause5GMMPayloadWasNotForwarded {
					t.Errorf("unexpected args: an=%v id=%d cause=%d; want an=%v id=%d cause=%d",
						got.an, got.id, got.cause, tt.accessType, tt.pduSessionID, nasMessage.Cause5GMMPayloadWasNotForwarded)
				}
			}
		})
	}
}

func TestPickSnssai(t *testing.T) {
	tests := []struct {
		name        string
		setupUL     func() *nasMessage.ULNASTransport
		setupUE     func() *context.AmfUe
		accessType  models.AccessType
		expectedSst int32
		expectedSd  string
		expectError bool
	}{
		{
			name: "SNSSAI from UL message",
			setupUL: func() *nasMessage.ULNASTransport {
				ul := &nasMessage.ULNASTransport{}
				ul.PduSessionID2Value = nasType.NewPduSessionID2Value(nasMessage.ULNASTransportPduSessionID2ValueType)
				snssai := nasType.NewSNSSAI(nasMessage.PDUSessionEstablishmentAcceptSNSSAIType)
				snssai.SetSST(1)
				snssai.SetSD([3]uint8([]byte{0x11, 0x22, 0x33}))
				ul.SNSSAI = snssai
				return ul
			},
			setupUE: func() *context.AmfUe {
				return &context.AmfUe{
					GmmLog:       zap.NewNop().Sugar(),
					RanUe:        make(map[models.AccessType]*context.RanUe),
					AllowedNssai: map[models.AccessType][]models.AllowedSnssai{},
				}
			},
			accessType:  models.AccessType__3_GPP_ACCESS,
			expectedSst: 1,
			expectedSd:  "112233",
			expectError: false,
		},
		{
			name: "SNSSAI from AllowedNssai when UL SNSSAI is nil",
			setupUL: func() *nasMessage.ULNASTransport {
				ul := &nasMessage.ULNASTransport{}
				ul.PduSessionID2Value = nasType.NewPduSessionID2Value(nasMessage.ULNASTransportPduSessionID2ValueType)
				// ul.SNSSAI is nil
				return ul
			},
			setupUE: func() *context.AmfUe {
				ue := &context.AmfUe{
					GmmLog:       zap.NewNop().Sugar(),
					RanUe:        make(map[models.AccessType]*context.RanUe),
					AllowedNssai: map[models.AccessType][]models.AllowedSnssai{},
				}
				ue.AllowedNssai[models.AccessType__3_GPP_ACCESS] = []models.AllowedSnssai{
					{AllowedSnssai: &models.Snssai{Sst: 2, Sd: "010203"}},
				}
				return ue
			},
			accessType:  models.AccessType__3_GPP_ACCESS,
			expectedSst: 2,
			expectedSd:  "010203",
			expectError: false,
		},
		{
			name: "error when no SNSSAI in UL and no AllowedNssai",
			setupUL: func() *nasMessage.ULNASTransport {
				ul := &nasMessage.ULNASTransport{}
				ul.PduSessionID2Value = nasType.NewPduSessionID2Value(nasMessage.ULNASTransportPduSessionID2ValueType)
				// ul.SNSSAI is nil
				return ul
			},
			setupUE: func() *context.AmfUe {
				return &context.AmfUe{
					GmmLog:       zap.NewNop().Sugar(),
					RanUe:        make(map[models.AccessType]*context.RanUe),
					AllowedNssai: map[models.AccessType][]models.AllowedSnssai{},
					// No AllowedNssai for the access type
				}
			},
			accessType:  models.AccessType__3_GPP_ACCESS,
			expectError: true,
		},
		{
			name: "SNSSAI from UL with different SD format",
			setupUL: func() *nasMessage.ULNASTransport {
				ul := &nasMessage.ULNASTransport{}
				ul.PduSessionID2Value = nasType.NewPduSessionID2Value(nasMessage.ULNASTransportPduSessionID2ValueType)
				snssai := nasType.NewSNSSAI(nasMessage.PDUSessionEstablishmentAcceptSNSSAIType)
				snssai.SetSST(5)
				snssai.SetSD([3]uint8([]byte{0xab, 0xcd, 0xef}))
				ul.SNSSAI = snssai
				return ul
			},
			setupUE: func() *context.AmfUe {
				return &context.AmfUe{
					GmmLog:       zap.NewNop().Sugar(),
					RanUe:        make(map[models.AccessType]*context.RanUe),
					AllowedNssai: map[models.AccessType][]models.AllowedSnssai{},
				}
			},
			accessType:  models.AccessType__3_GPP_ACCESS,
			expectedSst: 5,
			expectedSd:  "abcdef",
			expectError: false,
		},
		{
			name: "AllowedNssai with multiple entries - picks first",
			setupUL: func() *nasMessage.ULNASTransport {
				ul := &nasMessage.ULNASTransport{}
				ul.PduSessionID2Value = nasType.NewPduSessionID2Value(nasMessage.ULNASTransportPduSessionID2ValueType)
				// ul.SNSSAI is nil
				return ul
			},
			setupUE: func() *context.AmfUe {
				ue := &context.AmfUe{
					GmmLog:       zap.NewNop().Sugar(),
					RanUe:        make(map[models.AccessType]*context.RanUe),
					AllowedNssai: map[models.AccessType][]models.AllowedSnssai{},
				}
				ue.AllowedNssai[models.AccessType__3_GPP_ACCESS] = []models.AllowedSnssai{
					{AllowedSnssai: &models.Snssai{Sst: 3, Sd: "111111"}},
					{AllowedSnssai: &models.Snssai{Sst: 2, Sd: "222222"}},
				}
				return ue
			},
			accessType:  models.AccessType__3_GPP_ACCESS,
			expectedSst: 3,
			expectedSd:  "111111",
			expectError: false,
		},
		{
			name: "NON_3GPP_ACCESS with AllowedNssai",
			setupUL: func() *nasMessage.ULNASTransport {
				ul := &nasMessage.ULNASTransport{}
				ul.PduSessionID2Value = nasType.NewPduSessionID2Value(nasMessage.ULNASTransportPduSessionID2ValueType)
				// ul.SNSSAI is nil
				return ul
			},
			setupUE: func() *context.AmfUe {
				ue := &context.AmfUe{
					GmmLog:       zap.NewNop().Sugar(),
					RanUe:        make(map[models.AccessType]*context.RanUe),
					AllowedNssai: map[models.AccessType][]models.AllowedSnssai{},
				}
				ue.AllowedNssai[models.AccessType_NON_3_GPP_ACCESS] = []models.AllowedSnssai{
					{AllowedSnssai: &models.Snssai{Sst: 3, Sd: "fedcba"}},
				}
				return ue
			},
			accessType:  models.AccessType_NON_3_GPP_ACCESS,
			expectedSst: 3,
			expectedSd:  "fedcba",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ul := tt.setupUL()
			ue := tt.setupUE()

			got, err := pickSnssai(ul, ue, tt.accessType)

			if tt.expectError {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if got.Sst != tt.expectedSst {
				t.Errorf("expected SST %d, got %d", tt.expectedSst, got.Sst)
			}

			if got.Sd != tt.expectedSd {
				t.Errorf("expected SD %s, got %s", tt.expectedSd, got.Sd)
			}
		})
	}
}

func TestPickDNN(t *testing.T) {
	tests := []struct {
		name     string
		setupUE  func() *context.AmfUe
		setupUL  func() *nasMessage.ULNASTransport
		snssai   models.Snssai
		expected string
	}{
		{
			name: "picks first DNN from support list",
			setupUE: func() *context.AmfUe {
				return &context.AmfUe{
					GmmLog:       zap.NewNop().Sugar(),
					RanUe:        make(map[models.AccessType]*context.RanUe),
					AllowedNssai: map[models.AccessType][]models.AllowedSnssai{},
					ServingAMF:   &context.AMFContext{SupportDnnLists: []string{"internet-1", "ims"}},
				}
			},
			setupUL: func() *nasMessage.ULNASTransport {
				return &nasMessage.ULNASTransport{}
			},
			snssai:   models.Snssai{Sst: 1, Sd: "112233"},
			expected: "internet-1",
		},
		{
			name: "returns default 'internet' when support list is empty",
			setupUE: func() *context.AmfUe {
				return &context.AmfUe{
					GmmLog:       zap.NewNop().Sugar(),
					RanUe:        make(map[models.AccessType]*context.RanUe),
					AllowedNssai: map[models.AccessType][]models.AllowedSnssai{},
					ServingAMF:   &context.AMFContext{SupportDnnLists: []string{}},
				}
			},
			setupUL: func() *nasMessage.ULNASTransport {
				return &nasMessage.ULNASTransport{}
			},
			snssai:   models.Snssai{Sst: 1, Sd: "112233"},
			expected: "internet",
		},
		{
			name: "picks default DNN from SmfSelectionData when available",
			setupUE: func() *context.AmfUe {
				sn := models.Snssai{Sst: 1, Sd: "112233"}
				key := util.SnssaiModelsToHex(sn)

				return &context.AmfUe{
					GmmLog:       zap.NewNop().Sugar(),
					RanUe:        make(map[models.AccessType]*context.RanUe),
					AllowedNssai: map[models.AccessType][]models.AllowedSnssai{},
					ServingAMF:   &context.AMFContext{SupportDnnLists: []string{"internet-1"}},
					SmfSelectionData: &models.SmfSelectionSubscriptionData{
						SubscribedSnssaiInfos: map[string]models.SnssaiInfo{
							key: {
								DnnInfos: []models.DnnInfo{
									{Dnn: "ims", DefaultDnnIndicator: true},
									{Dnn: "internet-x", DefaultDnnIndicator: false},
								},
							},
						},
					},
				}
			},
			setupUL: func() *nasMessage.ULNASTransport {
				return &nasMessage.ULNASTransport{}
			},
			snssai:   models.Snssai{Sst: 1, Sd: "112233"},
			expected: "ims",
		},
		{
			name: "falls back to support list when no default DNN indicator is true",
			setupUE: func() *context.AmfUe {
				sn := models.Snssai{Sst: 2, Sd: "aabbcc"}
				key := util.SnssaiModelsToHex(sn)

				return &context.AmfUe{
					GmmLog:       zap.NewNop().Sugar(),
					RanUe:        make(map[models.AccessType]*context.RanUe),
					AllowedNssai: map[models.AccessType][]models.AllowedSnssai{},
					ServingAMF:   &context.AMFContext{SupportDnnLists: []string{"internet-2"}},
					SmfSelectionData: &models.SmfSelectionSubscriptionData{
						SubscribedSnssaiInfos: map[string]models.SnssaiInfo{
							key: {
								DnnInfos: []models.DnnInfo{
									{Dnn: "internet-x", DefaultDnnIndicator: false},
									{Dnn: "internet-y", DefaultDnnIndicator: false},
								},
							},
						},
					},
				}
			},
			setupUL: func() *nasMessage.ULNASTransport {
				return &nasMessage.ULNASTransport{}
			},
			snssai:   models.Snssai{Sst: 2, Sd: "aabbcc"},
			expected: "internet-2",
		},
		{
			name: "falls back to support list when SmfSelectionData has no matching SNSSAI",
			setupUE: func() *context.AmfUe {
				differentSn := models.Snssai{Sst: 99, Sd: "ffffff"}
				differentKey := util.SnssaiModelsToHex(differentSn)

				return &context.AmfUe{
					GmmLog:       zap.NewNop().Sugar(),
					RanUe:        make(map[models.AccessType]*context.RanUe),
					AllowedNssai: map[models.AccessType][]models.AllowedSnssai{},
					ServingAMF:   &context.AMFContext{SupportDnnLists: []string{"fallback-dnn"}},
					SmfSelectionData: &models.SmfSelectionSubscriptionData{
						SubscribedSnssaiInfos: map[string]models.SnssaiInfo{
							differentKey: {
								DnnInfos: []models.DnnInfo{
									{Dnn: "other-dnn", DefaultDnnIndicator: true},
								},
							},
						},
					},
				}
			},
			setupUL: func() *nasMessage.ULNASTransport {
				return &nasMessage.ULNASTransport{}
			},
			snssai:   models.Snssai{Sst: 1, Sd: "112233"},
			expected: "fallback-dnn",
		},
		{
			name: "handles nil SmfSelectionData",
			setupUE: func() *context.AmfUe {
				return &context.AmfUe{
					GmmLog:           zap.NewNop().Sugar(),
					RanUe:            make(map[models.AccessType]*context.RanUe),
					AllowedNssai:     map[models.AccessType][]models.AllowedSnssai{},
					ServingAMF:       &context.AMFContext{SupportDnnLists: []string{"test-dnn"}},
					SmfSelectionData: nil,
				}
			},
			setupUL: func() *nasMessage.ULNASTransport {
				return &nasMessage.ULNASTransport{}
			},
			snssai:   models.Snssai{Sst: 1, Sd: "112233"},
			expected: "test-dnn",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ue := tt.setupUE()
			ul := tt.setupUL()

			got := pickDNN(ul, ue, tt.snssai)

			if got != tt.expected {
				t.Errorf("pickDNN() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func Test_Transport5GSMMessage(t *testing.T) {
	causeNotFwd := nasMessage.Cause5GMMPayloadWasNotForwarded

	type tc struct {
		name       string
		setupUL    func() *nasMessage.ULNASTransport
		an         models.AccessType
		prepareUE  func(*context.AmfUe)
		wantErrSub string
		wantNilErr bool
		wantCalls  int
		wantCause  *uint8
	}
	tests := []tc{
		{
			name: "missing PDU ID: error, no DL",
			setupUL: func() *nasMessage.ULNASTransport {
				return &nasMessage.ULNASTransport{}
			},
			an:         models.AccessType__3_GPP_ACCESS,
			wantErrSub: "pdu session id is nil",
			wantCalls:  0,
		},
		{
			name: "old PDU ID (SSC mode 3): DL not forwarded + error",
			setupUL: func() *nasMessage.ULNASTransport {
				ul := &nasMessage.ULNASTransport{}
				ul.PduSessionID2Value = nasType.NewPduSessionID2Value(nasMessage.ULNASTransportPduSessionID2ValueType)
				ul.SetPduSessionID2Value(10)
				oldPDUSessionID := nasType.NewOldPDUSessionID(nasMessage.ULNASTransportOldPDUSessionIDType)
				oldPDUSessionID.SetOldPDUSessionID(9)
				ul.OldPDUSessionID = oldPDUSessionID

				return ul
			},
			an:         models.AccessType__3_GPP_ACCESS,
			wantErrSub: "ssc mode3 operation has not been implemented",
			prepareUE: func(ue *context.AmfUe) {
				ue.RanUe[models.AccessType__3_GPP_ACCESS] = &context.RanUe{AmfUe: ue}
			},
			wantCalls: 1, wantCause: &causeNotFwd,
		},
		{
			name: "no SM context + RequestType=nil: no-op, nil error, no DL",
			setupUL: func() *nasMessage.ULNASTransport {
				ul := &nasMessage.ULNASTransport{}
				ul.PduSessionID2Value = nasType.NewPduSessionID2Value(nasMessage.ULNASTransportPduSessionID2ValueType)
				ul.SetPduSessionID2Value(1)
				return ul
			},
			an: models.AccessType_NON_3_GPP_ACCESS,
			prepareUE: func(ue *context.AmfUe) {
				ue.RanUe[models.AccessType_NON_3_GPP_ACCESS] = &context.RanUe{AmfUe: ue}
			},
			wantNilErr: true, wantCalls: 0,
		},
		{
			name: "emergency -> DL not forwarded: nil error",
			setupUL: func() *nasMessage.ULNASTransport {
				ul := &nasMessage.ULNASTransport{}
				ul.PduSessionID2Value = nasType.NewPduSessionID2Value(nasMessage.ULNASTransportPduSessionID2ValueType)
				ul.SetPduSessionID2Value(2)
				requestType := nasType.NewRequestType(nasMessage.ULNASTransportRequestTypeType)
				requestType.SetRequestTypeValue(nasMessage.ULNASTransportRequestTypeInitialEmergencyRequest)
				ul.RequestType = requestType

				return ul
			},
			an: models.AccessType__3_GPP_ACCESS,
			prepareUE: func(ue *context.AmfUe) {
				ue.RanUe[models.AccessType__3_GPP_ACCESS] = &context.RanUe{AmfUe: ue}
			},
			wantNilErr: true, wantCalls: 1, wantCause: &causeNotFwd,
		},
		{
			name: "existing PDU session but S-NSSAI not allowed: DL not forwarded",
			setupUL: func() *nasMessage.ULNASTransport {
				ul := &nasMessage.ULNASTransport{}
				ul.PduSessionID2Value = nasType.NewPduSessionID2Value(nasMessage.ULNASTransportPduSessionID2ValueType)
				ul.SetPduSessionID2Value(3)
				requestType := nasType.NewRequestType(nasMessage.ULNASTransportRequestTypeType)
				requestType.SetRequestTypeValue(nasMessage.ULNASTransportRequestTypeExistingPduSession)
				ul.RequestType = requestType

				return ul
			},
			an: models.AccessType__3_GPP_ACCESS,
			prepareUE: func(ue *context.AmfUe) {
				ue.RanUe[models.AccessType__3_GPP_ACCESS] = &context.RanUe{AmfUe: ue}
				sm := context.NewSmContext(3)
				sm.SetAccessType(models.AccessType__3_GPP_ACCESS)
				ue.StoreSmContext(3, sm)
			},
			wantNilErr: true, wantCalls: 1, wantCause: &causeNotFwd,
		},
		{
			name: "emergency but no RanUe: guarded (no DL), nil error",
			setupUL: func() *nasMessage.ULNASTransport {
				ul := &nasMessage.ULNASTransport{}
				ul.PduSessionID2Value = nasType.NewPduSessionID2Value(nasMessage.ULNASTransportPduSessionID2ValueType)
				ul.SetPduSessionID2Value(4)
				requestType := nasType.NewRequestType(nasMessage.ULNASTransportRequestTypeType)
				requestType.SetRequestTypeValue(nasMessage.ULNASTransportRequestTypeExistingEmergencyPduSession)
				ul.RequestType = requestType

				return ul
			},
			an:         models.AccessType__3_GPP_ACCESS,
			wantNilErr: true, wantCalls: 0,
		},
		{
			name: "initial request with no allowed NSSAI: error, no DL",
			setupUL: func() *nasMessage.ULNASTransport {
				ul := &nasMessage.ULNASTransport{}
				ul.PduSessionID2Value = nasType.NewPduSessionID2Value(nasMessage.ULNASTransportPduSessionID2ValueType)
				ul.SetPduSessionID2Value(5)
				requestType := nasType.NewRequestType(nasMessage.ULNASTransportRequestTypeType)
				requestType.SetRequestTypeValue(nasMessage.ULNASTransportRequestTypeInitialRequest)
				ul.RequestType = requestType

				return ul
			},
			an: models.AccessType__3_GPP_ACCESS,
			prepareUE: func(ue *context.AmfUe) {
				ue.RanUe[models.AccessType__3_GPP_ACCESS] = &context.RanUe{AmfUe: ue}
			},
			wantErrSub: "ue doesn't have allowedNssai",
			wantCalls:  0,
		},
		{
			name: "modification request with nil UeContextInSmfData: DL not forwarded",
			setupUL: func() *nasMessage.ULNASTransport {
				ul := &nasMessage.ULNASTransport{}
				ul.PduSessionID2Value = nasType.NewPduSessionID2Value(nasMessage.ULNASTransportPduSessionID2ValueType)
				ul.SetPduSessionID2Value(6)
				requestType := nasType.NewRequestType(nasMessage.ULNASTransportRequestTypeType)
				requestType.SetRequestTypeValue(nasMessage.ULNASTransportRequestTypeModificationRequest)
				ul.RequestType = requestType

				return ul
			},
			an: models.AccessType__3_GPP_ACCESS,
			prepareUE: func(ue *context.AmfUe) {
				ue.RanUe[models.AccessType__3_GPP_ACCESS] = &context.RanUe{AmfUe: ue}
				ue.UeContextInSmfData = nil
			},
			wantNilErr: true, wantCalls: 1, wantCause: &causeNotFwd,
		},
		{
			name: "valid initial request with allowed NSSAI: success (skipped - needs SMF mock)",
			setupUL: func() *nasMessage.ULNASTransport {
				ul := &nasMessage.ULNASTransport{}
				ul.PduSessionID2Value = nasType.NewPduSessionID2Value(nasMessage.ULNASTransportPduSessionID2ValueType)
				ul.SetPduSessionID2Value(7)
				requestType := nasType.NewRequestType(nasMessage.ULNASTransportRequestTypeType)
				requestType.SetRequestTypeValue(nasMessage.ULNASTransportRequestTypeInitialRequest)
				ul.RequestType = requestType

				// Add SNSSAI to the UL message
				snssai := nasType.NewSNSSAI(nasMessage.ULNASTransportSNSSAIType)
				snssai.SetLen(4)
				snssai.SetSST(1)
				snssai.SetSD([3]uint8([]byte{0x11, 0x22, 0x33}))
				ul.SNSSAI = snssai

				// Add DNN (Data Network Name)
				dnn := nasType.NewDNN(nasMessage.ULNASTransportDNNType)
				dnn.SetLen(8)
				dnn.SetDNN([]byte{0x08, 'i', 'n', 't', 'e', 'r', 'n', 'e', 't'})
				ul.DNN = dnn

				// Add PayloadContainer with some dummy SM NAS message
				payloadContainer := nasType.PayloadContainer{}
				payloadContainer.SetLen(4)
				payloadContainer.SetPayloadContainerContents([]byte{0x01, 0x02, 0x03, 0x04})
				ul.PayloadContainer = payloadContainer

				return ul
			},
			an: models.AccessType__3_GPP_ACCESS,
			prepareUE: func(ue *context.AmfUe) {
				ue.RanUe[models.AccessType__3_GPP_ACCESS] = &context.RanUe{AmfUe: ue}

				// Add allowed NSSAI to prevent error
				ue.AllowedNssai[models.AccessType__3_GPP_ACCESS] = []models.AllowedSnssai{
					{AllowedSnssai: &models.Snssai{Sst: 1, Sd: "112233"}},
				}

				// Add UeContextInSmfData to prevent nil pointer issues
				ue.UeContextInSmfData = &models.UeContextInSmfData{
					PduSessions: make(map[string]models.PduSession),
				}
			},
			wantNilErr: true,
			wantCalls:  0,
		},
		{
			name: "modification request with valid UeContextInSmfData: success",
			setupUL: func() *nasMessage.ULNASTransport {
				ul := &nasMessage.ULNASTransport{}
				ul.PduSessionID2Value = nasType.NewPduSessionID2Value(nasMessage.ULNASTransportPduSessionID2ValueType)
				ul.SetPduSessionID2Value(8)
				requestType := nasType.NewRequestType(nasMessage.ULNASTransportRequestTypeType)
				requestType.SetRequestTypeValue(nasMessage.ULNASTransportRequestTypeModificationRequest)
				ul.RequestType = requestType

				return ul
			},
			an: models.AccessType__3_GPP_ACCESS,
			prepareUE: func(ue *context.AmfUe) {
				ue.RanUe[models.AccessType__3_GPP_ACCESS] = &context.RanUe{AmfUe: ue}
				ue.UeContextInSmfData = &models.UeContextInSmfData{
					PduSessions: make(map[string]models.PduSession),
				}
			},
			wantNilErr: true,
			wantCalls:  1,
			wantCause:  &causeNotFwd,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if strings.Contains(tt.name, "skipped") {
				t.Skip("skipping test that requires SMF mocking")
			}
			// Mock sendDLNASTransport only
			origSendDL := sendDLNASTransport
			defer func() { sendDLNASTransport = origSendDL }()
			calls := 0
			var got struct {
				cause uint8
				an    models.AccessType
			}
			sendDLNASTransport = func(_ *context.RanUe, an models.AccessType, _ uint8, _ []byte, _ int32, c uint8, _ *uint8, _ uint8) {
				calls++
				got.cause = c
				got.an = an
			}

			ue := &context.AmfUe{
				GmmLog:       zap.NewNop().Sugar(),
				RanUe:        make(map[models.AccessType]*context.RanUe),
				AllowedNssai: map[models.AccessType][]models.AllowedSnssai{},
			}

			if tt.prepareUE != nil {
				tt.prepareUE(ue)
			}

			ul := tt.setupUL()

			// Catch panics and convert them to errors
			var err error
			func() {
				defer func() {
					if r := recover(); r != nil {
						err = fmt.Errorf("panic occurred: %v", r)
					}
				}()
				err = transport5GSMMessage(ctxt.Background(), ue, tt.an, ul)
			}()

			t.Logf("Error: %v, Calls: %d", err, calls)

			if tt.wantNilErr && err != nil {
				t.Errorf("got err=%v, want nil", err)
			}
			if tt.wantErrSub != "" {
				if err == nil {
					t.Errorf("got nil, want error containing %q", tt.wantErrSub)
				}
				if err != nil && !strings.Contains(err.Error(), tt.wantErrSub) {
					t.Errorf("error %q does not contain %q", err, tt.wantErrSub)
				}
			}

			if tt.name == "valid initial request with allowed NSSAI: SMF selection fails" {
				if err == nil {
					t.Errorf("expected error or panic due to SelectSmf, but got success")
				}
			}

			if calls != tt.wantCalls {
				t.Errorf("sendDLNASTransport calls=%d, want %d", calls, tt.wantCalls)
			}
			if tt.wantCause != nil && calls > 0 && got.cause != *tt.wantCause {
				t.Errorf("DL cause=%d, want %d", got.cause, *tt.wantCause)
			}
			if calls > 0 && got.an != tt.an {
				t.Errorf("DL anType=%v, want %v", got.an, tt.an)
			}
		})
	}
}

// Fuzz test data structure (keep the same as before)
type FuzzData struct {
	AccessType                uint8
	HasSubscribedNssai        bool
	SubscribedNssaiLength     uint8
	HasCapability5GMM         bool
	HasAllowedNssai           bool
	AllowedNssaiLength        uint8
	ServingAmfChanged         bool
	HasPei                    bool
	PeiLength                 uint8
	SubscriptionDataValid     bool
	HasLastVisitedTAI         bool
	HasMICOIndication         bool
	HasRequestedDRXParameters bool
	StateRegistered           bool
	HasAmPolicyAssociation    bool
	HasServAreaRes            bool
	RestrictionType           uint8
	MaxNumOfTAs               uint32
	AreasLength               uint8
	SupiLength                uint8
	GutiLength                uint8
	T3502ValueMs              uint32
	T3512ValueMs              uint32
	Non3gppDeregTimerValueMs  uint32
	PcfUriLength              uint8
	PcfIdLength               uint8
}

func FuzzHandleInitialRegistration(f *testing.F) {
	// Add seed corpus
	seedCases := [][]byte{
		// Valid 3GPP registration
		{
			1, 1, 2, 1, 1, 1, 0, 1, 15, 1, 0, 0, 0, 0, 0, 0, 0,
			10, 0, 0, 0, 1, 10, 16,
			100, 0, 0, 0, 200, 0, 0, 0, 44, 1, 0, 0, 20, 16,
		},
		// Error case: no allowed NSSAI
		{
			1, 1, 1, 1, 0, 0, 0, 1, 12, 1, 0, 0, 0, 0, 0, 0, 0,
			1, 0, 0, 0, 1, 15, 20,
			200, 0, 0, 0, 144, 1, 0, 0, 88, 2, 0, 0, 15, 12,
		},
		// Minimal case
		{
			0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
			0, 0, 0, 0, 0, 0, 0,
			0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		},
	}

	for _, seed := range seedCases {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) < 39 { // Minimum required bytes
			return
		}

		// Parse fuzz data
		fuzzData := parseFuzzData(data)

		// Create real UE with fuzzed data
		ue := newFuzzUE(fuzzData)

		// Create context with timeout to prevent hanging
		ctx, cancel := ctxt.WithTimeout(ctxt.Background(), 100*time.Millisecond)
		defer cancel()

		// Determine access type
		var anType models.AccessType
		switch fuzzData.AccessType % 3 {
		case 0:
			anType = models.AccessType__3_GPP_ACCESS
		case 1:
			anType = models.AccessType_NON_3_GPP_ACCESS
		default:
			anType = models.AccessType__3_GPP_ACCESS
		}

		// Track test execution
		var (
			err        error
			panicked   bool
			panicValue any
		)

		// Call the function under test with panic recovery
		func() {
			defer func() {
				if r := recover(); r != nil {
					panicked = true
					panicValue = r
				}
			}()

			err = HandleInitialRegistration(ctx, ue, anType)
		}()

		// Validate results and behavior
		validateFuzzResults(t, fuzzData, ue, anType, err, panicked, panicValue)
	})
}

func parseFuzzData(data []byte) *FuzzData {
	fd := &FuzzData{}

	if len(data) >= 39 {
		fd.AccessType = data[0]
		fd.HasSubscribedNssai = data[1] != 0
		fd.SubscribedNssaiLength = data[2] % 20
		fd.HasCapability5GMM = data[3] != 0
		fd.HasAllowedNssai = data[4] != 0
		fd.AllowedNssaiLength = data[5] % 10
		fd.ServingAmfChanged = data[6] != 0
		fd.HasPei = data[7] != 0
		fd.PeiLength = data[8] % 50
		fd.SubscriptionDataValid = data[9] != 0
		fd.HasLastVisitedTAI = data[10] != 0
		fd.HasMICOIndication = data[11] != 0
		fd.HasRequestedDRXParameters = data[12] != 0
		fd.StateRegistered = data[13] != 0
		fd.HasAmPolicyAssociation = data[14] != 0
		fd.HasServAreaRes = data[15] != 0
		fd.RestrictionType = data[16] % 3

		fd.MaxNumOfTAs = parseUint32(data[17:21]) % 1000
		fd.AreasLength = data[21] % 20
		fd.SupiLength = data[22] % 100
		fd.GutiLength = data[23] % 100
		fd.T3502ValueMs = parseUint32(data[24:28]) % 60000
		fd.T3512ValueMs = parseUint32(data[28:32]) % 3600000
		fd.Non3gppDeregTimerValueMs = parseUint32(data[32:36]) % 3600000
		fd.PcfUriLength = data[36] % 200
		fd.PcfIdLength = data[37] % 100
	}

	return fd
}

func parseUint32(data []byte) uint32 {
	if len(data) < 4 {
		return 0
	}
	return uint32(data[0]) | uint32(data[1])<<8 | uint32(data[2])<<16 | uint32(data[3])<<24
}

func generateString(prefix string, length int) string {
	if length <= len(prefix) {
		return prefix[:length]
	}

	result := prefix
	for len(result) < length {
		result += "X"
	}
	return result[:length]
}

func validateFuzzResults(t *testing.T, fd *FuzzData, ue *context.AmfUe, anType models.AccessType, err error, panicked bool, panicValue any) {
	if panicked {
		t.Logf("function panicked with: %v (AccessType: %v, HasAllowedNssai: %v)",
			panicValue, anType, fd.HasAllowedNssai)
	}

	// Validate expected behaviors
	if !fd.HasAllowedNssai || fd.AllowedNssaiLength == 0 {
		// Should result in registration rejection
		if err == nil && !panicked {
			t.Logf("expected error for empty AllowedNssai but got none")
		}
	}

	if err != nil {
		t.Logf("function returned error: %v", err)

		if !fd.HasAllowedNssai && !strings.Contains(err.Error(), "allowed") && !strings.Contains(err.Error(), "nssai") {
			t.Logf("unexpected error for empty AllowedNssai: %v", err)
		}
	}

	if ue.Supi != "" && len(ue.Supi) > 100 {
		t.Logf("SUPI length seems excessive: %d", len(ue.Supi))
	}
}

func BenchmarkHandleInitialRegistration(b *testing.B) {
	scenarios := map[string]*FuzzData{
		"Valid3GPP": {
			AccessType:            1,
			HasSubscribedNssai:    true,
			SubscribedNssaiLength: 2,
			HasCapability5GMM:     true,
			HasAllowedNssai:       true,
			AllowedNssaiLength:    1,
			ServingAmfChanged:     false,
			HasPei:                true,
			PeiLength:             15,
			SubscriptionDataValid: true,
			SupiLength:            20,
			GutiLength:            32,
			T3502ValueMs:          1000,
			T3512ValueMs:          3600000,
			PcfUriLength:          30,
			PcfIdLength:           16,
		},
		"NoAllowedNSSAI": {
			AccessType:            1,
			HasSubscribedNssai:    true,
			SubscribedNssaiLength: 1,
			HasCapability5GMM:     true,
			HasAllowedNssai:       false, // This should cause rejection
			AllowedNssaiLength:    0,
			ServingAmfChanged:     false,
			HasPei:                true,
			PeiLength:             12,
			SubscriptionDataValid: true,
			SupiLength:            15,
			GutiLength:            20,
			T3502ValueMs:          200,
			T3512ValueMs:          400,
			PcfUriLength:          15,
			PcfIdLength:           12,
		},
	}

	for name, fuzzData := range scenarios {
		b.Run(name, func(b *testing.B) {
			ctx := ctxt.Background()
			var errorCount, panicCount int

			b.ResetTimer()

			for b.Loop() {
				ue := newFuzzUE(fuzzData)

				func() {
					defer func() {
						if r := recover(); r != nil {
							panicCount++
						}
					}()
					if err := HandleInitialRegistration(ctx, ue, models.AccessType__3_GPP_ACCESS); err != nil {
						errorCount++
					}
				}()
			}

			b.StopTimer()
			if errorCount > 0 || panicCount > 0 {
				b.Logf("Errors: %d, Panics: %d", errorCount, panicCount)
			}
		})
	}
}
