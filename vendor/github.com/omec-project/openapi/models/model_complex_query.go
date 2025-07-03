// Copyright 2019 Communication Service/Software Laboratory, National Chiao Tung University (free5gc.org)
//
// SPDX-License-Identifier: Apache-2.0

package models

type ComplexQuery struct {
	CNf *Cnf `json: "cnf,omitempty" bson:"cnf,omitempty"`
	DNf *Dnf `json: "dnf,omitempty" bson:"dnf,omitempty"`
}
