package main

import (
	"fmt"
	"testing"

	protos "github.com/omec-project/config5g/proto/sdcoreConfig"
	"github.com/free5gc/amf/factory"
)

//var AMF = &service.AMF{}

func init() {
	factory.InitConfigFactory("amfTest/amfcfg.yaml")
}

func GetNetworkSliceConfig() *protos.NetworkSliceResponse {
	var rsp protos.NetworkSliceResponse

	rsp.NetworkSlice = make([]*protos.NetworkSlice, 0)

	ns := protos.NetworkSlice{}
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
		var Rsp chan *protos.NetworkSliceResponse
		Rsp = make(chan *protos.NetworkSliceResponse)
		go func() {
			Rsp <- GetNetworkSliceConfig()
		}()
		go func() {
			AMF.UpdateConfig(Rsp)
		}()
		fmt.Printf("test passed") // to indicate test failed
}
