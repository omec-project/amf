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
	if nfProfile.NfServices != nil {
		for _, service := range nfProfile.NfServices {
			if service.ServiceName == serviceName && service.NfServiceStatus == nfServiceStatus {
				port := int32(0)
				if len(service.IpEndPoints) > 0 {
					port = service.IpEndPoints[0].GetPort()
				}
				if nfProfile.GetFqdn() != "" {
					nfUri = getSbiUri(service.Scheme, nfProfile.GetFqdn(), port)
				} else if service.GetFqdn() != "" {
					nfUri = getSbiUri(service.Scheme, service.GetFqdn(), port)
				} else if service.GetApiPrefix() != "" {
					nfUri = service.GetApiPrefix()
				} else if len(service.IpEndPoints) > 0 {
					point := (service.IpEndPoints)[0]
					if point.GetIpv4Address() != "" {
						nfUri = getSbiUri(service.Scheme, point.GetIpv4Address(), point.GetPort())
					} else if len(nfProfile.Ipv4Addresses) != 0 {
						nfUri = getSbiUri(service.Scheme, nfProfile.Ipv4Addresses[0], point.GetPort())
					}
				}
			}
			if nfUri != "" {
				break
			}
		}
	}
	return
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
		}
	}
	return
}
