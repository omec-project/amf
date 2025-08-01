// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0
package ngap_test

import (
	"testing"
	"time"

	"github.com/omec-project/amf/context"
	"github.com/omec-project/amf/factory"
	"github.com/omec-project/amf/logger"
	"github.com/omec-project/amf/metrics"
	"github.com/omec-project/amf/ngap"
	ngaputil "github.com/omec-project/amf/ngap/util"
	"github.com/omec-project/amf/util"
	"github.com/omec-project/openapi/models"
)

func init() {
	// Initializing AMF Context from config.
	testAmfConfig := "../util/testdata/amfcfg.yaml"
	if err := factory.InitConfigFactory(testAmfConfig); err != nil {
		logger.NgapLog.Fatalln("failed to initialize Factory Config")
	}
	if err := metrics.InitialiseKafkaStream(factory.AmfConfig.Configuration); err != nil {
		logger.NgapLog.Fatalln("failed to initialize Kafka Stream")
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
}

// TestHandleNGSetupRequest validates package ngap's handling for NGSetupRequest
func TestHandleNGSetupRequest(t *testing.T) {
	// test cases
	testTable := []struct {
		gnbName, tac string
		gnbId        []byte
		bitLength    uint64
		want, testId byte
	}{
		// expecting SuccessfulOutcome
		{
			testId:    1,
			gnbName:   "GNB2",
			tac:       "\x00\x00\x01",
			gnbId:     []byte{0x00, 0x00, 0x08},
			bitLength: 22,
			want:      ngaputil.NgapPDUSuccessfulOutcome,
		},
		// expecting UnsuccessfulOutcome due to unsupported TA
		{
			testId:    2,
			gnbName:   "GNB2",
			tac:       "\x00\x00\x04",
			gnbId:     []byte{0x00, 0x00, 0x08},
			bitLength: 22,
			want:      ngaputil.NgapPDUUnSuccessfulOutcome,
		},
	}

	conn := &ngaputil.TestConn{}
	for _, test := range testTable {
		testNGSetupReq, err := ngaputil.GetNGSetupRequest(test.gnbId, test.bitLength, test.gnbName, test.tac)
		if err != nil {
			t.Log("Failed to to create NGSetupRequest")
			return
		}
		ngap.Dispatch(conn, testNGSetupReq)
		time.Sleep(2 * time.Second)
		// conn.data holds the NGAP response message
		if len(conn.Data) == 0 {
			t.Error("Unexpected message drop")
			return
		}

		// The first byte of the NGAPPDU indicates the type of NGAP Message
		if conn.Data[0] != test.want {
			t.Error("Test case", test.testId, "failed.  Want:",
				ngaputil.MessageTypeMap[test.want], ",  Got:", ngaputil.MessageTypeMap[conn.Data[0]])
		}
	}
}
