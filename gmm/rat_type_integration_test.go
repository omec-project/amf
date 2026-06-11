// Copyright (c) 2026 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package gmm_test

import (
	ctxt "context"
	"strings"
	"testing"

	amf_context "github.com/omec-project/amf/context"
	"github.com/omec-project/amf/gmm"
	"github.com/omec-project/amf/ngap"
	ngaputil "github.com/omec-project/amf/ngap/util"
	"github.com/omec-project/nas/v2/nasMessage"
	"github.com/omec-project/ngap/v2/ngapType"
	"github.com/omec-project/openapi/v2/models"
	"go.uber.org/zap"
)

func TestHandleRegistrationRequestUpgradesRatTypeFromNGSetupRATInformation(t *testing.T) {
	amfSelf := amf_context.AMF_Self()
	originalSupportTaiLists := amfSelf.SupportTaiLists
	defer func() {
		amfSelf.SupportTaiLists = originalSupportTaiLists
	}()

	amfSelf.SupportTaiLists = nil

	ngSetupRequest := ngaputil.BuildNGSetupRequest()
	supportedTAItem := &ngSetupRequest.InitiatingMessage.Value.NGSetup.ProtocolIEs.List[2].Value.SupportedTAList.List[0]
	supportedTAItem.IEExtensions = &ngapType.ProtocolExtensionContainerSupportedTAItemExtIEs{
		List: []ngapType.SupportedTAItemExtIEs{{
			Id:          ngapType.ProtocolExtensionID{Value: 179},
			Criticality: ngapType.Criticality{Value: ngapType.CriticalityPresentIgnore},
			ExtensionValue: ngapType.SupportedTAItemExtIEsExtensionValue{
				RATInformation: &ngapType.RATInformation{Value: ngapType.RATInformationPresentNRLEO},
			},
		}},
	}

	ran := &amf_context.AmfRan{
		AnType:          models.ACCESSTYPE__3_GPP_ACCESS,
		Conn:            &ngaputil.TestConn{},
		Log:             zap.NewNop().Sugar(),
		SupportedTAList: amf_context.NewSupportedTAIList(),
	}

	ngap.HandleNGSetupRequest(ran, &ngSetupRequest)

	ratInformation := ran.RatInformationForTAC("000001")
	if ratInformation == nil {
		t.Fatal("expected NG setup to populate RATInformation for TAC 000001")
	}
	if ratInformation.Value != ngapType.RATInformationPresentNRLEO {
		t.Fatalf("expected NG setup RATInformation %v, got %v", ngapType.RATInformationPresentNRLEO, ratInformation.Value)
	}

	amfSelf.SupportTaiLists = []models.Tai{{
		PlmnId: models.PlmnId{Mcc: "208", Mnc: "93"},
		Tac:    "1",
	}}

	ue := &amf_context.AmfUe{
		GmmLog: zap.NewNop().Sugar(),
		RanUe:  map[models.AccessType]*amf_context.RanUe{},
		OnGoing: map[models.AccessType]*amf_context.OnGoingProcedureWithPrio{
			models.ACCESSTYPE__3_GPP_ACCESS: {},
		},
	}
	ue.RanUe[models.ACCESSTYPE__3_GPP_ACCESS] = &amf_context.RanUe{
		AmfUe: ue,
		Ran:   ran,
		Log:   zap.NewNop().Sugar(),
		Tai: models.Tai{
			PlmnId: models.PlmnId{Mcc: "208", Mnc: "93"},
			Tac:    "000001",
		},
		Location: models.UserLocation{NrLocation: models.NewNrLocationWithDefaults()},
	}

	registrationRequest := nasMessage.NewRegistrationRequest(0)
	registrationRequest.SetRegistrationType5GS(nasMessage.RegistrationType5GSInitialRegistration)
	registrationRequest.MobileIdentity5GS.SetLen(1)
	registrationRequest.MobileIdentity5GS.Buffer = []byte{nasMessage.MobileIdentity5GSTypeNoIdentity}

	err := gmm.HandleRegistrationRequest(ctxt.Background(), ue, models.ACCESSTYPE__3_GPP_ACCESS, 0, registrationRequest)
	if err == nil {
		t.Fatal("expected missing UESecurityCapability to stop registration")
	}
	if !strings.Contains(err.Error(), "UESecurityCapability is nil") {
		t.Fatalf("expected UESecurityCapability rejection, got %v", err)
	}
	if ue.RatType != models.RATTYPE_NR_LEO {
		t.Fatalf("expected UE ratType %q after NG setup and registration, got %q", models.RATTYPE_NR_LEO, ue.RatType)
	}
}
