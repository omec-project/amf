// Copyright (c) 2026 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"testing"

	"github.com/omec-project/openapi/v2/models"
)

func TestSearchNFServiceUriBuildsUriFromFqdn(t *testing.T) {
	ipEndPoint := models.IpEndPoint{}
	ipEndPoint.SetPort(29518)
	nfService := models.NFService{}
	nfService.SetServiceName(models.SERVICENAME_NAMF_COMM)
	nfService.SetNfServiceStatus(models.NFSERVICESTATUS_REGISTERED)
	nfService.SetScheme(models.URISCHEME_HTTP)
	nfService.SetFqdn("amf")
	nfService.SetIpEndPoints([]models.IpEndPoint{ipEndPoint})
	nfProfile := models.NFProfileDiscovery{}
	nfProfile.SetNfServiceList(map[string]models.NFService{"0": nfService})

	if got := SearchNFServiceUri(nfProfile, models.SERVICENAME_NAMF_COMM, models.NFSERVICESTATUS_REGISTERED); got != "http://amf:29518" {
		t.Fatalf("SearchNFServiceUri() = %q, want %q", got, "http://amf:29518")
	}
}
