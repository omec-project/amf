// Copyright 2019 Communication Service/Software Laboratory, National Chiao Tung University (free5gc.org)
//
// SPDX-License-Identifier: Apache-2.0

package ngapType

// Need to import "github.com/omec-project/aper" if it uses "aper"

type ProcedureCode struct {
	Value int64 `aper:"valueLB:0,valueUB:255"`
}

const (
	ProcedureCodeAMFConfigurationUpdate                int64 = 0
	ProcedureCodeAMFStatusIndication                   int64 = 1
	ProcedureCodeCellTrafficTrace                      int64 = 2
	ProcedureCodeDeactivateTrace                       int64 = 3
	ProcedureCodeDownlinkNASTransport                  int64 = 4
	ProcedureCodeDownlinkNonUEAssociatedNRPPaTransport int64 = 5
	ProcedureCodeDownlinkRANConfigurationTransfer      int64 = 6
	ProcedureCodeDownlinkRANStatusTransfer             int64 = 7
	ProcedureCodeDownlinkUEAssociatedNRPPaTransport    int64 = 8
	ProcedureCodeErrorIndication                       int64 = 9
	ProcedureCodeHandoverCancel                        int64 = 10
	ProcedureCodeHandoverNotification                  int64 = 11
	ProcedureCodeHandoverPreparation                   int64 = 12
	ProcedureCodeHandoverResourceAllocation            int64 = 13
	ProcedureCodeInitialContextSetup                   int64 = 14
	ProcedureCodeInitialUEMessage                      int64 = 15
	ProcedureCodeLocationReportingControl              int64 = 16
	ProcedureCodeLocationReportingFailureIndication    int64 = 17
	ProcedureCodeLocationReport                        int64 = 18
	ProcedureCodeNASNonDeliveryIndication              int64 = 19
	ProcedureCodeNGReset                               int64 = 20
	ProcedureCodeNGSetup                               int64 = 21
	ProcedureCodeOverloadStart                         int64 = 22
	ProcedureCodeOverloadStop                          int64 = 23
	ProcedureCodePaging                                int64 = 24
	ProcedureCodePathSwitchRequest                     int64 = 25
	ProcedureCodePDUSessionResourceModify              int64 = 26
	ProcedureCodePDUSessionResourceModifyIndication    int64 = 27
	ProcedureCodePDUSessionResourceRelease             int64 = 28
	ProcedureCodePDUSessionResourceSetup               int64 = 29
	ProcedureCodePDUSessionResourceNotify              int64 = 30
	ProcedureCodePrivateMessage                        int64 = 31
	ProcedureCodePWSCancel                             int64 = 32
	ProcedureCodePWSFailureIndication                  int64 = 33
	ProcedureCodePWSRestartIndication                  int64 = 34
	ProcedureCodeRANConfigurationUpdate                int64 = 35
	ProcedureCodeRerouteNASRequest                     int64 = 36
	ProcedureCodeRRCInactiveTransitionReport           int64 = 37
	ProcedureCodeTraceFailureIndication                int64 = 38
	ProcedureCodeTraceStart                            int64 = 39
	ProcedureCodeUEContextModification                 int64 = 40
	ProcedureCodeUEContextRelease                      int64 = 41
	ProcedureCodeUEContextReleaseRequest               int64 = 42
	ProcedureCodeUERadioCapabilityCheck                int64 = 43
	ProcedureCodeUERadioCapabilityInfoIndication       int64 = 44
	ProcedureCodeUETNLABindingRelease                  int64 = 45
	ProcedureCodeUplinkNASTransport                    int64 = 46
	ProcedureCodeUplinkNonUEAssociatedNRPPaTransport   int64 = 47
	ProcedureCodeUplinkRANConfigurationTransfer        int64 = 48
	ProcedureCodeUplinkRANStatusTransfer               int64 = 49
	ProcedureCodeUplinkUEAssociatedNRPPaTransport      int64 = 50
	ProcedureCodeWriteReplaceWarning                   int64 = 51
	ProcedureCodeSecondaryRATDataUsageReport           int64 = 52
)
