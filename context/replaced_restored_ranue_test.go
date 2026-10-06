// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"testing"

	"github.com/omec-project/openapi/v2/models"
)

// restoredAfterARestart stores a live UE, forgets it as a restart would, restores it by SUPI,
// and returns its RAN, the restored UE and the RanUe the restore published.
func restoredAfterARestart(t *testing.T, supi string, ranUeNgapID int64) (*AmfRan, *AmfUe, *RanUe) {
	t.Helper()

	self := AMF_Self()
	ran, ue, _, doc := liveUeWithItsDocument(t, supi, ranUeNgapID)
	ue.Remove()
	serveStoredDocument(t, doc)

	restored, ok := self.AmfUeFindBySupi(supi)
	if !ok {
		t.Fatal("the stored context was not restored")
	}
	restoredRanUe := restored.GetRanUe(models.ACCESSTYPE__3_GPP_ACCESS)
	if restoredRanUe == nil || self.RanUeFindByAmfUeNgapIDLocal(restoredRanUe.AmfUeNgapId) != restoredRanUe {
		t.Fatal("the restore did not publish its RanUe")
	}
	t.Cleanup(func() { self.RanUePool.CompareAndDelete(restoredRanUe.AmfUeNgapId, restoredRanUe) })

	return ran, restored, restoredRanUe
}

// newRanUeOn makes a RanUe on ran, as an Initial UE Message would.
func newRanUeOn(t *testing.T, ran *AmfRan, ranUeNgapID int64) *RanUe {
	t.Helper()

	self := AMF_Self()
	// Allocate the id without the DRSM, which the stored-document tests stub.
	dbStore := self.EnableDbStore
	self.EnableDbStore = false
	ranUe, err := ran.NewRanUe(ranUeNgapID)
	self.EnableDbStore = dbStore
	if err != nil {
		t.Fatalf("NewRanUe: %v", err)
	}
	t.Cleanup(func() { self.RanUePool.Delete(ranUe.AmfUeNgapId) })

	return ranUe
}

// When the AMF terminates SCTP itself, a RanUe a restore published, replaced by a newer one,
// leaves RanUePool. Each RAN node erased its UE contexts in the NG Setup it made after the
// restart (TS 38.413 8.7.1.1), so no release will come to remove it.
func TestAReplacedRestoredRanUeLeavesRanUePool(t *testing.T) {
	withSctpLb(t, false)
	self := AMF_Self()

	ran, restored, restoredRanUe := restoredAfterARestart(t, "imsi-208930100009941", 941)
	newer := newRanUeOn(t, ran, 942)
	restored.AttachRanUe(newer)

	if self.RanUeFindByAmfUeNgapIDLocal(restoredRanUe.AmfUeNgapId) == restoredRanUe {
		t.Errorf("the replaced restored RanUe is still in RanUePool under %d", restoredRanUe.AmfUeNgapId)
	}
	if self.RanUeFindByAmfUeNgapIDLocal(newer.AmfUeNgapId) != newer {
		t.Error("the newer RanUe is not in RanUePool")
	}
}

// Behind an SCTP load balancer an AMF restart is followed by no NG Setup, so the RAN node can
// still release a restored RanUe, and that release has to find it.
func TestAReplacedRestoredRanUeStaysBehindAnSctpLoadBalancer(t *testing.T) {
	withSctpLb(t, true)
	self := AMF_Self()

	ran, restored, restoredRanUe := restoredAfterARestart(t, "imsi-208930100009943", 943)
	newer := newRanUeOn(t, ran, 944)
	restored.AttachRanUe(newer)

	if self.RanUeFindByAmfUeNgapID(restoredRanUe.AmfUeNgapId) != restoredRanUe {
		t.Error("a release of the replaced restored RanUe would not find it")
	}
}

// A replaced RanUe that a RAN lists stays in RanUePool: its own release, which also takes it off
// that list, removes it.
func TestAReplacedRanUeThatARanListsStaysInRanUePool(t *testing.T) {
	withSctpLb(t, false)
	self := AMF_Self()

	t.Run("a live one", func(t *testing.T) {
		ran, ue, first, _ := liveUeWithItsDocument(t, "imsi-208930100009945", 945)
		ue.AttachRanUe(newRanUeOn(t, ran, 946))

		if self.RanUeFindByAmfUeNgapIDLocal(first.AmfUeNgapId) != first {
			t.Error("replacing a RanUe its RAN lists took it out of RanUePool before its release")
		}
	})

	t.Run("a restored one a lookup by RAN-UE-NGAP-ID listed", func(t *testing.T) {
		ran, restored, restoredRanUe := restoredAfterARestart(t, "imsi-208930100009947", 947)
		if ran.RanUeFindByRanUeNgapID(947) != restoredRanUe {
			t.Fatal("a lookup by RAN-UE-NGAP-ID did not list the restored RanUe")
		}
		restored.AttachRanUe(newRanUeOn(t, ran, 948))

		if self.RanUeFindByAmfUeNgapIDLocal(restoredRanUe.AmfUeNgapId) != restoredRanUe {
			t.Error("replacing a restored RanUe its RAN lists took it out of RanUePool")
		}
	})

	t.Run("a restored one switched to another RAN", func(t *testing.T) {
		ran, restored, restoredRanUe := restoredAfterARestart(t, "imsi-208930100009949", 949)
		target := self.NewAmfRanId("208:93:switch-target")
		target.AnType = models.ACCESSTYPE__3_GPP_ACCESS
		t.Cleanup(func() { self.AmfRanPool.Delete("208:93:switch-target") })
		if err := restoredRanUe.SwitchToRan(target, 950); err != nil {
			t.Fatalf("SwitchToRan: %v", err)
		}
		restored.AttachRanUe(newRanUeOn(t, ran, 951))

		if self.RanUeFindByAmfUeNgapIDLocal(restoredRanUe.AmfUeNgapId) != restoredRanUe {
			t.Error("replacing a restored RanUe another RAN lists took it out of RanUePool")
		}
	})
}
