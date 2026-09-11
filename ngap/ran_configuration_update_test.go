// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package ngap

import (
	"encoding/hex"
	"testing"

	"github.com/omec-project/amf/context"
	"github.com/omec-project/ngap/v2/ngapConvert"
	"github.com/omec-project/ngap/v2/ngapType"
	"github.com/omec-project/openapi/v2/models"
	"github.com/prometheus/client_golang/prometheus"
)

// ranConfigurationUpdateWithTACs builds a RAN CONFIGURATION UPDATE carrying one Supported TA
// Item per TAC, each broadcasting the one PLMN this test's AMF serves.
func ranConfigurationUpdateWithTACs(tacs ...string) *ngapType.NGAPPDU {
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

	return &ngapType.NGAPPDU{
		Present: ngapType.NGAPPDUPresentInitiatingMessage,
		InitiatingMessage: &ngapType.InitiatingMessage{
			ProcedureCode: ngapType.ProcedureCode{Value: ngapType.ProcedureCodeRANConfigurationUpdate},
			Value: ngapType.InitiatingMessageValue{
				Present: ngapType.InitiatingMessagePresentRANConfigurationUpdate,
				RANConfigurationUpdate: &ngapType.RANConfigurationUpdate{
					ProtocolIEs: ngapType.ProtocolIEContainerRANConfigurationUpdateIEs{
						List: []ngapType.RANConfigurationUpdateIEs{
							{
								Id: ngapType.ProtocolIEID{Value: ngapType.ProtocolIEIDSupportedTAList},
								Value: ngapType.RANConfigurationUpdateIEsValue{
									Present:         ngapType.RANConfigurationUpdateIEsPresentSupportedTAList,
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

// serveTACs makes the AMF serve the given tracking areas for the duration of one test, so an
// update naming them takes the acknowledged path. Clause 8.7.2.2's "shall overwrite" is in
// Successful Operation, so a test that pins the replacement against a refused update would be
// pinning something the clause does not say.
func serveTACs(t *testing.T, tacs ...string) {
	t.Helper()

	self := context.AMF_Self()
	previous := self.SupportTaiLists

	t.Cleanup(func() { self.SupportTaiLists = previous })

	served := make([]models.Tai, 0, len(tacs))
	for _, tac := range tacs {
		served = append(served, models.Tai{
			PlmnId: models.PlmnId{Mcc: "208", Mnc: "93"},
			Tac:    tac,
		})
	}

	self.SupportTaiLists = served
}

func tacsOf(list []context.SupportedTAI) []string {
	tacs := make([]string, 0, len(list))
	for _, supported := range list {
		tacs = append(tacs, supported.Tai.Tac)
	}

	return tacs
}

// TS 38.413 clause 8.7.2.2: when the Supported TA List IE is included in a RAN CONFIGURATION
// UPDATE, the AMF shall overwrite the whole list of supported TAs and the slices of each.
// This path appended instead, where HandleNGSetupRequest clears first, so every update grew
// the list: a tracking area the gNB had stopped broadcasting stayed in the AMF's idea of what
// this RAN serves, and once the list reached its capacity a genuinely new one was dropped in
// silence.
func TestRanConfigurationUpdateReplacesTheSupportedTaList(t *testing.T) {
	serveTACs(t, "1", "2")

	ran := context.NewAmfRanDefault()
	// Every production constructor in context gives the RAN its list; the bare test helper
	// does not, and the append is guarded by that list's capacity - so without this the
	// handler records nothing and the test would pass for the wrong reason.
	ran.SupportedTAList = context.NewSupportedTAIList()

	HandleRanConfigurationUpdate(ran, ranConfigurationUpdateWithTACs("000001"))

	if got := tacsOf(ran.SupportedTAList); len(got) != 1 || got[0] != "000001" {
		t.Fatalf("after the first update the TA list is %v, want exactly [000001]", got)
	}

	// The gNB now broadcasts a different tracking area and has stopped broadcasting the
	// first. Both facts have to land, and dropping the old one is the half that was missing.
	HandleRanConfigurationUpdate(ran, ranConfigurationUpdateWithTACs("000002"))

	got := tacsOf(ran.SupportedTAList)
	if len(got) != 1 || got[0] != "000002" {
		t.Errorf("after the second update the TA list is %v, want exactly [000002] - the list is "+
			"overwritten, not added to", got)
	}
}

// The capacity is 16 TAIs x 12 broadcast PLMNs, and the append is guarded by it, so an
// accumulating list does not grow without bound - it fills up and then silently discards
// what arrives next. That is the failure this protects against on a long-lived association.
func TestRepeatedRanConfigurationUpdatesDoNotFillTheTaList(t *testing.T) {
	serveTACs(t, "1")

	ran := context.NewAmfRanDefault()
	// Every production constructor in context gives the RAN its list; the bare test helper
	// does not, and the append is guarded by that list's capacity - so without this the
	// handler records nothing and the test would pass for the wrong reason.
	ran.SupportedTAList = context.NewSupportedTAIList()

	for range 200 {
		HandleRanConfigurationUpdate(ran, ranConfigurationUpdateWithTACs("000001"))
	}

	if got := tacsOf(ran.SupportedTAList); len(got) != 1 {
		t.Errorf("after 200 updates the TA list holds %d entries (%v), want 1", len(got), got)
	}
}

// gnbSessProfileTACs reads back which tracking areas still have a gnb_session_profile series
// for one gNB, in either state. A gauge that is only ever Set holds its last sample once
// nothing writes it again, so "is the series still there" is the question, not "what does it
// say". The id and ip labels are part of the query because the registry is global to the
// package: other tests in it publish this gauge for their own gNBs, and one of them uses the
// same TAC.
func gnbSessProfileTACs(t *testing.T, id, ip string) map[string]struct{} {
	t.Helper()

	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("Gather() = %v", err)
	}

	tacs := map[string]struct{}{}

	for _, family := range families {
		if family.GetName() != "gnb_session_profile" {
			continue
		}

		for _, metric := range family.GetMetric() {
			labels := map[string]string{}
			for _, label := range metric.GetLabel() {
				labels[label.GetName()] = label.GetValue()
			}

			if labels["id"] == id && labels["ip"] == ip {
				tacs[labels["tac"]] = struct{}{}
			}
		}
	}

	return tacs
}

// Replacing the list is only half of the repair. gnb_session_profile is written by SetRanStats,
// which walks the RAN's current list, and its only callers are the connect and disconnect
// transitions - so a tracking area dropped by an update is never visited again. Before the
// list was replaced at all, the departing TAC was still in the list and the disconnect walk
// eventually zeroed it; replacing the list without retiring the series would have turned a
// stale sample into a permanent one, which is exactly what this change must not do.
func TestARetiredTrackingAreaLeavesTheGauge(t *testing.T) {
	serveTACs(t, "1", "2")

	ran := context.NewAmfRanDefault()
	ran.SupportedTAList = context.NewSupportedTAIList()
	ran.Name = "gnb-retire-test"
	ran.GnbIp = "198.51.100.7"

	HandleRanConfigurationUpdate(ran, ranConfigurationUpdateWithTACs("000001"))

	// The update path does not announce; the connect transition is what publishes the series,
	// as it does after an NG Setup Response.
	ran.SetRanStats(context.RanConnected)

	if _, published := gnbSessProfileTACs(t, ran.Name, ran.GnbIp)["000001"]; !published {
		t.Fatal("the gauge has no series for 000001, so this test cannot show one being retired")
	}

	HandleRanConfigurationUpdate(ran, ranConfigurationUpdateWithTACs("000002"))

	if _, stale := gnbSessProfileTACs(t, ran.Name, ran.GnbIp)["000001"]; stale {
		t.Error("gnb_session_profile still carries a series for 000001 after the gNB stopped " +
			"broadcasting it, so exported state outlives the condition it describes")
	}
}
