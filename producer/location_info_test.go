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
	}
	if provideLocInfo.SupportedFeatures != nil {
		t.Fatalf("expected supportedFeatures to be omitted, got %q", *provideLocInfo.SupportedFeatures)
	}
	if provideLocInfo.CurrentLoc != nil {
		t.Fatalf("expected currentLoc to be omitted, got %v", *provideLocInfo.CurrentLoc)
	}
}
