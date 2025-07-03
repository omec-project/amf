// Copyright 2019 Communication Service/Software Laboratory, National Chiao Tung University (free5gc.org)
//
// SPDX-License-Identifier: Apache-2.0

package ngapType

// Need to import "github.com/omec-project/aper" if it uses "aper"

type CoreNetworkAssistanceInformation struct {
	UEIdentityIndexValue            UEIdentityIndexValue `aper:"valueLB:0,valueUB:1"`
	UESpecificDRX                   *PagingDRX           `aper:"optional"`
	PeriodicRegistrationUpdateTimer PeriodicRegistrationUpdateTimer
	MICOModeIndication              *MICOModeIndication `aper:"optional"`
	TAIListForInactive              TAIListForInactive
	ExpectedUEBehaviour             *ExpectedUEBehaviour                                              `aper:"valueExt,optional"`
	IEExtensions                    *ProtocolExtensionContainerCoreNetworkAssistanceInformationExtIEs `aper:"optional"`
}
