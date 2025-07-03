// Copyright 2019 Communication Service/Software Laboratory, National Chiao Tung University (free5gc.org)
//
// SPDX-License-Identifier: Apache-2.0

package ngapType

// Need to import "github.com/omec-project/aper" if it uses "aper"

type GBRQosInformation struct {
	MaximumFlowBitRateDL    BitRate
	MaximumFlowBitRateUL    BitRate
	GuaranteedFlowBitRateDL BitRate
	GuaranteedFlowBitRateUL BitRate
	NotificationControl     *NotificationControl                               `aper:"optional"`
	MaximumPacketLossRateDL *PacketLossRate                                    `aper:"optional"`
	MaximumPacketLossRateUL *PacketLossRate                                    `aper:"optional"`
	IEExtensions            *ProtocolExtensionContainerGBRQosInformationExtIEs `aper:"optional"`
}
