// Copyright (C) 2026 Intel Corporation
// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0

package factory

import (
	"fmt"
	"regexp"

	"github.com/omec-project/amf/logger"
	"github.com/omec-project/util/nfconfig"
)

var AmfConfig Config

const (
	AMFID_PATTERN = "^[A-Fa-f0-9]{6}$"
	defaultAmfID  = "cafe00"
)

// TODO: Support configuration update from REST api
func InitConfigFactory(f string) error {
	if err := nfconfig.LoadYAML(f, &AmfConfig); err != nil {
		return err
	}
	if AmfConfig.Configuration.AmfId == "" {
		AmfConfig.Configuration.AmfId = defaultAmfID
		logger.CfgLog.Infof("amfId not set in configuration file. Using %s", AmfConfig.Configuration.AmfId)
	}
	if AmfConfig.Configuration.KafkaInfo.EnableKafka == nil {
		enableKafka := true
		AmfConfig.Configuration.KafkaInfo.EnableKafka = &enableKafka
	}
	if AmfConfig.Configuration.Telemetry != nil && AmfConfig.Configuration.Telemetry.Enabled {
		if AmfConfig.Configuration.Telemetry.Ratio == nil {
			defaultRatio := 1.0
			AmfConfig.Configuration.Telemetry.Ratio = &defaultRatio
		}

		if AmfConfig.Configuration.Telemetry.OtlpEndpoint == "" {
			return fmt.Errorf("OTLP endpoint is not set in the configuration")
		}
	}
	// Resolved after the telemetry checks so that error precedence matches the old code:
	// telemetry error, then webuiUri, then amfId.
	webuiUri, err := nfconfig.ResolveWebuiUri(AmfConfig.Configuration.WebuiUri, logger.CfgLog)
	if err != nil {
		return err
	}
	AmfConfig.Configuration.WebuiUri = webuiUri
	return validateAmfId(AmfConfig.Configuration.AmfId)
}

func CheckConfigVersion() error {
	return nfconfig.CheckVersion(AmfConfig.GetVersion(), AMF_EXPECTED_CONFIG_VERSION, logger.CfgLog)
}

func validateAmfId(amfId string) error {
	amfIdMatch, err := regexp.MatchString(AMFID_PATTERN, amfId)
	if err != nil {
		return fmt.Errorf("invalid amfId: %s. It should match the following pattern: `%s`", amfId, AMFID_PATTERN)
	}
	if !amfIdMatch {
		return fmt.Errorf("invalid amfId: %s. It should match the following pattern: `%s`", amfId, AMFID_PATTERN)
	}
	return nil
}
