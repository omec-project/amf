// Copyright 2019 Communication Service/Software Laboratory, National Chiao Tung University (free5gc.org)
//
// SPDX-License-Identifier: Apache-2.0

package openapi

import (
	"encoding/json"
	"reflect"

	"github.com/omec-project/openapi/logger"
)

func MarshToJsonString(v interface{}) (result []string) {
	types := reflect.TypeOf(v)
	val := reflect.ValueOf(v)
	if types.Kind() == reflect.Slice {
		for i := 0; i < val.Len(); i++ {
			tmp, err := json.Marshal(val.Index(i).Interface())
			if err != nil {
				logger.OpenapiLog.Errorf("failed to json encode: %v", err)
			}
			result = append(result, string(tmp))
		}
	} else {
		tmp, err := json.Marshal(v)
		if err != nil {
			logger.OpenapiLog.Errorf("failed to json encode: %v", err)
		}

		result = append(result, string(tmp))
	}
	return
}
