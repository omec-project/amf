// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"testing"
	"time"

	"github.com/omec-project/openapi/v2/models"
)

// detachGrace is longer than the time AttachRanUe gives a replaced RanUe before it lets go of
// the UE.
const detachGrace = 3 * time.Second

// ueReplacingItsRanUe attaches a UE to one RanUe and then to another on the same RAN, and
// returns the UE and both RanUes. AttachRanUe starts the replaced RanUe's grace period.
func ueReplacingItsRanUe(t *testing.T, gnbID string, ranUeNgapID int64) (*AmfUe, *AmfRan, *RanUe, *RanUe) {
	t.Helper()

	self := AMF_Self()
	ran := self.NewAmfRanId(gnbID)
	ran.AnType = models.ACCESSTYPE__3_GPP_ACCESS
	t.Cleanup(func() { self.AmfRanPool.Delete(gnbID) })

	ue := &AmfUe{}
	ue.init()
	old, err := ran.NewRanUe(ranUeNgapID)
	if err != nil {
		t.Fatalf("NewRanUe: %v", err)
	}
	ue.AttachRanUe(old)
	newer, err := ran.NewRanUe(ranUeNgapID + 1)
	if err != nil {
		t.Fatalf("NewRanUe: %v", err)
	}
	t.Cleanup(func() {
		self.RanUePool.Delete(old.AmfUeNgapId)
		self.RanUePool.Delete(newer.AmfUeNgapId)
	})
	ue.AttachRanUe(newer)

	return ue, ran, old, newer
}

// lettingGo waits up to detachGrace for ranUe to let go of its UE, and reports whether it did.
func lettingGo(ranUe *RanUe) bool {
	deadline := time.Now().Add(detachGrace)
	for time.Now().Before(deadline) {
		if ranUe.GetAmfUe() == nil {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}

	return ranUe.GetAmfUe() == nil
}

// A RanUe that a newer one has replaced lets go of the UE once its grace period is over, unless
// it has become the UE's RanUe again, or another UE's. It used to let go only while the newer
// one was still attached: once that one had been released, or replaced in turn, the old RanUe
// kept pointing at the UE indefinitely, and after a deregistration at a UE that was gone. Each
// case waits out the grace period, so they run in parallel, each on its own RAN.
func TestAReplacedRanUeLetsGoOfTheUe(t *testing.T) {
	t.Run("while the newer RanUe is attached", func(t *testing.T) {
		t.Parallel()

		_, _, old, _ := ueReplacingItsRanUe(t, "208:93:detach-attached", 941)

		if !lettingGo(old) {
			t.Error("the replaced RanUe still points at the UE")
		}
	})

	t.Run("after the newer RanUe is released", func(t *testing.T) {
		t.Parallel()

		_, _, old, newer := ueReplacingItsRanUe(t, "208:93:detach-released", 943)
		if err := newer.Remove(); err != nil {
			t.Fatalf("removing the newer RanUe: %v", err)
		}

		if !lettingGo(old) {
			t.Error("the replaced RanUe still points at the UE after the newer one was released")
		}
	})

	t.Run("after the newer RanUe is replaced in turn", func(t *testing.T) {
		t.Parallel()

		ue, ran, old, newer := ueReplacingItsRanUe(t, "208:93:detach-replaced", 945)
		newest, err := ran.NewRanUe(947)
		if err != nil {
			t.Fatalf("NewRanUe: %v", err)
		}
		t.Cleanup(func() { AMF_Self().RanUePool.Delete(newest.AmfUeNgapId) })
		ue.AttachRanUe(newest)

		if !lettingGo(old) {
			t.Error("the first RanUe still points at the UE after the second was replaced in turn")
		}
		if !lettingGo(newer) {
			t.Error("the second RanUe still points at the UE")
		}
	})

	t.Run("not when it is attached again", func(t *testing.T) {
		t.Parallel()

		ue, _, old, _ := ueReplacingItsRanUe(t, "208:93:detach-again", 948)
		ue.AttachRanUe(old)

		if lettingGo(old) {
			t.Error("a RanUe attached again let go of the UE")
		}
	})

	t.Run("not from another UE it has been attached to since", func(t *testing.T) {
		t.Parallel()

		_, _, old, _ := ueReplacingItsRanUe(t, "208:93:detach-other", 950)
		other := &AmfUe{}
		other.init()
		other.AttachRanUe(old)

		if lettingGo(old) {
			t.Error("the first UE's grace period detached the RanUe from another UE")
		}
	})
}
