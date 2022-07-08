package producer

import (
	"github.com/omec-project/amf/context"
	"github.com/omec-project/amf/factory"
	"github.com/omec-project/amf/gmm"
	"github.com/omec-project/amf/util"
	"github.com/omec-project/fsm"
	"github.com/omec-project/openapi/models"
	"testing"
)

type TestCases struct {
	amfContext         context.AMFContext
	amfue              context.AmfUe
	expectedUeFsmState string
	description        string
}

func init() {
	factory.InitConfigFactory("../amfTest/amfcfg.yaml")

	self := context.AMF_Self()
	util.InitAmfContext(self)

	gmm.Mockinit()
}

func TestHandleOAMPurgeUEContextRequest_UEDeregistered(t *testing.T) {
	self := context.AMF_Self()
	amfUe := self.NewAmfUe("imsi-208930100007497")

	HandleOAMPurgeUEContextRequest(amfUe.Supi, "", nil)

	if _, ok := self.AmfUeFindBySupi(amfUe.Supi); ok {
		t.Errorf("test failed")
	}
}

func TestHandleOAMPurgeUEContextRequest_UERegistered(t *testing.T) {
	self := context.AMF_Self()
	amfUe := self.NewAmfUe("imsi-208930100007497")
	amfUe.State[models.AccessType__3_GPP_ACCESS] = fsm.NewState(context.Registered)

	HandleOAMPurgeUEContextRequest(amfUe.Supi, "", nil)

	if _, ok := self.AmfUeFindBySupi(amfUe.Supi); ok {
		t.Errorf("test failed")
	}
}
