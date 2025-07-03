// Copyright 2019 Communication Service/Software Laboratory, National Chiao Tung University (free5gc.org)
//
// SPDX-License-Identifier: Apache-2.0

package ngapConvert

import (
	"encoding/binary"

	"github.com/omec-project/ngap/ngapType"
)

func PortNumberToInt(port ngapType.PortNumber) (portInt32 int32) {
	portInt32 = int32(binary.BigEndian.Uint16(port.Value))
	return
}

func PortNumberToNgap(portInt32 int32) (port ngapType.PortNumber) {
	port.Value = make([]byte, 2)
	binary.BigEndian.PutUint16(port.Value, uint16(portInt32))
	return
}
