// Copyright 2019 Communication Service/Software Laboratory, National Chiao Tung University (free5gc.org)
//
// SPDX-License-Identifier: Apache-2.0

package logger

import (
	"reflect"

	"github.com/asaskevich/govalidator"
)

type Logger struct {
	AMF    *LogSetting `yaml:"AMF" valid:"optional"`
	AUSF   *LogSetting `yaml:"AUSF" valid:"optional"`
	N3IWF  *LogSetting `yaml:"N3IWF" valid:"optional"`
	NRF    *LogSetting `yaml:"NRF" valid:"optional"`
	NSSF   *LogSetting `yaml:"NSSF" valid:"optional"`
	PCF    *LogSetting `yaml:"PCF" valid:"optional"`
	SMF    *LogSetting `yaml:"SMF" valid:"optional"`
	UDM    *LogSetting `yaml:"UDM" valid:"optional"`
	UDR    *LogSetting `yaml:"UDR" valid:"optional"`
	UPF    *LogSetting `yaml:"UPF" valid:"optional"`
	NEF    *LogSetting `yaml:"NEF" valid:"optional"`
	BSF    *LogSetting `yaml:"BSF" valid:"optional"`
	CHF    *LogSetting `yaml:"CHF" valid:"optional"`
	UDSF   *LogSetting `yaml:"UDSF" valid:"optional"`
	NWDAF  *LogSetting `yaml:"NWDAF" valid:"optional"`
	WEBUI  *LogSetting `yaml:"WEBUI" valid:"optional"`
	SCTPLB *LogSetting `yaml:"SCTPLB" valid:"optional"`

	Aper                         *LogSetting `yaml:"Aper" valid:"optional"`
	Util                         *LogSetting `yaml:"Util" valid:"optional"`
	MongoDBLibrary               *LogSetting `yaml:"MongoDBLibrary" valid:"optional"`
	NAS                          *LogSetting `yaml:"NAS" valid:"optional"`
	NGAP                         *LogSetting `yaml:"NGAP" valid:"optional"`
	Config5g                     *LogSetting `yaml:"Config5g" valid:"optional"`
	OpenApi                      *LogSetting `yaml:"OpenApi" valid:"optional"`
	NamfCommunication            *LogSetting `yaml:"NamfCommunication" valid:"optional"`
	NamfEventExposure            *LogSetting `yaml:"NamfEventExposure" valid:"optional"`
	NnssfNSSAIAvailability       *LogSetting `yaml:"NnssfNSSAIAvailability" valid:"optional"`
	NnssfNSSelection             *LogSetting `yaml:"NnssfNSSelection" valid:"optional"`
	NsmfEventExposure            *LogSetting `yaml:"NsmfEventExposure" valid:"optional"`
	NsmfPDUSession               *LogSetting `yaml:"NsmfPDUSession" valid:"optional"`
	NudmEventExposure            *LogSetting `yaml:"NudmEventExposure" valid:"optional"`
	NudmParameterProvision       *LogSetting `yaml:"NudmParameterProvision" valid:"optional"`
	NudmSubscriberDataManagement *LogSetting `yaml:"NudmSubscriberDataManagement" valid:"optional"`
	NudmUEAuthentication         *LogSetting `yaml:"NudmUEAuthentication" valid:"optional"`
	NudmUEContextManagement      *LogSetting `yaml:"NudmUEContextManagement" valid:"optional"`
	NudrDataRepository           *LogSetting `yaml:"NudrDataRepository" valid:"optional"`
	PFCP                         *LogSetting `yaml:"PFCP" valid:"optional"`
}

func (l *Logger) Validate() (bool, error) {
	logger := reflect.ValueOf(l).Elem()
	for i := 0; i < logger.NumField(); i++ {
		if logSetting := logger.Field(i).Interface().(*LogSetting); logSetting != nil {
			result, err := logSetting.validate()
			return result, err
		}
	}

	result, err := govalidator.ValidateStruct(l)
	return result, err
}

type LogSetting struct {
	DebugLevel string `yaml:"debugLevel" valid:"debugLevel"`
}

func (l *LogSetting) validate() (bool, error) {
	govalidator.TagMap["debugLevel"] = govalidator.Validator(func(str string) bool {
		if str == "panic" || str == "fatal" || str == "error" || str == "warn" ||
			str == "info" || str == "debug" {
			return true
		} else {
			return false
		}
	})

	result, err := govalidator.ValidateStruct(l)
	return result, err
}
