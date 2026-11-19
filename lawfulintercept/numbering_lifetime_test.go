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

// TestNumberingReturnsToZeroWhenEveryWarrantIsWithdrawn is the assertion that makes the
// numbering-leak class detectable, rather than a test of one leak.
//
// A numbering context created for a (warrant, subject) pair and never released — by a relabel,
// by a modification, by a withdrawal that takes a different path — is invisible in operation:
// the element keeps numbering correctly, delivery keeps working, and the only symptom is an
// entry per subject per warrant held for the life of the process. A long-lived element
// accumulates them until it dies and takes every warrant it holds with it.
//
// What closes the class rather than the instance is the invariant: **an element holding no
// tasking holds no numbering.** It is cheap to state, it is true at every point where
// interception has stopped, and any future path that creates a context without releasing it
// fails here whether or not anybody thought of that path.
//
// Deliberately over two warrants and several subjects each. One warrant with one subject can
// pass by accident — a release keyed on the wrong thing still empties a map with one entry.
func TestNumberingReturnsToZeroWhenEveryWarrantIsWithdrawn(t *testing.T) {
	const (
		firstXID  = types.XID("11111111-1111-4111-8111-111111111111")
		secondXID = types.XID("22222222-2222-4222-8222-222222222222")
	)

	sub := &subsystem{
		store: store.New(),
		neID:  testNEID,
		ids:   x2x3.NewIdentity(testNEID, amfInterceptionPoint),
	}
	awaitScans(t, sub)

	task := func(xid types.XID, supi string) types.InterceptTask {
		return types.InterceptTask{
			XID:      xid,
			Targets:  []types.TargetIdentifier{{Type: types.TargetSUPI, Value: supi}},
			Products: []types.ProductType{types.ProductIRI},
			State:    types.TaskActive,
		}
	}

	first := task(firstXID, "262019876543210")
	second := task(secondXID, "262019876543211")

	// Records under each warrant, for two subjects each: the state a leak would strand.
	for _, xid := range []types.XID{firstXID, secondXID} {
		for _, corr := range [][x2x3.CorrelationIDLength]byte{{1}, {2}} {
			for range 3 {
				sub.ids.Attributes(parseXID(xid), corr, time.Now(), nil, nil)
			}
		}
	}
	if n := sub.ids.Contexts(); n != 4 {
		t.Fatalf("numbering contexts = %d, want 4 — the state this test tracks was never created", n)
	}

	// Withdrawn one at a time, which is what a DeactivateTask does and what the hook the X1
	// server calls receives. The intermediate assertion is not decoration: a release that
	// discarded *everything* on any withdrawal would reach zero at the end and be a worse
	// defect than the leak, because the surviving warrant's next record would repeat a
	// sequence number the mediation function has already seen — which is how that mediation
	// function detects lost product.
	sub.applyTaskChange(&first, nil)
	if n := sub.ids.Contexts(); n != 2 {
		t.Fatalf("withdrawing one warrant left %d numbering contexts, want the 2 the other "+
			"warrant is still numbering in", n)
	}

	sub.applyTaskChange(&second, nil)
	if n := sub.ids.Contexts(); n != 0 {
		t.Errorf("%d numbering context(s) survive the tasking that created them; an element "+
			"holding no warrants must hold no numbering, or every warrant this element ever "+
			"served costs an entry per subject for the life of the process", n)
	}
}
