// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

package gmm_test

import (
	"fmt"
	"testing"

	"github.com/omec-project/amf/gmm"
	"github.com/omec-project/util/fsm"
)

func TestGmmFSM(t *testing.T) {
	if err := fsm.ExportDot(gmm.GmmFSM, "gmm"); err != nil {
		fmt.Printf("fsm export data return error: %+v", err)
	}
}
