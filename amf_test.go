// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0
//
/*
 * AMF Unit Testcases
 *
 */
package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"testing"
	"time"

	"github.com/omec-project/amf/factory"
	"github.com/omec-project/amf/service"
	protos "github.com/omec-project/config5g/proto/sdcoreConfig"
)

var AMFTest = &service.AMF{}

func init() {
	if err := os.Setenv("POD_IP", "127.0.0.1"); err != nil {
		log.Printf("Could not set env POD_IP: %+v", err)
	}
	if err := factory.InitConfigFactory("amfTest/amfcfg.yaml"); err != nil {
		log.Printf("Could not InitConfigFactory: %+v", err)
	}
}

func GetNetworkSliceConfig() *protos.NetworkSliceResponse {
	var rsp protos.NetworkSliceResponse

	rsp.NetworkSlice = make([]*protos.NetworkSlice, 0)

	ns := protos.NetworkSlice{}
	ns.OperationType = protos.OpType_SLICE_ADD
	slice := protos.NSSAI{Sst: "1", Sd: "010203"}
	ns.Nssai = &slice

	site := protos.SiteInfo{SiteName: "siteOne", Gnb: make([]*protos.GNodeB, 0), Plmn: new(protos.PlmnId)}
	gNb := protos.GNodeB{Name: "gnb", Tac: 1}
	site.Gnb = append(site.Gnb, &gNb)
	site.Plmn.Mcc = "208"
	site.Plmn.Mnc = "93"
	ns.Site = &site

	rsp.NetworkSlice = append(rsp.NetworkSlice, &ns)
	return &rsp
}

func TestInitialConfig(t *testing.T) {
	factory.AmfConfig.Configuration.PlmnSupportList = nil
	factory.AmfConfig.Configuration.ServedGumaiList = nil
	factory.AmfConfig.Configuration.SupportTAIList = nil
	Rsp := make(chan *protos.NetworkSliceResponse, 1)

	go func() {
		AMFTest.UpdateConfig(context.Background(), Rsp)
	}()
	Rsp <- GetNetworkSliceConfig()

	time.Sleep(2 * time.Second)
	close(Rsp)

	if factory.AmfConfig.Configuration.PlmnSupportList != nil &&
		factory.AmfConfig.Configuration.ServedGumaiList != nil &&
		factory.AmfConfig.Configuration.SupportTAIList != nil {
		t.Logf("test passed")
	} else {
		t.Errorf("test failed")
	}
}

// data in JSON format which
// is to be decoded
var Data = []byte(`{
	"NetworkSlice": [
		{
		 "Name": "siteOne",
		 "Nssai": {"Sst": "1", "Sd": "010203"},
		 "Site": {
			"SiteName": "siteOne",
			"Gnb": [
				{"Name": "gnb1", "Tac": 1},
				{"Name": "gnb2", "Tac": 2}
			],
			"Plmn": {"mcc": "208", "mnc": "93"}
		  }
		}
		]}`)

func TestUpdateConfig(t *testing.T) {
	var nrp protos.NetworkSliceResponse
	err := json.Unmarshal(Data, &nrp)
	if err != nil {
		panic(err)
	}
	Rsp := make(chan *protos.NetworkSliceResponse)
	go func() {
		Rsp <- &nrp
	}()
	go func() {
		AMFTest.UpdateConfig(context.Background(), Rsp)
	}()

	time.Sleep(2 * time.Second)
	if len(factory.AmfConfig.Configuration.SupportTAIList) == 2 {
		t.Logf("test passed")
	} else {
		t.Errorf("test failed")
	}
}
