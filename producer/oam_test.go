// SPDX-FileCopyrightText: 2022 Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0
//

package producer

import (
	ctxt "context"
	"testing"

	"github.com/omec-project/amf/context"
	"github.com/omec-project/amf/factory"
	"github.com/omec-project/amf/gmm"
	"github.com/omec-project/amf/logger"
	"github.com/omec-project/amf/util"
	"github.com/omec-project/openapi/models"
	"github.com/omec-project/util/fsm"
)

func init() {
	if err := factory.InitConfigFactory("../util/testdata/amfcfg.yaml"); err != nil {
		logger.ProducerLog.Errorf("error in InitConfigFactory: %v", err)
	}

	self := context.AMF_Self()
	util.InitAmfContext(self)
	self.ServedGuamiList = []models.Guami{
		{
			PlmnId: &models.PlmnId{Mcc: "208", Mnc: "93"},
			AmfId:  "cafe00",
		},
	}
	self.SupportTaiLists = []models.Tai{
		{
			PlmnId: &models.PlmnId{Mcc: "208", Mnc: "93"},
			Tac:    "1",
		},
	}
	self.PlmnSupportList = []models.PlmnSnssai{
		{
			PlmnId: &models.PlmnId{Mcc: "208", Mnc: "93"},
			SNssaiList: []models.Snssai{
				{
					Sst: 1, Sd: "010203",
				},
				{
					Sst: 1, Sd: "112233",
				},
			},
		},
	}

	gmm.Mockinit()
}

func TestHandleOAMPurgeUEContextRequest(t *testing.T) {
	tests := []struct {
		name                                string
		setupUE                            func(*context.AMFContext) *context.AmfUe
		expectedDeregisteredInitiatedCount uint32
		expectedRegisteredCount            uint32
		description                        string
	}{
		{
			name: "UE_Deregistered",
			setupUE: func(self *context.AMFContext) *context.AmfUe {
				// UE is created but not in registered state (default deregistered)
				return self.NewAmfUe("imsi-208930100007497")
			},
			expectedDeregisteredInitiatedCount: 0,
			expectedRegisteredCount:            0,
			description:                        "UE in deregistered state should be purged without state transitions",
		},
		{
			name: "UE_Registered",
			setupUE: func(self *context.AMFContext) *context.AmfUe {
				amfUe := self.NewAmfUe("imsi-208930100007497")
				// Set UE to registered state
				amfUe.State[models.AccessType__3_GPP_ACCESS] = fsm.NewState(context.Registered)
				return amfUe
			},
			expectedDeregisteredInitiatedCount: 1,
			expectedRegisteredCount:            2,
			description:                        "UE in registered state should trigger deregistration before purge",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			self := context.AMF_Self()
			var err error
			self.Drsm, err = util.MockDrsmInit()
			if err != nil {
				t.Fatalf("error in MockDrsmInit: %v", err)
			}

			// Reset mock counters
			gmm.MockDeregisteredInitiatedCallCount = 0
			gmm.MockRegisteredCallCount = 0

			amfUe := tt.setupUE(self)
			HandleOAMPurgeUEContextRequest(ctxt.Background(), amfUe.Supi, "", nil)
			if _, ok := self.AmfUeFindBySupi(amfUe.Supi); ok {
				t.Errorf("UE should have been purged from context but still exists")
			}

			if gmm.MockDeregisteredInitiatedCallCount != tt.expectedDeregisteredInitiatedCount {
				t.Errorf("MockDeregisteredInitiatedCallCount: got = %d, want = %d",
					gmm.MockDeregisteredInitiatedCallCount, tt.expectedDeregisteredInitiatedCount)
			}

			if gmm.MockRegisteredCallCount != tt.expectedRegisteredCount {
				t.Errorf("MockRegisteredCallCount: got = %d, want = %d",
					gmm.MockRegisteredCallCount, tt.expectedRegisteredCount)
			}

			t.Logf("Test passed: %s", tt.description)
		})
	}
}
