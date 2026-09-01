// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

package util

import (
	"fmt"

	"github.com/omec-project/openapi/v2/models"
)

func SearchNFServiceUri(nfProfile models.NFProfileDiscovery, serviceName models.ServiceName,
	nfServiceStatus models.NFServiceStatus,
) (nfUri string) {
	for _, service := range nfProfile.GetNfServices() {
		if service.GetServiceName() == serviceName && service.GetNfServiceStatus() == nfServiceStatus {
			port := int32(0)
			if len(service.GetIpEndPoints()) > 0 {
				port = service.GetIpEndPoints()[0].GetPort()
			}
			if nfProfile.GetFqdn() != "" {
				nfUri = getSbiUri(service.GetScheme(), nfProfile.GetFqdn(), port)
			} else if service.GetFqdn() != "" {
				nfUri = getSbiUri(service.GetScheme(), service.GetFqdn(), port)
			} else if service.GetApiPrefix() != "" {
				nfUri = service.GetApiPrefix()
			} else if len(service.GetIpEndPoints()) > 0 {
				point := service.GetIpEndPoints()[0]
				if point.GetIpv4Address() != "" {
					nfUri = getSbiUri(service.GetScheme(), point.GetIpv4Address(), point.GetPort())
				} else if len(nfProfile.GetIpv4Addresses()) != 0 {
					nfUri = getSbiUri(service.GetScheme(), nfProfile.GetIpv4Addresses()[0], point.GetPort())
				}
			}
		}
		if nfUri != "" {
			break
		}
	}
	return nfUri
}

func getSbiUri(scheme models.UriScheme, ipv4Address string, port int32) (uri string) {
	if port != 0 {
		uri = fmt.Sprintf("%s://%s:%d", scheme, ipv4Address, port)
	} else {
		switch scheme {
		case models.URISCHEME_HTTP:
			uri = fmt.Sprintf("%s://%s:80", scheme, ipv4Address)
		case models.URISCHEME_HTTPS:
			uri = fmt.Sprintf("%s://%s:443", scheme, ipv4Address)
		default:
			// Handle unexpected scheme, default to http
			uri = fmt.Sprintf("%s://%s:80", models.URISCHEME_HTTP, ipv4Address)
		}
	}
	return
}
