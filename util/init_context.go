// SPDX-FileCopyrightText: 2024 Intel Corporation
// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

package util

import (
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/omec-project/amf/context"
	"github.com/omec-project/amf/factory"
	"github.com/omec-project/amf/logger"
	"github.com/omec-project/nas/security"
	"github.com/omec-project/openapi/models"
	"github.com/omec-project/util/drsm"
)

func InitDrsm() (drsm.DrsmInterface, error) {
	podname := os.Getenv("HOSTNAME")
	podip := os.Getenv("POD_IP")
	logger.UtilLog.Infof("NfId Instance: %v", context.AMF_Self().NfId)
	podId := drsm.PodId{PodName: podname, PodInstance: context.AMF_Self().NfId, PodIp: podip}
	logger.UtilLog.Debugf("PodId: %v", podId)
	dbUrl := "mongodb://mongodb-arbiter-headless"
	if factory.AmfConfig.Configuration.Mongodb != nil &&
		factory.AmfConfig.Configuration.Mongodb.Url != "" {
		dbUrl = factory.AmfConfig.Configuration.Mongodb.Url
	}
	opt := &drsm.Options{ResIdSize: 24, Mode: drsm.ResourceClient}
	db := drsm.DbInfo{Url: dbUrl, Name: factory.AmfConfig.Configuration.AmfDBName}

	// amfid is being used for amfngapid, subscriberid and tmsi for this release
	return drsm.InitDRSM("amfid", podId, db, opt)
}

func InitAmfContext(amfContext *context.AMFContext) {
	config := factory.AmfConfig
	logger.UtilLog.Infof("amfconfig Info: Version[%s] Description[%s]", config.Info.Version, config.Info.Description)
	configuration := config.Configuration
	if amfContext.NfId == "" {
		amfContext.NfId = uuid.New().String()
	}

	if configuration.AmfName != "" {
		amfContext.Name = configuration.AmfName
	}
	amfContext.NgapIpList = []string{"127.0.0.1"} // default localhost
	if configuration.NgapIpList != nil {
		amfContext.NgapIpList = configuration.NgapIpList
	}
	amfContext.NgapPort = configuration.NgapPort
	amfContext.SctpGrpcPort = configuration.SctpGrpcPort
	sbi := configuration.Sbi
	if sbi.Scheme != "" {
		amfContext.UriScheme = models.UriScheme(sbi.Scheme)
	} else {
		logger.UtilLog.Warnln("SBI scheme has not been set. Using http as default")
		amfContext.UriScheme = "http"
	}
	amfContext.RegisterIPv4 = factory.AMF_DEFAULT_IPV4 // default localhost
	amfContext.SBIPort = factory.AMF_DEFAULT_PORT_INT  // default port
	if sbi != nil {
		if sbi.RegisterIPv4 != "" {
			amfContext.RegisterIPv4 = os.Getenv("POD_IP")
		}
		if sbi.Port != 0 {
			amfContext.SBIPort = sbi.Port
		}
		if tls := sbi.TLS; tls != nil {
			if tls.Key != "" {
				amfContext.Key = tls.Key
			}
			if tls.PEM != "" {
				amfContext.PEM = tls.PEM
			}
		}
		amfContext.BindingIPv4 = os.Getenv(sbi.BindingIPv4)
		if amfContext.BindingIPv4 != "" {
			logger.UtilLog.Infoln("parsing ServerIPv4 address from ENV Variable")
		} else {
			amfContext.BindingIPv4 = sbi.BindingIPv4
			if amfContext.BindingIPv4 == "" {
				logger.UtilLog.Warnln("error parsing ServerIPv4 address from string. Using the 0.0.0.0 as default")
				amfContext.BindingIPv4 = "0.0.0.0"
			}
		}
	}
	serviceNameList := configuration.ServiceNameList
	amfContext.InitNFService(serviceNameList, config.Info.Version)
	amfContext.ServedGuamiList = []models.Guami{}
	amfContext.SupportTaiLists = []models.Tai{}
	amfContext.PlmnSupportList = []models.PlmnSnssai{}
	amfContext.SupportDnnLists = configuration.SupportDnnList
	if configuration.NrfUri != "" {
		amfContext.NrfUri = configuration.NrfUri
	} else {
		logger.UtilLog.Warnln("NRF Uri is empty! Using localhost as NRF IPv4 address")
		amfContext.NrfUri = factory.AMF_DEFAULT_NRFURI
	}
	security := configuration.Security
	if security != nil {
		amfContext.SecurityAlgorithm.IntegrityOrder = getIntAlgOrder(security.IntegrityOrder)
		amfContext.SecurityAlgorithm.CipheringOrder = getEncAlgOrder(security.CipheringOrder)
	}
	amfContext.NetworkName = configuration.NetworkName
	amfContext.T3502Value = configuration.T3502Value
	amfContext.T3512Value = configuration.T3512Value
	amfContext.Non3gppDeregistrationTimerValue = configuration.Non3gppDeregistrationTimerValue
	amfContext.T3513Cfg = configuration.T3513
	amfContext.T3522Cfg = configuration.T3522
	amfContext.T3550Cfg = configuration.T3550
	amfContext.T3560Cfg = configuration.T3560
	amfContext.T3565Cfg = configuration.T3565
	amfContext.EnableSctpLb = configuration.EnableSctpLb
	amfContext.EnableDbStore = configuration.EnableDbStore
	amfContext.EnableNrfCaching = configuration.EnableNrfCaching
	if configuration.EnableNrfCaching {
		if configuration.NrfCacheEvictionInterval == 0 {
			amfContext.NrfCacheEvictionInterval = time.Duration(900) // 15 mins
		} else {
			amfContext.NrfCacheEvictionInterval = time.Duration(configuration.NrfCacheEvictionInterval)
		}
	}
}

func getIntAlgOrder(integrityOrder []string) (intOrder []uint8) {
	for _, intAlg := range integrityOrder {
		switch intAlg {
		case "NIA0":
			intOrder = append(intOrder, security.AlgIntegrity128NIA0)
		case "NIA1":
			intOrder = append(intOrder, security.AlgIntegrity128NIA1)
		case "NIA2":
			intOrder = append(intOrder, security.AlgIntegrity128NIA2)
		case "NIA3":
			intOrder = append(intOrder, security.AlgIntegrity128NIA3)
		default:
			logger.UtilLog.Errorf("unsupported algorithm: %s", intAlg)
		}
	}
	return
}

func getEncAlgOrder(cipheringOrder []string) (encOrder []uint8) {
	for _, encAlg := range cipheringOrder {
		switch encAlg {
		case "NEA0":
			encOrder = append(encOrder, security.AlgCiphering128NEA0)
		case "NEA1":
			encOrder = append(encOrder, security.AlgCiphering128NEA1)
		case "NEA2":
			encOrder = append(encOrder, security.AlgCiphering128NEA2)
		case "NEA3":
			encOrder = append(encOrder, security.AlgCiphering128NEA3)
		default:
			logger.UtilLog.Errorf("unsupported algorithm: %s", encAlg)
		}
	}
	return
}
