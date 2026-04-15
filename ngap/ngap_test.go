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

func waitForConnData(t *testing.T, conn *ngaputil.TestConn, timeout time.Duration) []byte {
	t.Helper()
	timeoutTimer := time.NewTimer(timeout)
	defer timeoutTimer.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		data := conn.Snapshot()
		if len(data) > 0 {
			return data
		}

		select {
		case <-timeoutTimer.C:
			t.Fatal("timed out waiting for NGAP response")
		case <-ticker.C:
		}
	}
}

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
		conn.Reset()
		testNGSetupReq, err := ngaputil.GetNGSetupRequest(test.gnbId, test.bitLength, test.gnbName, test.tac)
		if err != nil {
			t.Log("Failed to to create NGSetupRequest")
			return
		}
		ngap.Dispatch(conn, testNGSetupReq)
		response := waitForConnData(t, conn, 2*time.Second)

		// The first byte of the NGAPPDU indicates the type of NGAP Message
		if response[0] != test.want {
			t.Error("Test case", test.testId, "failed.  Want:",
				ngaputil.MessageTypeMap[test.want], ",  Got:", ngaputil.MessageTypeMap[response[0]])
		}
	}
}
