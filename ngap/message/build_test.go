// Copyright (c) 2026 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package message

import (
	"fmt"
	"testing"

	"github.com/omec-project/amf/context"
	"github.com/omec-project/nas/v2/nasType"
	"github.com/omec-project/ngap/v2/ngapConvert"
	"github.com/omec-project/ngap/v2/ngapType"
	"github.com/omec-project/openapi/v2"
	"github.com/omec-project/openapi/v2/models"
)

func TestBuildAllowedNSSAIFromAllowedSnssaiDeduplicates(t *testing.T) {
	allowedNSSAI, err := buildAllowedNSSAIFromAllowedSnssai([]models.AllowedSnssai{
		{AllowedSnssai: models.Snssai{Sst: 1, Sd: openapi.PtrString("010203")}},
		{AllowedSnssai: models.Snssai{Sst: 1, Sd: openapi.PtrString("010203")}},
		{AllowedSnssai: models.Snssai{Sst: 1, Sd: openapi.PtrString("010203")}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := len(allowedNSSAI.List); got != 1 {
		t.Fatalf("expected one deduplicated AllowedNSSAI item, got %d", got)
	}
}

func TestBuildAllowedNSSAIFromAllowedSnssaiRejectsTooManyUnique(t *testing.T) {
	entries := make([]models.AllowedSnssai, 0, maxAllowedNSSAIItems+1)
	for index := range maxAllowedNSSAIItems + 1 {
		entries = append(entries, models.AllowedSnssai{
			AllowedSnssai: models.Snssai{Sst: int32(index + 1), Sd: openapi.PtrString(fmt.Sprintf("%06x", index+1))},
		})
	}

	_, err := buildAllowedNSSAIFromAllowedSnssai(entries)
	if err == nil {
		t.Fatal("expected oversized AllowedNSSAI list to fail")
	}
}

func TestBuildHandoverRequestUsesUEAllowedNSSAI(t *testing.T) {
	ue := newRanUeForAllowedNSSAITest(models.ACCESSTYPE__3_GPP_ACCESS)
	cause := ngapType.Cause{
		Present:      ngapType.CausePresentRadioNetwork,
		RadioNetwork: &ngapType.CauseRadioNetwork{Value: ngapType.CauseRadioNetworkPresentUnspecified},
	}
	pduSessionResourceSetupList := ngapType.PDUSessionResourceSetupListHOReq{
		List: []ngapType.PDUSessionResourceSetupItemHOReq{{
			PDUSessionID:            ngapType.PDUSessionID{Value: 10},
			SNSSAI:                  ngapConvert.SNssaiToNgap(models.Snssai{Sst: 1, Sd: openapi.PtrString("010203")}),
			HandoverRequestTransfer: []byte{0x00},
		}},
	}

	if _, err := BuildHandoverRequest(
		ue,
		cause,
		pduSessionResourceSetupList,
		ngapType.SourceToTargetTransparentContainer{},
		false,
	); err != nil {
		t.Fatalf("expected handover request to use UE allowed NSSAI, got error: %v", err)
	}
}

func TestBuildPathSwitchRequestAcknowledgeUsesUEAllowedNSSAI(t *testing.T) {
	ue := newRanUeForAllowedNSSAITest(models.ACCESSTYPE__3_GPP_ACCESS)
	pduSessionResourceSwitchedList := ngapType.PDUSessionResourceSwitchedList{
		List: []ngapType.PDUSessionResourceSwitchedItem{{
			PDUSessionID:                         ngapType.PDUSessionID{Value: 10},
			PathSwitchRequestAcknowledgeTransfer: []byte{0x00},
		}},
	}

	if _, err := BuildPathSwitchRequestAcknowledge(
		ue,
		pduSessionResourceSwitchedList,
		ngapType.PDUSessionResourceReleasedListPSAck{},
		false,
		nil,
		nil,
		nil,
	); err != nil {
		t.Fatalf("expected path switch request acknowledge to use UE allowed NSSAI, got error: %v", err)
	}
}

func newRanUeForAllowedNSSAITest(anType models.AccessType) *context.RanUe {
	self := context.AMF_Self()
	self.ServedGuamiList = []models.Guami{{
		PlmnId: models.PlmnIdNid{Mcc: "001", Mnc: "01"},
		AmfId:  "cafe00",
	}}
	self.PlmnSupportList = []models.PlmnSnssai{{
		PlmnId:     models.PlmnId{Mcc: "001", Mnc: "01"},
		SNssaiList: buildSnssaiList(maxAllowedNSSAIItems + 1),
	}}

	ueSecurityCapability := nasType.NewUESecurityCapability(0)
	ueSecurityCapability.SetLen(2)

	amfUe := &context.AmfUe{
		AllowedNssai: map[models.AccessType][]models.AllowedSnssai{
			anType: {{AllowedSnssai: models.Snssai{Sst: 1, Sd: openapi.PtrString("010203")}}},
		},
		AccessAndMobilitySubscriptionData: &models.AccessAndMobilitySubscriptionData{
			SubscribedUeAmbr: &models.Ambr{Uplink: "1 Mbps", Downlink: "1 Mbps"},
		},
		UESecurityCapability: *ueSecurityCapability,
		NH:                   make([]byte, 32),
	}

	ran := &context.AmfRan{AnType: anType}
	ue := &context.RanUe{
		AmfUe:       amfUe,
		Ran:         ran,
		AmfUeNgapId: 1,
		RanUeNgapId: 2,
	}
	return ue
}

func buildSnssaiList(count int) []models.Snssai {
	list := make([]models.Snssai, 0, count)
	for index := range count {
		list = append(list, models.Snssai{Sst: int32(index + 1), Sd: openapi.PtrString(fmt.Sprintf("%06x", index+1))})
	}
	return list
}
