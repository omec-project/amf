// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: LicenseRef-ONF-Member-Only-1.0

package util

import (
	"encoding/json"
	"reflect"

	"github.com/free5gc/amf/logger"
)

func MarshToJsonString(v interface{}) (result []string) {
	types := reflect.TypeOf(v)
	val := reflect.ValueOf(v)
	if types.Kind() == reflect.Slice {
		for i := 0; i < val.Len(); i++ {
			tmp, err := json.Marshal(val.Index(i).Interface())
			if err != nil {
				logger.UtilLog.Errorf("Marshal error: %+v", err)
			}

			result = append(result, string(tmp))
		}
	} else {
		tmp, err := json.Marshal(v)
		if err != nil {
			logger.UtilLog.Errorf("Marshal error: %+v", err)
		}

		result = append(result, string(tmp))
	}
	return
}
