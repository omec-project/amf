// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
//
// SPDX-License-Identifier: Apache-2.0

package gmm

import (
	ctxt "context"
	"errors"
	"testing"

	"github.com/omec-project/amf/consumer"
	"github.com/omec-project/amf/context"
	"github.com/omec-project/nas/v2/nasMessage"
	"github.com/omec-project/nas/v2/nasType"
	"github.com/omec-project/openapi/v2"
	"github.com/omec-project/openapi/v2/models"
	"github.com/omec-project/util/fsm"
	"go.uber.org/zap"
)

// 5GMM cause #7 tells the UE its USIM is not allowed 5G services on this PLMN. Per
// TS 24.501 5.5.1.2.5 the UE deletes its 5G-GUTI, TAI list and ngKSI and treats the USIM
// as invalid for 5GS until it is switched off, the UICC is removed or T3245 expires.
// Congestion, sent without a T3346 value, is an abnormal case (5.5.1.2.7): the UE
// retries on T3511 and then T3502. A peer the AMF could not reach says nothing about the
// subscriber, so it must get the second answer, never the first.

// rejectTestUe returns a UE attached over 3GPP access, far enough into initial
// registration for HandleInitialRegistration to reach its reject paths.
func rejectTestUe() *context.AmfUe {
	ue := &context.AmfUe{
		GmmLog:           zap.NewNop().Sugar(),
		NASLog:           zap.NewNop().Sugar(),
		RanUe:            make(map[models.AccessType]*context.RanUe),
		AllowedNssai:     make(map[models.AccessType][]models.AllowedSnssai),
		RegistrationArea: make(map[models.AccessType][]models.Tai),
		State:            make(map[models.AccessType]*fsm.State),
		ReleaseCause:     make(map[models.AccessType]*context.CauseAll),
		Pei:              testPei,
		Supi:             "imsi-001010000000001",
	}
	ue.State[models.ACCESSTYPE__3_GPP_ACCESS] = fsm.NewState(context.Deregistered)
	ue.State[models.ACCESSTYPE_NON_3_GPP_ACCESS] = fsm.NewState(context.Deregistered)

	ran := context.NewAmfRanDefault()
	ran.RanId = models.NewGlobalRanNodeId(models.PlmnId{Mcc: "001", Mnc: "01"})
	ranUe := &context.RanUe{
		AmfUe: ue,
		Ran:   ran,
		Log:   zap.NewNop().Sugar(),
		Tai:   models.Tai{PlmnId: models.PlmnId{Mcc: "001", Mnc: "01"}, Tac: "000001"},
	}
	ue.RanUe[models.ACCESSTYPE__3_GPP_ACCESS] = ranUe
	ue.Tai = ranUe.Tai

	capability := nasType.NewCapability5GMM(nasMessage.RegistrationRequestCapability5GMMType)
	capability.SetLen(13)
	ue.RegistrationRequest = &nasMessage.RegistrationRequest{Capability5GMM: capability}

	return ue
}

// captureRejectCause replaces the registration reject sender for the test and returns a
// pointer to the cause it was called with, nil if it was not called.
func captureRejectCause(t *testing.T) **uint8 {
	t.Helper()

	original := sendRegistrationRejectForRegistration
	t.Cleanup(func() { sendRegistrationRejectForRegistration = original })

	var got *uint8
	sendRegistrationRejectForRegistration = func(_ *context.RanUe, cause uint8, _ string) {
		got = &cause
	}

	return &got
}

// The AMF reaches the same empty allowed NSSAI whether the UDM said the subscriber has
// no slices or could not be reached, so the fetch outcome is what decides the cause.
func TestAnEmptyAllowedNssaiIsCause7OnlyWhenTheUdmAnswered(t *testing.T) {
	tests := []struct {
		name       string
		fetchError error
		wantCause  uint8
	}{
		{
			name:      "the UDM answered: the subscriber has no slices",
			wantCause: nasMessage.Cause5GMM5GSServicesNotAllowed,
		},
		{
			name:       "the UDM did not answer",
			fetchError: errors.New("udm unreachable"),
			wantCause:  nasMessage.Cause5GMMCongestion,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The reject path publishes the UE's removal, which reads the Kafka setting.
			disableKafkaForRelayTest(t)
			originalGet, originalNssai := getSubscribedNssaiForRegistration, handleRequestedNssaiForRegistration
			t.Cleanup(func() {
				getSubscribedNssaiForRegistration, handleRequestedNssaiForRegistration = originalGet, originalNssai
			})
			getSubscribedNssaiForRegistration = func(ctxt.Context, *context.AmfUe) error {
				return tt.fetchError
			}
			// Leaves the allowed NSSAI empty, which is where both cases arrive.
			handleRequestedNssaiForRegistration = func(
				ctxt.Context, *context.AmfUe, *nasMessage.RegistrationRequest, models.AccessType,
			) error {
				return nil
			}
			cause := captureRejectCause(t)

			if err := HandleInitialRegistration(ctxt.Background(), rejectTestUe(),
				models.ACCESSTYPE__3_GPP_ACCESS); err == nil {
				t.Fatal("HandleInitialRegistration succeeded with an empty allowed NSSAI")
			}

			if *cause == nil {
				t.Fatal("no registration reject was sent for an empty allowed NSSAI")
			}
			if **cause != tt.wantCause {
				t.Errorf("reject cause = %d, want %d", **cause, tt.wantCause)
			}
		})
	}
}

// The PCF decides policy, not entitlement: the UDM has already accepted the subscriber
// by the time the AM policy association is created, so a PCF that refuses or cannot be
// reached is a condition to retry after, not a reason to invalidate the USIM.
func TestAPolicyAssociationFailureIsAnsweredWithCongestion(t *testing.T) {
	tests := []struct {
		name           string
		problemDetails *models.ProblemDetails
		err            error
	}{
		{
			name:           "the PCF refused",
			problemDetails: &models.ProblemDetails{Status: openapi.PtrInt32(403)},
		},
		{
			name: "the PCF could not be reached",
			err:  errors.New("pcf unreachable"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stubRegistrationUpToPolicy(t)
			originalPolicy := amPolicyControlCreateForRegistration
			t.Cleanup(func() { amPolicyControlCreateForRegistration = originalPolicy })
			amPolicyControlCreateForRegistration = func(
				ctxt.Context, *context.AmfUe, models.AccessType,
			) (*models.ProblemDetails, error) {
				return tt.problemDetails, tt.err
			}
			cause := captureRejectCause(t)

			if err := HandleInitialRegistration(ctxt.Background(), rejectTestUe(),
				models.ACCESSTYPE__3_GPP_ACCESS); err == nil {
				t.Fatal("HandleInitialRegistration succeeded without an AM policy association")
			}

			if *cause == nil {
				t.Fatal("no registration reject was sent when the AM policy association failed")
			}
			if **cause != nasMessage.Cause5GMMCongestion {
				t.Errorf("reject cause = %d, want congestion (%d)", **cause, nasMessage.Cause5GMMCongestion)
			}
		})
	}
}

// stubRegistrationUpToPolicy lets HandleInitialRegistration run as far as creating the
// AM policy association: a subscription with one slice, UDM registration done, and a
// PCF discovered.
func stubRegistrationUpToPolicy(t *testing.T) {
	t.Helper()

	originalGet, originalNssai := getSubscribedNssaiForRegistration, handleRequestedNssaiForRegistration
	originalUdm, originalLadn := communicateWithUDMForRegistration, assignLadnInfoForRegistration
	originalSearch := sendSearchNFInstancesForRegistration
	t.Cleanup(func() {
		getSubscribedNssaiForRegistration, handleRequestedNssaiForRegistration = originalGet, originalNssai
		communicateWithUDMForRegistration, assignLadnInfoForRegistration = originalUdm, originalLadn
		sendSearchNFInstancesForRegistration = originalSearch
	})

	slice := models.Snssai{Sst: 1, Sd: openapi.PtrString("010203")}
	getSubscribedNssaiForRegistration = func(_ ctxt.Context, ue *context.AmfUe) error {
		ue.SubscribedNssai = []models.SubscribedSnssai{{SubscribedSnssai: slice, DefaultIndication: openapi.PtrBool(true)}}

		return nil
	}
	handleRequestedNssaiForRegistration = func(
		_ ctxt.Context, ue *context.AmfUe, _ *nasMessage.RegistrationRequest, anType models.AccessType,
	) error {
		ue.AllowedNssai[anType] = []models.AllowedSnssai{{AllowedSnssai: slice}}

		return nil
	}
	communicateWithUDMForRegistration = func(_ ctxt.Context, ue *context.AmfUe, _ models.AccessType) error {
		ue.SubscriptionDataValid = true

		return nil
	}
	assignLadnInfoForRegistration = func(*context.AmfUe, *nasMessage.RegistrationRequest, models.AccessType) {}
	sendSearchNFInstancesForRegistration = func(
		ctxt.Context, string, models.NFType, models.NFType, consumer.SearchNFInstancesRequestConfigurer,
	) (*models.SearchResult, error) {
		nfProfile := models.NFProfileDiscovery{NfInstanceId: "pcf-instance"}
		nfProfile.SetNfServiceList(map[string]models.NFService{
			"0": {
				ServiceName:     models.SERVICENAME_NPCF_AM_POLICY_CONTROL,
				NfServiceStatus: models.NFSERVICESTATUS_REGISTERED,
				ApiPrefix:       openapi.PtrString("http://pcf.example.com"),
			},
		})

		return models.NewSearchResult(300, []models.NFProfileDiscovery{nfProfile}), nil
	}
}

// A 4xx is the UDM answering, and must not be mistaken for the UDM being unreachable:
// that would answer a subscriber with no slice data with congestion, and the UE would
// retry an answer that is not going to change.
func TestSliceSubscriptionUnavailableTellsNoAnswerFromAnAnswer(t *testing.T) {
	tests := []struct {
		name            string
		problemDetails  *models.ProblemDetails
		err             error
		wantUnavailable bool
	}{
		{name: "fetched"},
		{
			name:           "404: the UDM has no slice data for the subscriber",
			problemDetails: &models.ProblemDetails{Status: openapi.PtrInt32(404)},
		},
		{
			name:            "429: the UDM asked the AMF to slow down",
			problemDetails:  &models.ProblemDetails{Status: openapi.PtrInt32(429)},
			wantUnavailable: true,
		},
		{
			name:            "408: the UDM timed the request out",
			problemDetails:  &models.ProblemDetails{Status: openapi.PtrInt32(408)},
			wantUnavailable: true,
		},
		{
			name:            "503: the UDM could not answer",
			problemDetails:  &models.ProblemDetails{Status: openapi.PtrInt32(503)},
			wantUnavailable: true,
		},
		{
			name:            "a problem with no status",
			problemDetails:  &models.ProblemDetails{},
			wantUnavailable: true,
		},
		{
			name:            "no response",
			err:             errors.New("udm unreachable"),
			wantUnavailable: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := sliceSubscriptionUnavailable(tt.problemDetails, tt.err)
			if (err != nil) != tt.wantUnavailable {
				t.Errorf("sliceSubscriptionUnavailable() = %v, want unavailable = %v", err, tt.wantUnavailable)
			}
		})
	}
}
