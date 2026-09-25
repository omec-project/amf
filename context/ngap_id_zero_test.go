// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"testing"

	"github.com/omec-project/util/drsm"
)

// sequenceDrsm hands out the given ids in order. Every other DrsmInterface method is
// promoted from a nil interface, so an allocator that reaches for one panics.
type sequenceDrsm struct {
	drsm.DrsmInterface
	ids []int32
}

func (s *sequenceDrsm) AllocateInt32ID() (int32, error) {
	id := s.ids[0]
	s.ids = s.ids[1:]
	return id, nil
}

// A stored context with no RAN association carries amfUeNgapId 0, so a live UE given 0
// cannot be told apart from those: a lookup by that id after a restart can restore any of
// them. drsm can hand out 0 -- its ids are (chunk << 10) | offset, chunk 0 can be drawn,
// and offsets are handed out down to 0 -- so the AMF must not take it.
func TestAnAmfUeNgapIdOfZeroIsNeverAllocated(t *testing.T) {
	self := AMF_Self()
	originalStore, originalDrsm := self.EnableDbStore, self.Drsm
	t.Cleanup(func() { self.EnableDbStore, self.Drsm = originalStore, originalDrsm })
	self.EnableDbStore = true
	self.Drsm = &sequenceDrsm{ids: []int32{0, 1023}}

	id, err := self.AllocateAmfUeNgapID()
	if err != nil {
		t.Fatalf("AllocateAmfUeNgapID: %v", err)
	}
	if id != 1023 {
		t.Fatalf("allocated %d, want 1023: zero is what a detached stored context carries", id)
	}
}
