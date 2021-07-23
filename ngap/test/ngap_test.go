// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0
// SPDX-License-Identifier: LicenseRef-ONF-Member-Only-1.0

package ngap_test

import (
	"log"
	"testing"

	"github.com/free5gc/amf/context"
	"github.com/free5gc/amf/factory"
	"github.com/free5gc/amf/ngap"
	"github.com/free5gc/amf/util"
)

func init() {
	// Initializing AMF Context from config.
	testAmfConfig := "../../amfTest/amfcfg.yaml"
	if err := factory.InitConfigFactory(testAmfConfig); err != nil {
		log.Fatal("Failed to initialzie Factory Config")
	}

	util.InitAmfContext(context.AMF_Self())
}

// TestHandleNGSetupRequest validates package ngap's handling for NGSetupRequest
func TestHandleNGSetupRequest(t *testing.T) {
	conn := &testConn{}

	// test cases
	testTable := []struct {
		input []byte
		want  byte
	}{
		{
			input: testNGSetupReq,
			want:  ngapPDUSuccessfulOutcome,
		},
		{
			input: testNGSetupReqErr,
			want:  ngapPDUUnSuccessfulOutcome,
		},
	}

	for _, test := range testTable {
		ngap.Dispatch(conn, test.input)

		if len(conn.data) == 0 {
			t.Error("Unexpected message drop")
			return
		}

		if conn.data[0] != test.want {
			t.Error("Want:", messageTypeMap[test.want], ", Got:", messageTypeMap[conn.data[0]])
		}
	}
}
