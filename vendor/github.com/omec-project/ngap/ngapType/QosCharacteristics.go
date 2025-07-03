// Copyright 2019 Communication Service/Software Laboratory, National Chiao Tung University (free5gc.org)
//
// SPDX-License-Identifier: Apache-2.0

package ngapType

// Need to import "github.com/omec-project/aper" if it uses "aper"

const (
	QosCharacteristicsPresentNothing int = iota /* No components present */
	QosCharacteristicsPresentNonDynamic5QI
	QosCharacteristicsPresentDynamic5QI
	QosCharacteristicsPresentChoiceExtensions
)

type QosCharacteristics struct {
	Present          int
	NonDynamic5QI    *NonDynamic5QIDescriptor `aper:"valueExt"`
	Dynamic5QI       *Dynamic5QIDescriptor    `aper:"valueExt"`
	ChoiceExtensions *ProtocolIESingleContainerQosCharacteristicsExtIEs
}
