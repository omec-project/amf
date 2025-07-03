// Copyright 2019 Communication Service/Software Laboratory, National Chiao Tung University (free5gc.org)
//
// SPDX-License-Identifier: Apache-2.0

package ngapConvert

import (
	"github.com/omec-project/ngap/ngapType"
	"github.com/omec-project/openapi/models"
)

func AllowedNssaiToNgap(allowedNssaiModels []models.AllowedSnssai) (allowedNssaiNgap ngapType.AllowedNSSAI) {
	for _, allowedSnssai := range allowedNssaiModels {
		item := ngapType.AllowedNSSAIItem{
			SNSSAI: SNssaiToNgap(*allowedSnssai.AllowedSnssai),
		}
		allowedNssaiNgap.List = append(allowedNssaiNgap.List, item)
	}
	return
}

func AllowedNssaiToModels(allowedNssaiNgap ngapType.AllowedNSSAI) (allowedNssaiModels []models.AllowedSnssai) {
	for _, item := range allowedNssaiNgap.List {
		snssai := SNssaiToModels(item.SNSSAI)
		allowedSnssai := models.AllowedSnssai{
			AllowedSnssai: &snssai,
		}
		allowedNssaiModels = append(allowedNssaiModels, allowedSnssai)
	}
	return
}
