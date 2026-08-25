// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"net"
	"testing"

	"github.com/omec-project/amf/protos/sdcoreAmfServer"
	"github.com/omec-project/openapi/v2/models"
)

// restoredRan is the shape an AmfRan comes back in from the database: the serialisable
// fields survive, the transport and the logger do not.
func restoredRan() *AmfRan {
	return &AmfRan{RanPresent: RanPresentGNbId}
}

func ueWithRanUe(ranUe *RanUe) *AmfUe {
	ue := &AmfUe{}
	ue.init()
	ue.RanUe[models.ACCESSTYPE__3_GPP_ACCESS] = ranUe

	return ue
}

func withSctpLb(t *testing.T, enabled bool) {
	t.Helper()

	previous := AMF_Self().EnableSctpLb
	AMF_Self().EnableSctpLb = enabled

	t.Cleanup(func() { AMF_Self().EnableSctpLb = previous })
}

func TestHasLiveRanConnectionRequiresALiveTransport(t *testing.T) {
	withSctpLb(t, false)

	liveConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatalf("open a connection to stand in for the association: %v", err)
	}

	defer liveConn.Close()

	tests := []struct {
		name string
		ue   *AmfUe
		want bool
	}{
		{
			name: "no RanUe for the access type",
			ue: func() *AmfUe {
				ue := &AmfUe{}
				ue.init()
				return ue
			}(),
			want: false,
		},
		{
			name: "RanUe present but nil",
			ue:   ueWithRanUe(nil),
			want: false,
		},
		{
			name: "RanUe with no Ran at all",
			ue:   ueWithRanUe(&RanUe{}),
			want: false,
		},
		{
			// The case that mattered: a context restored from the database.
			name: "RanUe with a Ran restored without its transport",
			ue:   ueWithRanUe(&RanUe{Ran: restoredRan()}),
			want: false,
		},
		{
			name: "RanUe with a live association",
			ue:   ueWithRanUe(&RanUe{Ran: &AmfRan{Conn: liveConn}}),
			want: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.ue.HasLiveRanConnection(models.ACCESSTYPE__3_GPP_ACCESS); got != tc.want {
				t.Errorf("HasLiveRanConnection() = %t, want %t", got, tc.want)
			}

			// CmConnect deliberately still answers the wider question -- is this UE
			// associated over this access type -- because callers like GetAnType rely
			// on it for lookups where a stale RanUe is acceptable.
			if _, keyed := tc.ue.RanUe[models.ACCESSTYPE__3_GPP_ACCESS]; keyed &&
				!tc.ue.CmConnect(models.ACCESSTYPE__3_GPP_ACCESS) {
				t.Error("CmConnect() = false while a RanUe key exists; its meaning must not have changed")
			}
		})
	}
}

// Under SCTP load balancing the AMF holds no socket, so the channel is what has to
// exist -- judging by Conn there would report every UE as idle.
func TestHasLiveRanConnectionUnderSctpLoadBalancing(t *testing.T) {
	withSctpLb(t, true)

	restored := ueWithRanUe(&RanUe{Ran: restoredRan()})
	if restored.HasLiveRanConnection(models.ACCESSTYPE__3_GPP_ACCESS) {
		t.Error("HasLiveRanConnection() = true for a restored context, want false")
	}

	live := ueWithRanUe(&RanUe{Ran: &AmfRan{
		Amf2RanMsgChan: make(chan *sdcoreAmfServer.AmfMessage, 1),
	}})
	if !live.HasLiveRanConnection(models.ACCESSTYPE__3_GPP_ACCESS) {
		t.Error("HasLiveRanConnection() = false with a live message channel, want true")
	}
}
