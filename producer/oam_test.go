// SPDX-FileCopyrightText: 2022 Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0
//

package producer

import (
	"testing"

	"github.com/omec-project/amf/context"
	"github.com/omec-project/amf/factory"
	"github.com/omec-project/amf/gmm"
	"github.com/omec-project/amf/util"
	"github.com/omec-project/fsm"
	"github.com/omec-project/openapi/models"
	"github.com/stretchr/testify/assert"
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

	assert.Equal(t, uint32(0), gmm.MockDeregisteredInitiatedCallCount)
	assert.Equal(t, uint32(0), gmm.MockRegisteredCallCount)
}

func TestHandleOAMPurgeUEContextRequest_UERegistered(t *testing.T) {
	self := context.AMF_Self()
	amfUe := self.NewAmfUe("imsi-208930100007497")
	amfUe.State[models.AccessType__3_GPP_ACCESS] = fsm.NewState(context.Registered)

	HandleOAMPurgeUEContextRequest(amfUe.Supi, "", nil)

	if _, ok := self.AmfUeFindBySupi(amfUe.Supi); ok {
		t.Errorf("test failed")
	}

	assert.Equal(t, uint32(2), gmm.MockRegisteredCallCount)
	assert.Equal(t, uint32(1), gmm.MockDeregisteredInitiatedCallCount)
}
