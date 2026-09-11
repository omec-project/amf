// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package ngap

import (
	ctxt "context"
	"testing"

	"github.com/omec-project/amf/context"
	"github.com/omec-project/amf/logger"
	"github.com/omec-project/ngap/v2/ngapType"
	"github.com/omec-project/openapi/v2/models"
	"github.com/prometheus/client_golang/prometheus"
)

// unknownSmContextCount reads one label's value out of the default registry. The gathered types
// are used through their accessors only, so this needs no new module dependency.
func unknownSmContextCount(t *testing.T, message string) float64 {
	t.Helper()

	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("Gather() = %v", err)
	}

	for _, family := range families {
		if family.GetName() != "amf_ngap_unknown_sm_context_total" {
			continue
		}

		for _, metric := range family.GetMetric() {
			for _, label := range metric.GetLabel() {
				if label.GetName() == "message" && label.GetValue() == message {
					return metric.GetCounter().GetValue()
				}
			}
		}
	}

	return 0
}

func setupResponseNaming(ranUe *context.RanUe, pduSessionIDs ...int64) *ngapType.NGAPPDU {
	list := ngapType.PDUSessionResourceSetupListSURes{}
	for _, id := range pduSessionIDs {
		list.List = append(list.List, ngapType.PDUSessionResourceSetupItemSURes{
			PDUSessionID:                            ngapType.PDUSessionID{Value: id},
			PDUSessionResourceSetupResponseTransfer: []byte{},
		})
	}

	return &ngapType.NGAPPDU{
		Present: ngapType.NGAPPDUPresentSuccessfulOutcome,
		SuccessfulOutcome: &ngapType.SuccessfulOutcome{
			ProcedureCode: ngapType.ProcedureCode{Value: ngapType.ProcedureCodePDUSessionResourceSetup},
			Value: ngapType.SuccessfulOutcomeValue{
				Present: ngapType.SuccessfulOutcomePresentPDUSessionResourceSetup,
				PDUSessionResourceSetup: &ngapType.PDUSessionResourceSetupResponse{
					ProtocolIEs: ngapType.ProtocolIEContainerPDUSessionResourceSetupResponseIEs{
						List: []ngapType.PDUSessionResourceSetupResponseIEs{
							{
								Id: ngapType.ProtocolIEID{Value: ngapType.ProtocolIEIDAMFUENGAPID},
								Value: ngapType.PDUSessionResourceSetupResponseIEsValue{
									Present:     ngapType.PDUSessionResourceSetupResponseIEsPresentAMFUENGAPID,
									AMFUENGAPID: &ngapType.AMFUENGAPID{Value: ranUe.AmfUeNgapId},
								},
							},
							{
								Id: ngapType.ProtocolIEID{Value: ngapType.ProtocolIEIDRANUENGAPID},
								Value: ngapType.PDUSessionResourceSetupResponseIEsValue{
									Present:     ngapType.PDUSessionResourceSetupResponseIEsPresentRANUENGAPID,
									RANUENGAPID: &ngapType.RANUENGAPID{Value: ranUe.RanUeNgapId},
								},
							},
							{
								Id: ngapType.ProtocolIEID{Value: ngapType.ProtocolIEIDPDUSessionResourceSetupListSURes},
								Value: ngapType.PDUSessionResourceSetupResponseIEsValue{
									Present:                          ngapType.PDUSessionResourceSetupResponseIEsPresentPDUSessionResourceSetupListSURes,
									PDUSessionResourceSetupListSURes: &list,
								},
							},
						},
					},
				},
			},
		},
	}
}

// The unit of this defect is the session, not the message: one setup response can name several
// PDU sessions and only some of them may be unresolvable. A counter incremented per message
// could not express "two of the sessions in this message were stranded", which is why this is
// its own metric rather than a result label on ngap_messages_total.
func TestSetupResponseCountsSessionsRatherThanMessages(t *testing.T) {
	disableKafkaForTest(t)

	self := context.AMF_Self()

	ran := context.NewAmfRanDefault()
	ran.AnType = models.ACCESSTYPE__3_GPP_ACCESS

	ranUe, err := ran.NewRanUe(701)
	if err != nil {
		t.Fatalf("NewRanUe() = %v", err)
	}
	ranUe.Log = logger.NgapLog

	amfUe := self.NewAmfUe("")
	amfUe.AttachRanUe(ranUe)

	// The pools are global to the package, so what this test creates has to leave with it -
	// both the RanUe and its AMF UE NGAP ID, which the generator would otherwise keep issued.
	t.Cleanup(func() {
		if leftover := self.RanUeFindByAmfUeNgapIDLocal(ranUe.AmfUeNgapId); leftover != nil {
			if err := leftover.Remove(); err != nil {
				t.Errorf("cleanup RanUe failed: %v", err)
			}
		}
		amfUe.Remove()
	})

	before := unknownSmContextCount(t, "PDUSessionResourceSetupResponse")

	// Two sessions, neither of which this AMF has an SM context for.
	HandlePDUSessionResourceSetupResponse(ctxt.Background(), ran, setupResponseNaming(ranUe, 1, 2))

	if got := unknownSmContextCount(t, "PDUSessionResourceSetupResponse") - before; got != 2 {
		t.Errorf("counter rose by %v for one message naming two unresolvable sessions, want 2", got)
	}
}

// The condition being recorded is a missing pointer that used to be dropped silently. The
// recording must not become a second one.
func TestRecordUnknownSmContextSurvivesANilRanUe(t *testing.T) {
	before := unknownSmContextCount(t, "PathSwitchRequest")

	recordUnknownSmContext(nil, "PathSwitchRequest", 9)

	if got := unknownSmContextCount(t, "PathSwitchRequest") - before; got != 1 {
		t.Errorf("counter rose by %v with a nil RanUe, want 1", got)
	}
}

func releaseResponseNaming(ranUe *context.RanUe, pduSessionID int64) *ngapType.NGAPPDU {
	list := ngapType.PDUSessionResourceReleasedListRelRes{
		List: []ngapType.PDUSessionResourceReleasedItemRelRes{
			{
				PDUSessionID: ngapType.PDUSessionID{Value: pduSessionID},
				PDUSessionResourceReleaseResponseTransfer: []byte{},
			},
		},
	}

	return &ngapType.NGAPPDU{
		Present: ngapType.NGAPPDUPresentSuccessfulOutcome,
		SuccessfulOutcome: &ngapType.SuccessfulOutcome{
			ProcedureCode: ngapType.ProcedureCode{Value: ngapType.ProcedureCodePDUSessionResourceRelease},
			Value: ngapType.SuccessfulOutcomeValue{
				Present: ngapType.SuccessfulOutcomePresentPDUSessionResourceRelease,
				PDUSessionResourceRelease: &ngapType.PDUSessionResourceReleaseResponse{
					ProtocolIEs: ngapType.ProtocolIEContainerPDUSessionResourceReleaseResponseIEs{
						List: []ngapType.PDUSessionResourceReleaseResponseIEs{
							{
								Id: ngapType.ProtocolIEID{Value: ngapType.ProtocolIEIDAMFUENGAPID},
								Value: ngapType.PDUSessionResourceReleaseResponseIEsValue{
									Present:     ngapType.PDUSessionResourceReleaseResponseIEsPresentAMFUENGAPID,
									AMFUENGAPID: &ngapType.AMFUENGAPID{Value: ranUe.AmfUeNgapId},
								},
							},
							{
								Id: ngapType.ProtocolIEID{Value: ngapType.ProtocolIEIDRANUENGAPID},
								Value: ngapType.PDUSessionResourceReleaseResponseIEsValue{
									Present:     ngapType.PDUSessionResourceReleaseResponseIEsPresentRANUENGAPID,
									RANUENGAPID: &ngapType.RANUENGAPID{Value: ranUe.RanUeNgapId},
								},
							},
							{
								Id: ngapType.ProtocolIEID{Value: ngapType.ProtocolIEIDPDUSessionResourceReleasedListRelRes},
								Value: ngapType.PDUSessionResourceReleaseResponseIEsValue{
									Present:                              ngapType.PDUSessionResourceReleaseResponseIEsPresentPDUSessionResourceReleasedListRelRes,
									PDUSessionResourceReleasedListRelRes: &list,
								},
							},
						},
					},
				},
			},
		},
	}
}

// The release shape has the opposite consequence to the setup shape: here the RAN has
// already let the session go, so what must be discoverable is an SMF still holding one. Driving the handler
// rather than the recorder is the point — the defect was that this path reached no recorder
// at all.
func TestReleaseResponseIdentifiesASessionTheAmfCannotResolve(t *testing.T) {
	disableKafkaForTest(t)

	self := context.AMF_Self()

	ran := context.NewAmfRanDefault()
	ran.AnType = models.ACCESSTYPE__3_GPP_ACCESS

	ranUe, err := ran.NewRanUe(702)
	if err != nil {
		t.Fatalf("NewRanUe() = %v", err)
	}
	ranUe.Log = logger.NgapLog

	amfUe := self.NewAmfUe("")
	amfUe.AttachRanUe(ranUe)

	// The pools are global to the package, so what this test creates has to leave with it -
	// both the RanUe and its AMF UE NGAP ID, which the generator would otherwise keep issued.
	t.Cleanup(func() {
		if leftover := self.RanUeFindByAmfUeNgapIDLocal(ranUe.AmfUeNgapId); leftover != nil {
			if err := leftover.Remove(); err != nil {
				t.Errorf("cleanup RanUe failed: %v", err)
			}
		}
		amfUe.Remove()
	})

	before := unknownSmContextCount(t, "PDUSessionResourceReleaseResponse")

	HandlePDUSessionResourceReleaseResponse(ctxt.Background(), ran, releaseResponseNaming(ranUe, 5))

	if got := unknownSmContextCount(t, "PDUSessionResourceReleaseResponse") - before; got != 1 {
		t.Errorf("counter rose by %v for a release response naming one unresolvable session, want 1", got)
	}
}

func notifyNaming(ranUe *context.RanUe, pduSessionID int64) *ngapType.NGAPPDU {
	list := ngapType.PDUSessionResourceNotifyList{
		List: []ngapType.PDUSessionResourceNotifyItem{
			{
				PDUSessionID:                     ngapType.PDUSessionID{Value: pduSessionID},
				PDUSessionResourceNotifyTransfer: []byte{},
			},
		},
	}

	return &ngapType.NGAPPDU{
		Present: ngapType.NGAPPDUPresentInitiatingMessage,
		InitiatingMessage: &ngapType.InitiatingMessage{
			ProcedureCode: ngapType.ProcedureCode{Value: ngapType.ProcedureCodePDUSessionResourceNotify},
			Value: ngapType.InitiatingMessageValue{
				Present: ngapType.InitiatingMessagePresentPDUSessionResourceNotify,
				PDUSessionResourceNotify: &ngapType.PDUSessionResourceNotify{
					ProtocolIEs: ngapType.ProtocolIEContainerPDUSessionResourceNotifyIEs{
						List: []ngapType.PDUSessionResourceNotifyIEs{
							{
								Id: ngapType.ProtocolIEID{Value: ngapType.ProtocolIEIDAMFUENGAPID},
								Value: ngapType.PDUSessionResourceNotifyIEsValue{
									Present:     ngapType.PDUSessionResourceNotifyIEsPresentAMFUENGAPID,
									AMFUENGAPID: &ngapType.AMFUENGAPID{Value: ranUe.AmfUeNgapId},
								},
							},
							{
								Id: ngapType.ProtocolIEID{Value: ngapType.ProtocolIEIDRANUENGAPID},
								Value: ngapType.PDUSessionResourceNotifyIEsValue{
									Present:     ngapType.PDUSessionResourceNotifyIEsPresentRANUENGAPID,
									RANUENGAPID: &ngapType.RANUENGAPID{Value: ranUe.RanUeNgapId},
								},
							},
							{
								Id: ngapType.ProtocolIEID{Value: ngapType.ProtocolIEIDPDUSessionResourceNotifyList},
								Value: ngapType.PDUSessionResourceNotifyIEsValue{
									Present:                      ngapType.PDUSessionResourceNotifyIEsPresentPDUSessionResourceNotifyList,
									PDUSessionResourceNotifyList: &list,
								},
							},
						},
					},
				},
			},
		},
	}
}

// Seven of the sixteen sites recorded the missing context and then handed it to the session
// management function consumer anyway. Its first act is to read the SMF's URI out of the
// context, so the nil pointer ended the process - and the AMF's NGAP paths run with no
// recover(), which means every UE on every gNB, not just this session. The recording is worth
// little if the AMF does not survive to be scraped.
func TestNotifyForAnUnresolvableSessionDoesNotEndTheProcess(t *testing.T) {
	disableKafkaForTest(t)

	self := context.AMF_Self()

	ran := context.NewAmfRanDefault()
	ran.AnType = models.ACCESSTYPE__3_GPP_ACCESS

	ranUe, err := ran.NewRanUe(703)
	if err != nil {
		t.Fatalf("NewRanUe() = %v", err)
	}
	ranUe.Log = logger.NgapLog

	amfUe := self.NewAmfUe("")
	amfUe.AttachRanUe(ranUe)

	// The pools are global to the package, so what this test creates has to leave with it -
	// both the RanUe and its AMF UE NGAP ID, which the generator would otherwise keep issued.
	t.Cleanup(func() {
		if leftover := self.RanUeFindByAmfUeNgapIDLocal(ranUe.AmfUeNgapId); leftover != nil {
			if err := leftover.Remove(); err != nil {
				t.Errorf("cleanup RanUe failed: %v", err)
			}
		}
		amfUe.Remove()
	})

	before := unknownSmContextCount(t, "PDUSessionResourceNotify")

	// A panic here fails the test by unwinding it; the assertion below is for the quieter
	// failure where the session is skipped without being recorded.
	HandlePDUSessionResourceNotify(ctxt.Background(), ran, notifyNaming(ranUe, 7))

	if got := unknownSmContextCount(t, "PDUSessionResourceNotify") - before; got != 1 {
		t.Errorf("counter rose by %v for a notify naming one unresolvable session, want 1", got)
	}
}
