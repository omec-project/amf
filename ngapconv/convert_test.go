package ngapconv

import (
	"testing"

	"github.com/omec-project/openapi/v2"
	"github.com/omec-project/openapi/v2/models"
)

func TestPlmnRoundTrip(t *testing.T) {
	want := models.PlmnId{Mcc: "208", Mnc: "93"}
	got, err := PlmnIdToModels(PlmnIdToNgap(want))
	if err != nil {
		t.Fatalf("PlmnIdToModels() error = %v", err)
	}
	if got != want {
		t.Fatalf("PLMN round trip = %#v, want %#v", got, want)
	}
}

func TestSNssaiRoundTrip(t *testing.T) {
	want := models.Snssai{Sst: 1, Sd: openapi.PtrString("010203")}
	got, err := SNssaiToModels(SNssaiToNgap(want))
	if err != nil {
		t.Fatalf("SNssaiToModels() error = %v", err)
	}
	if got.GetSst() != want.GetSst() || got.GetSd() != want.GetSd() {
		t.Fatalf("S-NSSAI round trip = %#v", got)
	}
}
