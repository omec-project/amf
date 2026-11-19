// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package ngap

import (
	"testing"

	"github.com/omec-project/amf/context"
	"github.com/omec-project/ngap/v2/ngapType"
)

// TestAHandoverRecordKnowsWhetherHandoverRequiredWasCarried: the AMFRANHandoverRequest record
// needs HANDOVER REQUIRED's source-to-target container, carried forward in the RanUe's stash.
// No stash — the REQUIRED was never seen, or its state was already cleared — is a record that
// cannot be completed and is reported as not produced; a stash is the container to report.
// The acknowledge's own target-to-source container may be empty and is carried as it came.
func TestAHandoverRecordKnowsWhetherHandoverRequiredWasCarried(t *testing.T) {
	list := ngapType.PDUSessionResourceHandoverList{}

	stashed := &context.RanUe{HandOverSourceToTarget: []byte{0x01, 0x02}}
	if h := handoverOf(stashed, nil, []byte{}, list); !h.HasSourceToTarget {
		t.Error("a stashed source-to-target container was not marked carried, so the record " +
			"would be reported as not produced although everything it needs is held")
	} else if h.TargetToSource == nil || len(h.TargetToSource) != 0 {
		t.Errorf("the acknowledge's empty container became % x (nil=%v), want it as sent",
			h.TargetToSource, h.TargetToSource == nil)
	}

	cleared := &context.RanUe{}
	if h := handoverOf(cleared, nil, []byte{0x03}, list); h.HasSourceToTarget {
		t.Error("no stash was marked carried, so the record would be emitted with a missing " +
			"mandatory container")
	}
}

// TestTheHandoverRecordReadsTheUEUnderItsLock: the acknowledge is handled on the target RAN's
// read loop, while the source RanUe's association can be dropped at the same moment by its
// own RAN's release or by the delayed detach of a replaced RanUe. The report reads the
// association through GetAmfUe, once, under the lock those writers take. Run under -race,
// which is how this repository runs its tests; a direct read of the field is a data race.
func TestTheHandoverRecordReadsTheUEUnderItsLock(t *testing.T) {
	sourceUe := &context.RanUe{HandOverSourceToTarget: []byte{0x01}}
	ue := &context.AmfUe{}
	sourceUe.SetAmfUe(ue)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 2000 {
			sourceUe.DetachAmfUe()
			sourceUe.SetAmfUe(ue)
		}
	}()
	for range 2000 {
		reportHandover(sourceUe, []byte{0x02}, ngapType.PDUSessionResourceHandoverList{})
	}
	<-done
}
