package nasconv

import (
	"reflect"
	"testing"

	"github.com/omec-project/nas/v2/nasMessage"
	"github.com/omec-project/nas/v2/nasType"
	"github.com/omec-project/openapi/v2"
	"github.com/omec-project/openapi/v2/models"
)

func TestSnssaiToNas(t *testing.T) {
	got := SnssaiToNas(models.Snssai{Sst: 1, Sd: openapi.PtrString("010203")})
	if want := []uint8{0x04, 0x01, 0x01, 0x02, 0x03}; !reflect.DeepEqual(got, want) {
		t.Fatalf("SnssaiToNas() = %x, want %x", got, want)
	}
}

func TestRequestedNssaiToModels(t *testing.T) {
	input := &nasType.RequestedNSSAI{Iei: nasMessage.RegistrationRequestRequestedNSSAIType, Len: 5, Buffer: []uint8{0x04, 0x01, 0x01, 0x02, 0x03}}
	got, err := RequestedNssaiToModels(input)
	if err != nil {
		t.Fatalf("RequestedNssaiToModels() error = %v", err)
	}
	if len(got) != 1 || got[0].ServingSnssai.GetSst() != 1 || got[0].ServingSnssai.GetSd() != "010203" {
		t.Fatalf("RequestedNssaiToModels() = %#v", got)
	}
}
