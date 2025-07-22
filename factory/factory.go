// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

/*
 * AMF Configuration Factory
 */

package factory

import (
	"fmt"
	"net/url"
	"os"
	"reflect"
	"sync"

	"github.com/omec-project/amf/logger"
	"gopkg.in/yaml.v2"
)

var (
	AmfConfig  Config
	ConfigLock sync.RWMutex
)

// TODO: Support configuration update from REST api
func InitConfigFactory(f string) error {
	content, err := os.ReadFile(f)
	if err != nil {
		return err
	}
	if err = yaml.Unmarshal(content, &AmfConfig); err != nil {
		return err
	}
	if AmfConfig.Configuration.WebuiUri == "" {
		AmfConfig.Configuration.WebuiUri = "http://webui:5001"
		logger.CfgLog.Infof("webuiUri not set in configuration file. Using %v", AmfConfig.Configuration.WebuiUri)
		return nil
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
	err = validateWebuiUri(AmfConfig.Configuration.WebuiUri)
	return err
}

func UpdateConfig(f string) error {
	if content, err := os.ReadFile(f); err != nil {
		return err
	} else {
		var amfConfig Config

		if yamlErr := yaml.Unmarshal(content, &amfConfig); yamlErr != nil {
			return yamlErr
		}
		// Checking which config has been changed
		if !reflect.DeepEqual(AmfConfig.Configuration.AmfName, amfConfig.Configuration.AmfName) {
			logger.CfgLog.Infoln("updated AMF Name is changed to:", amfConfig.Configuration.AmfName)
		}
		if !reflect.DeepEqual(AmfConfig.Configuration.NgapIpList, amfConfig.Configuration.NgapIpList) {
			logger.CfgLog.Infoln("updated NgapList:", amfConfig.Configuration.NgapIpList)
		}
		if !reflect.DeepEqual(AmfConfig.Configuration.Sbi, amfConfig.Configuration.Sbi) {
			logger.CfgLog.Infoln("updated Sbi:", amfConfig.Configuration.Sbi)
		}
		if !reflect.DeepEqual(AmfConfig.Configuration.NetworkFeatureSupport5GS, amfConfig.Configuration.NetworkFeatureSupport5GS) {
			logger.CfgLog.Infoln("updated NetworkFeatureSupport5GS:", amfConfig.Configuration.NetworkFeatureSupport5GS)
		}
		if !reflect.DeepEqual(AmfConfig.Configuration.ServiceNameList, amfConfig.Configuration.ServiceNameList) {
			logger.CfgLog.Infoln("updated ServiceNameList:", amfConfig.Configuration.ServiceNameList)
		}

		/* we will not update below 3 configs if its controlled by ROC */
		/* TODO: document this as dynamic configmap updates for below 3 configs we dont support if its controlled by ROC*/
		if os.Getenv("MANAGED_BY_CONFIG_POD") == "true" {
			if !reflect.DeepEqual(AmfConfig.Configuration.ServedGumaiList, amfConfig.Configuration.ServedGumaiList) {
				logger.CfgLog.Infoln("updated ServedGumaiList:", amfConfig.Configuration.ServedGumaiList)
			}
			if !reflect.DeepEqual(AmfConfig.Configuration.SupportTAIList, amfConfig.Configuration.SupportTAIList) {
				logger.CfgLog.Infoln("updated SupportTAIList:", amfConfig.Configuration.SupportTAIList)
			}
			if !reflect.DeepEqual(AmfConfig.Configuration.PlmnSupportList, amfConfig.Configuration.PlmnSupportList) {
				logger.CfgLog.Infoln("updated PlmnSupportList:", amfConfig.Configuration.PlmnSupportList)
			}
		}
		if !reflect.DeepEqual(AmfConfig.Configuration.SupportDnnList, amfConfig.Configuration.SupportDnnList) {
			logger.CfgLog.Infoln("updated SupportDnnList:", amfConfig.Configuration.SupportDnnList)
		}
		if !reflect.DeepEqual(AmfConfig.Configuration.NrfUri, amfConfig.Configuration.NrfUri) {
			logger.CfgLog.Infoln("updated NrfUri:", amfConfig.Configuration.NrfUri)
		}
		if !reflect.DeepEqual(AmfConfig.Configuration.Security, amfConfig.Configuration.Security) {
			logger.CfgLog.Infoln("updated Security:", amfConfig.Configuration.Security)
		}
		if !reflect.DeepEqual(AmfConfig.Configuration.NetworkName, amfConfig.Configuration.NetworkName) {
			logger.CfgLog.Infoln("updated NetworkName:", amfConfig.Configuration.NetworkName)
		}
		if !reflect.DeepEqual(AmfConfig.Configuration.T3502Value, amfConfig.Configuration.T3502Value) {
			logger.CfgLog.Infoln("updated T3502Value:", amfConfig.Configuration.T3502Value)
		}
		if !reflect.DeepEqual(AmfConfig.Configuration.T3512Value, amfConfig.Configuration.T3512Value) {
			logger.CfgLog.Infoln("updated T3512Value:", amfConfig.Configuration.T3512Value)
		}
		if !reflect.DeepEqual(AmfConfig.Configuration.Non3gppDeregistrationTimerValue, amfConfig.Configuration.Non3gppDeregistrationTimerValue) {
			logger.CfgLog.Infoln("updated Non3gppDeregistrationTimerValue:", amfConfig.Configuration.Non3gppDeregistrationTimerValue)
		}
		if !reflect.DeepEqual(AmfConfig.Configuration.T3513, amfConfig.Configuration.T3513) {
			logger.CfgLog.Infoln("updated T3513:", amfConfig.Configuration.T3513)
		}
		if !reflect.DeepEqual(AmfConfig.Configuration.T3522, amfConfig.Configuration.T3522) {
			logger.CfgLog.Infoln("updated T3522:", amfConfig.Configuration.T3522)
		}
		if !reflect.DeepEqual(AmfConfig.Configuration.T3550, amfConfig.Configuration.T3550) {
			logger.CfgLog.Infoln("updated T3550:", amfConfig.Configuration.T3550)
		}
		if !reflect.DeepEqual(AmfConfig.Configuration.T3560, amfConfig.Configuration.T3560) {
			logger.CfgLog.Infoln("updated T3560:", amfConfig.Configuration.T3560)
		}
		if !reflect.DeepEqual(AmfConfig.Configuration.T3565, amfConfig.Configuration.T3565) {
			logger.CfgLog.Infoln("updated T3565:", amfConfig.Configuration.T3565)
		}

		amfConfig.Rcvd = true
		AmfConfig = amfConfig
	}
	return nil
}

func CheckConfigVersion() error {
	currentVersion := AmfConfig.GetVersion()

	if currentVersion != AMF_EXPECTED_CONFIG_VERSION {
		return fmt.Errorf("config version is [%s], but expected is [%s]",
			currentVersion, AMF_EXPECTED_CONFIG_VERSION)
	}

	logger.CfgLog.Infof("config version [%s]", currentVersion)

	return nil
}

func validateWebuiUri(uri string) error {
	parsedUrl, err := url.ParseRequestURI(uri)
	if err != nil {
		return err
	}
	if parsedUrl.Scheme != "http" && parsedUrl.Scheme != "https" {
		return fmt.Errorf("unsupported scheme for webuiUri: %s", parsedUrl.Scheme)
	}
	if parsedUrl.Hostname() == "" {
		return fmt.Errorf("missing host in webuiUri")
	}
	return nil
}
