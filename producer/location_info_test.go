// Copyright (c) 2026 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package producer

import (
	"testing"

	"github.com/omec-project/amf/context"
	"github.com/omec-project/openapi/v2/models"
)

func TestProvideLocationInfoProcedureOmitsSupportedFeaturesWithoutRanUe(t *testing.T) {
	self := context.AMF_Self()
	ue := self.NewAmfUe("imsi-208930100007499")
	ue.RanUe[models.ACCESSTYPE__3_GPP_ACCESS] = nil
	defer func() {
		delete(ue.RanUe, models.ACCESSTYPE__3_GPP_ACCESS)
		ue.Remove()
	}()

	requestLocInfo := models.NewRequestLocInfo()
	requestLocInfo.SetSupportedFeatures("1")

	provideLocInfo, problemDetails := ProvideLocationInfoProcedure(*requestLocInfo, ue.Supi)
	if problemDetails != nil {
		t.Fatalf("expected nil problem details, got %+v", problemDetails)
	}
	if provideLocInfo == nil {
		t.Fatal("expected non-nil location info")
		return
	}
	if provideLocInfo.SupportedFeatures != nil {
		t.Fatalf("expected supportedFeatures to be omitted, got %q", *provideLocInfo.SupportedFeatures)
		return
	}
	if provideLocInfo.CurrentLoc != nil {
		t.Fatalf("expected currentLoc to be omitted, got %v", *provideLocInfo.CurrentLoc)
		return
	}
}

// TestProvideLocationInfoDoesNotAliasLiveUeLocation is the shallow-copy test that a
// `-race` run cannot give us. The handler returns while the response is serialised
// later, so what matters is not only that the read took the lock but that what it
// handed back holds no path into the UE's live state. models.UserLocation is five
// pointer-bearing members, so a value copy under the lock would still alias, and the
// mutation below would show up in a response that has already been built.
func TestProvideLocationInfoDoesNotAliasLiveUeLocation(t *testing.T) {
	self := context.AMF_Self()
	ue := self.NewAmfUe("imsi-208930100007498")
	ue.RanUe[models.ACCESSTYPE__3_GPP_ACCESS] = nil
	defer func() {
		delete(ue.RanUe, models.ACCESSTYPE__3_GPP_ACCESS)
		ue.Remove()
	}()

	ue.SetLocation(models.UserLocation{
		NrLocation: &models.NrLocation{Tai: models.Tai{Tac: "000001"}},
	})

	requestLocInfo := models.NewRequestLocInfo()
	requestLocInfo.SetReqCurrentLoc(true)

	provideLocInfo, problemDetails := ProvideLocationInfoProcedure(*requestLocInfo, ue.GetSupi())
	if problemDetails != nil {
		t.Fatalf("expected nil problem details, got %+v", problemDetails)
	}
	if provideLocInfo == nil || provideLocInfo.Location == nil || provideLocInfo.Location.NrLocation == nil {
		t.Fatal("expected a location in the response")
		return
	}

	// What the UE's own NAS or NGAP procedure does while the response is in flight.
	ue.Location.NrLocation.Tai.Tac = "ffffff"

	if got := provideLocInfo.Location.NrLocation.Tai.Tac; got != "000001" {
		t.Fatalf("response aliases the live UE location: Tac = %q, want %q", got, "000001")
	}
}
