// Copyright 2019 Communication Service/Software Laboratory, National Chiao Tung University (free5gc.org)
//
// SPDX-License-Identifier: Apache-2.0

package models

type Cnf struct {
	CnfUnits []CnfUnit `json:"cnfUnits" bson:"cnfUnits"`
}
