// Copyright (C) 2024-Present Intel Corporation
// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0

package logger

import (
	utilLogger "github.com/omec-project/util/logger"
	"go.uber.org/zap/zapcore"
)

const (
	FieldRanAddr     string = "ran_addr"
	FieldRanId       string = "ran_id"
	FieldAmfUeNgapID string = "amf_ue_ngap_id"
	FieldSupi        string = "supi"
	FieldSuci        string = "suci"
)

var component = utilLogger.NewComponent("AMF")

var (
	AppLog             = component.Category("App")
	InitLog            = component.Category("Init")
	CfgLog             = component.Category("CFG")
	ContextLog         = component.Category("Context")
	DataRepoLog        = component.Category("DBRepo")
	NgapLog            = component.Category("NGAP")
	HandlerLog         = component.Category("Handler")
	HttpLog            = component.Category("HTTP")
	GmmLog             = component.Category("GMM")
	MtLog              = component.Category("MT")
	ProducerLog        = component.Category("Producer")
	LocationLog        = component.Category("LocInfo")
	CommLog            = component.Category("Comm")
	CallbackLog        = component.Category("Callback")
	UtilLog            = component.Category("Util")
	NasLog             = component.Category("NAS")
	ConsumerLog        = component.Category("Consumer")
	EeLog              = component.Category("EventExposure")
	GinLog             = component.Category("GIN")
	GrpcLog            = component.Category("GRPC")
	KafkaLog           = component.Category("Kafka")
	NrfRegistrationLog = component.Category("NrfRegistration")
	PollConfigLog      = component.Category("PollConfig")
)

// SetLogLevel: set the log level (panic|fatal|error|warn|info|debug)
func SetLogLevel(level zapcore.Level) {
	CfgLog.Infoln("set log level:", level)
	component.SetLevel(level)
}
