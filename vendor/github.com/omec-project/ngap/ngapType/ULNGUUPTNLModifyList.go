// Copyright 2019 Communication Service/Software Laboratory, National Chiao Tung University (free5gc.org)
//
// SPDX-License-Identifier: Apache-2.0

package ngapType

// Need to import "github.com/omec-project/aper" if it uses "aper"

/* Sequence of = 35, FULL Name = struct UL_NGU_UP_TNLModifyList */
/* ULNGUUPTNLModifyItem */
type ULNGUUPTNLModifyList struct {
	List []ULNGUUPTNLModifyItem `aper:"valueExt,sizeLB:1,sizeUB:4"`
}
