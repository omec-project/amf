// Copyright (c) 2026 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"testing"

	"github.com/omec-project/openapi/v2"
	"github.com/omec-project/openapi/v2/models"
)

func TestSearchNFServiceUriBuildsUriFromFqdn(t *testing.T) {
	nfProfile := models.NFProfileDiscovery{
		NfServices: []models.NFService{
			{
				ServiceName:     models.SERVICENAME_NAMF_COMM,
				NfServiceStatus: models.NFSERVICESTATUS_REGISTERED,
				Scheme:          models.URISCHEME_HTTP,
				Fqdn:            openapi.PtrString("amf"),
				IpEndPoints: []models.IpEndPoint{
					{
						Port: openapi.PtrInt32(29518),
					},
				},
			},
		},
	}

	if got := SearchNFServiceUri(nfProfile, models.SERVICENAME_NAMF_COMM, models.NFSERVICESTATUS_REGISTERED); got != "http://amf:29518" {
		t.Fatalf("SearchNFServiceUri() = %q, want %q", got, "http://amf:29518")
	}
}
