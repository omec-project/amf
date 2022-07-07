// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

//+build debug

package util

import (
	"github.com/omec-project/path_util"
)

var (
	AmfLogPath           = path_util.Free5gcPath("free5gc/amfsslkey.log")
	AmfPemPath           = path_util.Free5gcPath("free5gc/support/TLS/_debug.pem")
	AmfKeyPath           = path_util.Free5gcPath("free5gc/support/TLS/_debug.key")
	DefaultAmfConfigPath = path_util.Free5gcPath("free5gc/config/amfcfg.yaml")
)
