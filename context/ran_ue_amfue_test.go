// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
//
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"sync"
	"testing"
)

// A UE context release runs on a different goroutine from the NGAP reader, so the
// association can be dropped between a reader's nil check and its use of the pointer.
// The reader is the connection's only goroutine and it has no recover(), so observing
// that nil ends the whole AMF process rather than one procedure -- every UE on every
// gNB goes with it. Run under -race, this fails if the field is reached directly.
func TestTheAssociationCanBeDroppedWhileReadersHoldIt(t *testing.T) {
	ranUe := &RanUe{}
	amfUe := &AmfUe{Supi: "imsi-208930100007487"}
	ranUe.SetAmfUe(amfUe)

	const rounds = 5000

	var wg sync.WaitGroup

	wg.Add(1)

	go func() {
		defer wg.Done()

		for i := range rounds {
			if i%2 == 0 {
				ranUe.DetachAmfUe()
				continue
			}

			ranUe.SetAmfUe(amfUe)
		}
	}()

	wg.Add(1)

	go func() {
		defer wg.Done()

		for range rounds {
			// The shape every reader must use: read the association once, then work
			// from that pointer, so a release cannot turn it nil mid-procedure.
			if held := ranUe.GetAmfUe(); held != nil {
				_ = held.Supi
			}
		}
	}()

	wg.Wait()
}

// Release must not undo a newer association, which is why the clear is conditional.
func TestDetachAmfUeIfLeavesANewerAssociationAlone(t *testing.T) {
	ranUe := &RanUe{}
	first := &AmfUe{Supi: "imsi-208930100007487"}
	second := &AmfUe{Supi: "imsi-208930100007488"}

	ranUe.SetAmfUe(first)
	ranUe.SetAmfUe(second)

	ranUe.DetachAmfUeIf(first)

	if held := ranUe.GetAmfUe(); held != second {
		t.Errorf("association = %v, want the newer context %q", held, second.Supi)
	}

	ranUe.DetachAmfUeIf(second)

	if held := ranUe.GetAmfUe(); held != nil {
		t.Errorf("association = %v, want nil after releasing the context it holds", held)
	}
}

// FetchRanUeContext can hand back a nil RanUe, and callers read the association from
// it before checking anything else.
func TestGetAmfUeOnANilRanUe(t *testing.T) {
	var ranUe *RanUe

	if held := ranUe.GetAmfUe(); held != nil {
		t.Errorf("GetAmfUe() = %v, want nil", held)
	}
}
