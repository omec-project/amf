// SPDX-FileCopyrightText: 2026 Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"testing"

	"github.com/omec-project/ngap/ngapType"
)

func TestNrLocationHasNtnExt(t *testing.T) {
	tests := []struct {
		name string
		loc  *ngapType.UserLocationInformationNR
		want bool
	}{
		{
			name: "nil location",
			loc:  nil,
			want: false,
		},
		{
			name: "nil extensions",
			loc:  &ngapType.UserLocationInformationNR{},
			want: false,
		},
		{
			name: "empty extension list",
			loc: &ngapType.UserLocationInformationNR{
				IEExtensions: &ngapType.ProtocolExtensionContainerUserLocationInformationNRExtIEs{},
			},
			want: false,
		},
		{
			name: "unrelated extension",
			loc:  newNRLocationWithExtensionIDs(42),
			want: false,
		},
		{
			name: "NRNTN-TAI-Information extension present",
			loc:  newNRLocationWithExtensionIDs(ProtocolExtensionIDNRNTNTAIInformation),
			want: true,
		},
		{
			name: "NRNTN-TAI-Information among several extensions",
			loc:  newNRLocationWithExtensionIDs(42, 99, ProtocolExtensionIDNRNTNTAIInformation, 150),
			want: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := nrLocationHasNtnExt(tc.loc); got != tc.want {
				t.Errorf("nrLocationHasNtnExt = %v, want %v", got, tc.want)
			}
		})
	}
}

func newNRLocationWithExtensionIDs(ids ...int64) *ngapType.UserLocationInformationNR {
	list := make([]ngapType.UserLocationInformationNRExtIEs, 0, len(ids))
	for _, id := range ids {
		list = append(list, ngapType.UserLocationInformationNRExtIEs{
			Id: ngapType.ProtocolExtensionID{Value: id},
		})
	}
	return &ngapType.UserLocationInformationNR{
		IEExtensions: &ngapType.ProtocolExtensionContainerUserLocationInformationNRExtIEs{
			List: list,
		},
	}
}
