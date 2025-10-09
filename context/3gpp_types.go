// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

package context

import (
	"time"

	"github.com/omec-project/openapi/models"
)

const (
	maxNumOfTAI             int   = 16
	maxNumOfBroadcastPLMNs  int   = 12
	maxNumOfPLMNs           int   = 12
	MaxNumOfSlice           int   = 1024
	maxValueOfAmfUeNgapId   int64 = 1099511627775
	MaxNumOfServedGuamiList int   = 256
	MaxNumOfPDUSessions     int   = 256
	MaxNumOfDRBs            int   = 32
	MaxNumOfAOI             int   = 64
)

// timers at AMF side, defined in TS 24.501 table 10.2.2
const (
	TimeT3513 time.Duration = 6 * time.Second
	TimeT3522 time.Duration = 6 * time.Second
	TimeT3550 time.Duration = 6 * time.Second
	TimeT3560 time.Duration = 6 * time.Second
	TimeT3565 time.Duration = 6 * time.Second
)

type LADN struct {
	Dnn      string
	TaiLists []models.Tai
}

type CauseAll struct {
	Cause        *models.Cause
	NgapCause    *models.NgApCause
	Var5GmmCause *int32
}
