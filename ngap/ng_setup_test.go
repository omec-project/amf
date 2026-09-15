// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package ngap

import (
	"encoding/hex"
	"testing"

	"github.com/omec-project/amf/context"
	"github.com/omec-project/ngap/v2/aper"
	"github.com/omec-project/ngap/v2/ngapConvert"
	"github.com/omec-project/ngap/v2/ngapType"
	"github.com/omec-project/openapi/v2/models"
)

// ngSetupRequestWithTACs builds an NG SETUP REQUEST carrying one Supported TA Item per TAC, each
// broadcasting the one PLMN this test's AMF serves.
//
// The RAN Node Name IE is left out on purpose. It is optional (TS 38.413 clause 9.2.6.1) and the
// handler copies it into ran.Name, which is the gauge's id label - so carrying it would let a test
// change the label it reads back.
func ngSetupRequestWithTACs(tacs ...string) *ngapType.NGAPPDU {
	list := ngapType.SupportedTAList{}

	for _, tac := range tacs {
		raw, err := hex.DecodeString(tac)
		if err != nil {
			panic("test TAC is not hex: " + tac)
		}

		item := ngapType.SupportedTAItem{TAC: ngapType.TAC{Value: raw}}
		item.BroadcastPLMNList.List = append(item.BroadcastPLMNList.List, ngapType.BroadcastPLMNItem{
			PLMNIdentity: ngapConvert.PlmnIdToNgap(models.PlmnId{Mcc: "208", Mnc: "93"}),
			TAISliceSupportList: ngapType.SliceSupportList{
				List: []ngapType.SliceSupportItem{
					{SNSSAI: ngapConvert.SNssaiToNgap(models.Snssai{Sst: 1})},
				},
			},
		})
		list.List = append(list.List, item)
	}

	globalRANNodeID := ngapType.GlobalRANNodeID{
		Present: ngapType.GlobalRANNodeIDPresentGlobalGNBID,
		GlobalGNBID: &ngapType.GlobalGNBID{
			PLMNIdentity: ngapConvert.PlmnIdToNgap(models.PlmnId{Mcc: "208", Mnc: "93"}),
			GNBID: ngapType.GNBID{
				Present: ngapType.GNBIDPresentGNBID,
				GNBID:   &aper.BitString{Bytes: []byte{0x45, 0x46, 0x47}, BitLength: 24},
			},
		},
	}

	return &ngapType.NGAPPDU{
		Present: ngapType.NGAPPDUPresentInitiatingMessage,
		InitiatingMessage: &ngapType.InitiatingMessage{
			ProcedureCode: ngapType.ProcedureCode{Value: ngapType.ProcedureCodeNGSetup},
			Value: ngapType.InitiatingMessageValue{
				Present: ngapType.InitiatingMessagePresentNGSetup,
				NGSetup: &ngapType.NGSetupRequest{
					ProtocolIEs: ngapType.ProtocolIEContainerNGSetupRequestIEs{
						List: []ngapType.NGSetupRequestIEs{
							{
								Id: ngapType.ProtocolIEID{Value: ngapType.ProtocolIEIDGlobalRANNodeID},
								Value: ngapType.NGSetupRequestIEsValue{
									Present:         ngapType.NGSetupRequestIEsPresentGlobalRANNodeID,
									GlobalRANNodeID: &globalRANNodeID,
								},
							},
							{
								Id: ngapType.ProtocolIEID{Value: ngapType.ProtocolIEIDSupportedTAList},
								Value: ngapType.NGSetupRequestIEsValue{
									Present:         ngapType.NGSetupRequestIEsPresentSupportedTAList,
									SupportedTAList: &list,
								},
							},
						},
					},
				},
			},
		},
	}
}

// HandleNGSetupRequest replaces the supported TA list, and has done since before the RAN
// configuration update path was taught to. The other half of that repair was missing here too:
// gnb_session_profile is written by SetRanStats, which walks the RAN's *current* list, so a
// tracking area dropped by a second NG Setup on one association is never visited again and keeps
// its last Connected sample for the life of the process - including through the disconnect, whose
// walk only sees the list that replaced it.
func TestARepeatedNgSetupRetiresTheTacsItDrops(t *testing.T) {
	disableKafkaForTest(t)
	serveTACs(t, "1", "2")

	ran := context.NewAmfRanDefault()
	// Every production constructor in context gives the RAN its list; the bare test helper
	// does not, and the append is guarded by that list's capacity - so without this the
	// handler records nothing and the test would pass for the wrong reason.
	ran.SupportedTAList = context.NewSupportedTAIList()
	ran.Name = "gnb-ng-setup-retire-test"
	ran.GnbIp = "198.51.100.11"

	HandleNGSetupRequest(ran, ngSetupRequestWithTACs("000001"))

	if got := tacsOf(ran.SupportedTAList); len(got) != 1 || got[0] != "000001" {
		t.Fatalf("after the first NG Setup the TA list is %v, want exactly [000001]", got)
	}

	// The response is what publishes the gauge, so an accepted setup is the precondition for
	// there being anything to retire.
	if _, published := gnbSessProfileTACs(t, ran.Name, ran.GnbIp)["000001"]; !published {
		t.Fatal("the gauge has no series for 000001, so this test cannot show one being retired")
	}

	// The gNB sets the interface up again, now broadcasting a different tracking area.
	HandleNGSetupRequest(ran, ngSetupRequestWithTACs("000002"))

	left := gnbSessProfileTACs(t, ran.Name, ran.GnbIp)

	if _, stale := left["000001"]; stale {
		t.Error("gnb_session_profile still carries a series for 000001 after the gNB stopped " +
			"broadcasting it, so exported state outlives the condition it describes")
	}

	if _, published := left["000002"]; !published {
		t.Error("gnb_session_profile has no series for 000002, which the gNB now broadcasts")
	}
}

// The list is replaced before the AMF decides whether it can serve any of it, so a refused setup
// leaves the same orphans behind - and it is the case where nothing later writes the gauge at all.
// This is what pins the retirement to the list being replaced rather than to the response.
func TestARefusedNgSetupRetiresTheTacsItDropped(t *testing.T) {
	disableKafkaForTest(t)
	serveTACs(t, "1")

	ran := context.NewAmfRanDefault()
	ran.SupportedTAList = context.NewSupportedTAIList()
	ran.Name = "gnb-ng-setup-refused-test"
	ran.GnbIp = "198.51.100.12"

	HandleNGSetupRequest(ran, ngSetupRequestWithTACs("000001"))

	if _, published := gnbSessProfileTACs(t, ran.Name, ran.GnbIp)["000001"]; !published {
		t.Fatal("the gauge has no series for 000001, so this test cannot show one being retired")
	}

	// 000002 is not in this AMF's served TAI list, so the setup is refused - after the list has
	// already been replaced.
	HandleNGSetupRequest(ran, ngSetupRequestWithTACs("000002"))

	if got := tacsOf(ran.SupportedTAList); len(got) != 1 || got[0] != "000002" {
		t.Fatalf("after the refused NG Setup the TA list is %v, want exactly [000002] - this test "+
			"rests on the list being replaced before the AMF refuses", got)
	}

	if _, stale := gnbSessProfileTACs(t, ran.Name, ran.GnbIp)["000001"]; stale {
		t.Error("gnb_session_profile still carries a series for 000001 after a refused NG Setup " +
			"dropped it from the list, where nothing will ever write that series again")
	}
}
