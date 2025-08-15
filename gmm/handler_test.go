// Copyright 2019 free5GC.org
// SPDX-FileCopyrightText: 2025 Canonical Ltd.
//
// SPDX-License-Identifier: Apache-2.0
//

package gmm

import (
	ctxt "context"
	"strings"
	"testing"

	"github.com/omec-project/amf/context"
	"github.com/omec-project/amf/util"
	"github.com/omec-project/nas/nasMessage"
	"github.com/omec-project/nas/nasType"
	"github.com/omec-project/openapi/models"
	"go.uber.org/zap"
)

func newUE() *context.AmfUe {
	return &context.AmfUe{
		GmmLog:       zap.NewNop().Sugar(),
		RanUe:        make(map[models.AccessType]*context.RanUe),
		AllowedNssai: map[models.AccessType][]models.AllowedSnssai{},
	}
}

func emptyUL() *nasMessage.ULNASTransport {
	return &nasMessage.ULNASTransport{}
}

func withPduID(ul *nasMessage.ULNASTransport, id uint8) *nasMessage.ULNASTransport {
	ul.PduSessionID2Value = &nasType.PduSessionID2Value{}
	ul.SetPduSessionID2Value(id)
	return ul
}

func withOldPduID(ul *nasMessage.ULNASTransport, id uint8) *nasMessage.ULNASTransport {
	old := nasType.NewOldPDUSessionID(0)
	old.SetOldPDUSessionID(id)
	ul.OldPDUSessionID = old
	return ul
}

func withReqType(ul *nasMessage.ULNASTransport, v uint8) *nasMessage.ULNASTransport {
	rt := &nasType.RequestType{}
	rt.SetRequestTypeValue(v)
	ul.RequestType = rt
	return ul
}

func Test_PduSessionIDFromUL(t *testing.T) {
	if _, err := pduSessionIDFromUL(&nasMessage.ULNASTransport{}); err == nil {
		t.Error("expected error for nil PduSessionID2Value")
	}

	ul := withPduID(emptyUL(), 9)
	id, err := pduSessionIDFromUL(ul)
	if err != nil || id != 9 {
		t.Errorf("got id=%d err=%v; want id=9 nil", id, err)
	}
}

func Test_IsEmergencyRequestAndIsInitialRequest(t *testing.T) {
	if isEmergencyRequest(nil) {
		t.Error("nil request type should not be emergency")
	}
	rt := &nasType.RequestType{}
	rt.SetRequestTypeValue(nasMessage.ULNASTransportRequestTypeInitialEmergencyRequest)
	if !isEmergencyRequest(rt) {
		t.Error("initial emergency should be emergency")
	}
	rt.SetRequestTypeValue(nasMessage.ULNASTransportRequestTypeExistingEmergencyPduSession)
	if !isEmergencyRequest(rt) {
		t.Error("existing emergency should be emergency")
	}
	rt.SetRequestTypeValue(nasMessage.ULNASTransportRequestTypeInitialRequest)
	if !isInitialRequest(rt) {
		t.Error("initial request should be initial")
	}
	rt.SetRequestTypeValue(nasMessage.ULNASTransportRequestTypeModificationRequest)
	if isInitialRequest(rt) {
		t.Error("modification is not initial")
	}
}

func Test_SendNotForwarded(t *testing.T) {
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

	ue := newUE()
	sendNotForwarded(ue, models.AccessType__3_GPP_ACCESS, nil, 7)
	if calls != 0 {
		t.Errorf("expected 0 calls; got %d", calls)
	}

	ue.RanUe[models.AccessType__3_GPP_ACCESS] = &context.RanUe{AmfUe: ue}
	sendNotForwarded(ue, models.AccessType__3_GPP_ACCESS, []byte{0x01}, 7)
	if calls != 1 {
		t.Errorf("expected 1 call; got %d", calls)
	}
	if got.an != models.AccessType__3_GPP_ACCESS || got.id != 7 || got.cause != nasMessage.Cause5GMMPayloadWasNotForwarded {
		t.Errorf("unexpected args: an=%v id=%d cause=%d", got.an, got.id, got.cause)
	}
}

func Test_PickSnssai(t *testing.T) {
	ue := newUE()
	ul := withPduID(emptyUL(), 1)
	ul.SNSSAI = &nasType.SNSSAI{}
	ul.SNSSAI.SetLen(4)
	ul.SNSSAI.SetIei(0)
	ul.SetSST(1)
	ul.SetSD([3]uint8([]byte{0x11, 0x22, 0x33}))
	got, err := pickSnssai(ul, ue, models.AccessType__3_GPP_ACCESS)
	if err != nil || got.Sst != 1 || got.Sd != "112233" {
		t.Errorf("pickSnssai (UL) got=%+v err=%v; want sst=1 sd=112233", got, err)
	}

	ul.SNSSAI = nil
	ue.AllowedNssai[models.AccessType__3_GPP_ACCESS] = []models.AllowedSnssai{
		{AllowedSnssai: &models.Snssai{Sst: 2, Sd: "010203"}},
	}
	got, err = pickSnssai(ul, ue, models.AccessType__3_GPP_ACCESS)
	if err != nil || got.Sst != 2 || got.Sd != "010203" {
		t.Errorf("pickSnssai (Allowed) got=%+v err=%v; want sst=2 sd=010203", got, err)
	}

	delete(ue.AllowedNssai, models.AccessType__3_GPP_ACCESS)
	_, err = pickSnssai(ul, ue, models.AccessType__3_GPP_ACCESS)
	if err == nil {
		t.Error("expected error when allowedNssai missing")
	}
}

func Test_PickDNN(t *testing.T) {
	ue := newUE()

	sn := models.Snssai{Sst: 1, Sd: "112233"}

	ue.ServingAMF = &context.AMFContext{SupportDnnLists: []string{"internet-1", "ims"}}
	if d := pickDNN(&nasMessage.ULNASTransport{}, ue, sn); d != "internet-1" {
		t.Errorf("pickDNN (support list) got=%q; want %q", d, "internet-1")
	}

	ue.ServingAMF = &context.AMFContext{SupportDnnLists: []string{}}
	if d := pickDNN(&nasMessage.ULNASTransport{}, ue, sn); d != "internet" {
		t.Errorf("pickDNN (default) got=%q; want %q", d, "internet")
	}

	key := util.SnssaiModelsToHex(sn)
	ue.SmfSelectionData = &models.SmfSelectionSubscriptionData{
		SubscribedSnssaiInfos: map[string]models.SnssaiInfo{
			key: {
				DnnInfos: []models.DnnInfo{
					{Dnn: "ims", DefaultDnnIndicator: true},
					{Dnn: "internet-x", DefaultDnnIndicator: false},
				},
			},
		},
	}

	ue.ServingAMF = &context.AMFContext{SupportDnnLists: []string{"internet-1"}}

	if d := pickDNN(&nasMessage.ULNASTransport{}, ue, sn); d != "ims" {
		t.Errorf("pickDNN (smf selection) got=%q; want %q", d, "ims")
	}
}

func Test_Transport5GSMMessage(t *testing.T) {
	causeNotFwd := nasMessage.Cause5GMMPayloadWasNotForwarded

	type tc struct {
		name       string
		ul         *nasMessage.ULNASTransport
		an         models.AccessType
		prepareUE  func(*context.AmfUe)
		wantErrSub string
		wantNilErr bool
		wantCalls  int
		wantCause  *uint8
	}
	tests := []tc{
		{
			name:       "missing PDU ID: error, no DL",
			ul:         emptyUL(),
			an:         models.AccessType__3_GPP_ACCESS,
			wantErrSub: "pdu session id is nil",
			wantCalls:  0,
		},
		{
			name:       "Old PDU ID (SSC mode 3): DL not forwarded + error",
			ul:         withOldPduID(withPduID(emptyUL(), 10), 9),
			an:         models.AccessType__3_GPP_ACCESS,
			wantErrSub: "ssc mode3 operation has not been implemented",
			prepareUE: func(ue *context.AmfUe) {
				ue.RanUe[models.AccessType__3_GPP_ACCESS] = &context.RanUe{AmfUe: ue}
			},
			wantCalls: 1, wantCause: &causeNotFwd,
		},
		{
			name: "no SM context + RequestType=nil: no-op, nil error, no DL",
			ul:   withPduID(emptyUL(), 1),
			an:   models.AccessType_NON_3_GPP_ACCESS,
			prepareUE: func(ue *context.AmfUe) {
				ue.RanUe[models.AccessType_NON_3_GPP_ACCESS] = &context.RanUe{AmfUe: ue}
			},
			wantNilErr: true, wantCalls: 0,
		},
		{
			name: "emergency -> DL not forwarded: nil error",
			ul:   withReqType(withPduID(emptyUL(), 2), nasMessage.ULNASTransportRequestTypeInitialEmergencyRequest),
			an:   models.AccessType__3_GPP_ACCESS,
			prepareUE: func(ue *context.AmfUe) {
				ue.RanUe[models.AccessType__3_GPP_ACCESS] = &context.RanUe{AmfUe: ue}
			},
			wantNilErr: true, wantCalls: 1, wantCause: &causeNotFwd,
		},
		{
			name: "existing PDU session but S-NSSAI not allowed: DL not forwarded",
			ul:   withReqType(withPduID(emptyUL(), 3), nasMessage.ULNASTransportRequestTypeExistingPduSession),
			an:   models.AccessType__3_GPP_ACCESS,
			prepareUE: func(ue *context.AmfUe) {
				ue.RanUe[models.AccessType__3_GPP_ACCESS] = &context.RanUe{AmfUe: ue}
				sm := context.NewSmContext(3)
				sm.SetAccessType(models.AccessType__3_GPP_ACCESS)
				ue.StoreSmContext(3, sm)
			},
			wantNilErr: true, wantCalls: 1, wantCause: &causeNotFwd,
		},
		{
			name:       "emergency but no RanUe: guarded (no DL), nil error",
			ul:         withReqType(withPduID(emptyUL(), 4), nasMessage.ULNASTransportRequestTypeExistingEmergencyPduSession),
			an:         models.AccessType__3_GPP_ACCESS,
			wantNilErr: true, wantCalls: 0,
		},
		{
			name: "initial request with no allowed NSSAI: error, no DL",
			ul:   withReqType(withPduID(emptyUL(), 5), nasMessage.ULNASTransportRequestTypeInitialRequest),
			an:   models.AccessType__3_GPP_ACCESS,
			prepareUE: func(ue *context.AmfUe) {
				ue.RanUe[models.AccessType__3_GPP_ACCESS] = &context.RanUe{AmfUe: ue}
			},
			wantErrSub: "ue doesn't have allowedNssai",
			wantCalls:  0,
		},
		{
			name: "modification request with nil UeContextInSmfData: DL not forwarded",
			ul:   withReqType(withPduID(emptyUL(), 6), nasMessage.ULNASTransportRequestTypeModificationRequest),
			an:   models.AccessType__3_GPP_ACCESS,
			prepareUE: func(ue *context.AmfUe) {
				ue.RanUe[models.AccessType__3_GPP_ACCESS] = &context.RanUe{AmfUe: ue}
				ue.UeContextInSmfData = nil
			},
			wantNilErr: true, wantCalls: 1, wantCause: &causeNotFwd,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orig := sendDLNASTransport
			defer func() { sendDLNASTransport = orig }()
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

			ue := newUE()
			if tt.prepareUE != nil {
				tt.prepareUE(ue)
			}

			err := transport5GSMMessage(ctxt.Background(), ue, tt.an, tt.ul)

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
