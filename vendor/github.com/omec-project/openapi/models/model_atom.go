// Copyright 2019 Communication Service/Software Laboratory, National Chiao Tung University (free5gc.org)
//
// SPDX-License-Identifier: Apache-2.0

package models

type Atom struct {
	Attr     string `json:"attr" bson:"attr"`
	Value    string `json:"value" bson:"value"` // TODO: AnyType
	Negative bool   `json:"negative,omitempty" bson:"negative,omitempty"`
}
