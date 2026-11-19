// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package lawfulintercept

import (
	"testing"
	"time"

	"github.com/omec-project/util/li/store"
	"github.com/omec-project/util/li/types"
	"github.com/omec-project/util/li/x2x3"
)

// TestARelabelReleasesTheSupersededLabelsNumbering is the IRI-POI half of the numbering
// lifetime.
//
// The numbering is keyed by the *delivery* XID — the provisioned ProductID where a task
// carries one — so a ModifyTask that moves the label moves the key, though it never changes
// the task XID. Every record from that point carries the new label, so the contexts under
// the old one are stranded: one entry per subject this warrant had records for, held for the
// life of the process.
//
// The other half is why the release cannot simply be unconditional: a modification that
// leaves the labelling alone must release nothing, because those contexts are the ones
// this task's own next record will number in, and re-issuing a number the mediation
// function has already seen is how loss is signalled on this interface.
func TestARelabelReleasesTheSupersededLabelsNumbering(t *testing.T) {
	const (
		xid      = types.XID("11111111-1111-4111-8111-111111111111")
		oldLabel = types.XID("aaaaaaaa-1111-4111-8111-111111111111")
		newLabel = types.XID("cccccccc-3333-4333-8333-333333333333")
	)

	sub := &subsystem{
		store: store.New(),
		neID:  testNEID,
		ids:   x2x3.NewIdentity(testNEID, amfInterceptionPoint),
	}
	awaitScans(t, sub)

	prev := types.InterceptTask{
		XID:       xid,
		ProductID: oldLabel,
		Targets:   []types.TargetIdentifier{{Type: types.TargetSUPI, Value: "262019876543210"}},
		Products:  []types.ProductType{types.ProductIRI},
		State:     types.TaskActive,
	}

	// Two subjects' records numbered under the warrant's label, as a warrant covering
	// more than one identity produces.
	corrs := [][x2x3.CorrelationIDLength]byte{{1}, {2}}
	for range 3 {
		for _, corr := range corrs {
			sub.ids.Attributes(parseXID(oldLabel), corr, time.Now(), nil, nil)
		}
	}
	if n := sub.ids.Contexts(); n != 2 {
		t.Fatalf("numbering contexts = %d, want 2 — the state this test tracks was never created", n)
	}

	// A modification that leaves the labelling alone: nothing is released, and the next
	// record continues the sequence.
	unchanged := prev
	unchanged.Products = []types.ProductType{types.ProductIRI, types.ProductCC}
	sub.applyTaskChange(&prev, &unchanged)

	if n := sub.ids.Contexts(); n != 2 {
		t.Fatalf("numbering contexts = %d after a modification that did not move the label, want 2: "+
			"the next record repeats a sequence number the mediation function has already seen for "+
			"that context, which is how it detects lost product", n)
	}

	// The relabel. Everything numbered under the superseded label is stranded, because
	// every record from here carries the new one.
	next := unchanged
	next.ProductID = newLabel
	sub.applyTaskChange(&unchanged, &next)

	if n := sub.ids.Contexts(); n != 0 {
		t.Errorf("numbering contexts = %d after the delivery label was superseded, want 0 — one "+
			"entry per subject this warrant had records for, held for the life of the process and "+
			"never numbered in again", n)
	}
}
