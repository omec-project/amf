// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"encoding/json"
	"testing"

	ngaputil "github.com/omec-project/amf/ngap/util"
	"github.com/omec-project/ngap/v2/aper"
	"github.com/omec-project/ngap/v2/ngapConvert"
	"github.com/omec-project/ngap/v2/ngapType"
	"github.com/omec-project/openapi/v2/models"
)

// newDirectSctpRan builds a RAN the way an AMF that terminates SCTP itself does: NewAmfRan
// keys AmfRanPool by the net.Conn, and SetRanId -- which the NG Setup handler calls -- is
// what populates GnbId. Driving both is the point: GnbId is present on this path, it is
// just not the key the pool is indexed by.
func newDirectSctpRan(t *testing.T, gnbValue []byte) *AmfRan {
	t.Helper()

	ran := AMF_Self().NewAmfRan(&ngaputil.TestConn{})
	globalRANNodeID := &ngapType.GlobalRANNodeID{
		Present: ngapType.GlobalRANNodeIDPresentGlobalGNBID,
		GlobalGNBID: &ngapType.GlobalGNBID{
			PLMNIdentity: ngapConvert.PlmnIdToNgap(models.PlmnId{Mcc: "208", Mnc: "93"}),
			GNBID: ngapType.GNBID{
				Present: ngapType.GNBIDPresentGNBID,
				GNBID:   &aper.BitString{Bytes: gnbValue, BitLength: 24},
			},
		},
	}
	if err := ran.SetRanId(globalRANNodeID); err != nil {
		t.Fatalf("SetRanId: %v", err)
	}
	if ran.GnbId == "" {
		t.Fatal("SetRanId left GnbId empty, so this fixture proves nothing")
	}

	return ran
}

// A UE context restored from the datastore has to re-attach to its RAN by the GnbId string
// it stored. On the direct-SCTP path AmfRanPool is keyed by net.Conn, so an exact-key lookup
// can never find that RAN and every restored context comes back with a nil Ran -- about two
// warnings per UE, which is what made this visible at scale.
//
// Mutation check: with the range in AmfRanFindByGnbId removed, this fails at "restored
// context has no RAN".
func TestRestoredUeFindsItsRanInDirectSctpMode(t *testing.T) {
	self := AMF_Self()
	ran := newDirectSctpRan(t, []byte{0x45, 0x46, 0x47})
	t.Cleanup(func() { self.AmfRanPool.Delete(ran.Conn) })

	// Built directly rather than through NewAmfUe, as the other tests in this package do:
	// NewAmfUe allocates a GUTI and this binary loads no config, so ServedGuamiList is empty.
	ue := &AmfUe{}
	ue.init()
	ue.SetSupi("imsi-208930100009001")
	// A strict enum decode refuses an empty ScType, and DbFetch only works around that on
	// its retry path. Give the round-trip a valid one so this test exercises the RAN lookup
	// rather than the decoder.
	ue.NgKsi = models.NgKsi{Tsc: models.SCTYPE_NATIVE, Ksi: 0}

	ranUe, err := ran.NewRanUe(11)
	if err != nil {
		t.Fatalf("NewRanUe: %v", err)
	}
	ue.AttachRanUe(ranUe)
	t.Cleanup(func() { self.RanUePool.Delete(ranUe.AmfUeNgapId) })

	// The per-UE "Ran Connection is not Exist" warning is gated on exactly this lookup, so
	// asserting it succeeds is asserting the warning is not emitted for the normal case.
	if _, ok := self.AmfRanFindByGnbId(ran.GnbId); !ok {
		t.Fatalf("AmfRanFindByGnbId(%q) missed a natively-attached gNB", ran.GnbId)
	}

	stored, err := json.Marshal(ue)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	restored := &AmfUe{}
	restored.init()
	if err := json.Unmarshal(stored, restored); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	restoredRanUe := restored.RanUe[models.ACCESSTYPE__3_GPP_ACCESS]
	if restoredRanUe == nil {
		t.Fatal("restored context has no RanUe at all")
		return
	}
	// Pointer identity is the whole assertion. RanUe is serialised (json:"ranUe") and its
	// Ran field carries no tag, so the stored document holds a nested copy of the AmfRan and
	// a restore always comes back with a non-nil Ran. That copy has no Conn, no RanUeList
	// and no place in AmfRanPool -- writing to it reaches no gNB. Re-attaching means getting
	// the live pool object back, not a lookalike.
	if restoredRanUe.Ran == nil {
		t.Fatal("restored context has no RAN at all")
		return
	}
	if restoredRanUe.Ran != ran {
		t.Fatalf("restored context holds a detached copy of the RAN, not the live one "+
			"(copy: GnbId=%q Conn=%v; live: GnbId=%q)",
			restoredRanUe.Ran.GnbId, restoredRanUe.Ran.Conn, ran.GnbId)
	}
}

// An empty GnbId is the UE that was stored while it had no RAN at all (MarshalJSON guards
// that case and writes ""). Ranging the pool for "" would match any RAN that has not yet
// completed NG Setup, which is worse than not finding one.
func TestAmfRanFindByGnbIdDoesNotMatchOnEmptyId(t *testing.T) {
	self := AMF_Self()
	ran := self.NewAmfRan(&ngaputil.TestConn{}) // no SetRanId, so GnbId is ""
	t.Cleanup(func() { self.AmfRanPool.Delete(ran.Conn) })

	if found, ok := self.AmfRanFindByGnbId(""); ok {
		t.Fatalf("empty GnbId matched a RAN (GnbId=%q)", found.GnbId)
	}
}

// A GNBID that arrives on the CHOICE's extension arm leaves GNBValue empty and returns no
// error, so SetRanId builds "<mcc>:<mnc>:" and appends nothing. That id is non-empty but
// identifies no gNB -- every RAN on the PLMN that degenerates the same way shares it -- so
// ranging for it would attach a restored UE to an arbitrary one of them. Driven through
// SetRanId rather than by assigning GnbId, because the point is that the real writer
// produces this.
func TestAmfRanFindByGnbIdDoesNotMatchADegenerateId(t *testing.T) {
	self := AMF_Self()

	extensionArm := func() *ngapType.GlobalRANNodeID {
		return &ngapType.GlobalRANNodeID{
			Present: ngapType.GlobalRANNodeIDPresentGlobalGNBID,
			GlobalGNBID: &ngapType.GlobalGNBID{
				PLMNIdentity: ngapConvert.PlmnIdToNgap(models.PlmnId{Mcc: "208", Mnc: "93"}),
				GNBID:        ngapType.GNBID{Present: ngapType.GNBIDPresentChoiceExtensions},
			},
		}
	}

	first := self.NewAmfRan(&ngaputil.TestConn{})
	t.Cleanup(func() { self.AmfRanPool.Delete(first.Conn) })
	if err := first.SetRanId(extensionArm()); err != nil {
		t.Fatalf("SetRanId: %v", err)
	}
	second := self.NewAmfRan(&ngaputil.TestConn{})
	t.Cleanup(func() { self.AmfRanPool.Delete(second.Conn) })
	if err := second.SetRanId(extensionArm()); err != nil {
		t.Fatalf("SetRanId: %v", err)
	}

	if first.GnbId != second.GnbId {
		t.Fatalf("expected two RANs to share a degenerate GnbId, got %q and %q",
			first.GnbId, second.GnbId)
	}
	if first.GnbId == "" {
		t.Fatal("expected a non-empty degenerate GnbId; the empty guard would already cover \"\"")
	}

	if found, ok := self.AmfRanFindByGnbId(first.GnbId); ok {
		t.Fatalf("degenerate GnbId %q matched a RAN (GnbId=%q); a restored UE would attach "+
			"to an arbitrary gNB on this PLMN", first.GnbId, found.GnbId)
	}
}

// The sctplb path keys the pool by GnbId, and that exact-key hit must keep working.
func TestAmfRanFindByGnbIdStillFindsSctplbRan(t *testing.T) {
	self := AMF_Self()
	ran := self.NewAmfRanId("208:93:sctplb-1")
	t.Cleanup(func() { self.AmfRanPool.Delete("208:93:sctplb-1") })

	found, ok := self.AmfRanFindByGnbId("208:93:sctplb-1")
	if !ok {
		t.Fatal("sctplb RAN not found by its GnbId key")
		return
	}
	if found != ran {
		t.Fatal("found a different RAN than the one stored")
	}
}
