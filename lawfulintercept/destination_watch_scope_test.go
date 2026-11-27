// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package lawfulintercept

import (
	"testing"

	"github.com/omec-project/util/li/store"
	"github.com/omec-project/util/li/types"
)

// TestTheWatcherDoesNotWatchMDF2ForAContentOnlyTask: a task ordering only content delivers no
// xIRI from this element, so the configured MDF2 is not its destination. canApply keeps such a
// task out of the store, so this is the watcher agreeing with the status answer and with
// delivery rather than a live fault; the task is stored directly to ask the question.
func TestTheWatcherDoesNotWatchMDF2ForAContentOnlyTask(t *testing.T) {
	st := store.New()
	sub := &subsystem{store: st, mdf2: testAgency2Addr}
	down := func(string) bool { return true }

	if !st.Activate(types.InterceptTask{
		XID:      "33333333-3333-4333-8333-333333333333",
		Targets:  []types.TargetIdentifier{{Type: types.TargetSUPI, Value: "001010000000001"}},
		Products: []types.ProductType{types.ProductCC},
		State:    types.TaskActive,
	}) {
		t.Fatal("activate the content task")
	}
	if h := sub.x2Health(down); len(h) != 0 {
		t.Errorf("the watcher samples %v for a content-only task; it delivers no xIRI from here", h)
	}

	// A task naming no destination is served by the configured MDF2, reported at element scope.
	if !st.Activate(types.InterceptTask{
		XID:      "22222222-2222-4222-8222-222222222222",
		Targets:  []types.TargetIdentifier{{Type: types.TargetSUPI, Value: "001010000000002"}},
		Products: []types.ProductType{types.ProductIRI},
		State:    types.TaskActive,
	}) {
		t.Fatal("activate the IRI task")
	}
	h := sub.x2Health(down)
	if len(h) != 1 || h[0].Address != testAgency2Addr || h[0].DID != "" || !h[0].Unreachable {
		t.Errorf("the watcher samples %+v, want the configured MDF2 once, unreachable, at element scope", h)
	}
}
