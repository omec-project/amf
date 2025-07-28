// SPDX-FileCopyrightText: 2022-present Intel Corporation
// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

package service

import (
	ctxt "context"
	"fmt"
	"net/http"
	_ "net/http/pprof" // Using package only for invoking initialization.
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/omec-project/amf/communication"
	"github.com/omec-project/amf/consumer"
	amfContext "github.com/omec-project/amf/context"
	"github.com/omec-project/amf/eventexposure"
	"github.com/omec-project/amf/factory"
	"github.com/omec-project/amf/gmm"
	"github.com/omec-project/amf/httpcallback"
	"github.com/omec-project/amf/location"
	"github.com/omec-project/amf/logger"
	"github.com/omec-project/amf/metrics"
	"github.com/omec-project/amf/mt"
	"github.com/omec-project/amf/nfregistration"
	"github.com/omec-project/amf/ngap"
	ngap_message "github.com/omec-project/amf/ngap/message"
	ngap_service "github.com/omec-project/amf/ngap/service"
	"github.com/omec-project/amf/oam"
	"github.com/omec-project/amf/polling"
	"github.com/omec-project/amf/producer/callback"
	"github.com/omec-project/amf/tracing"
	"github.com/omec-project/amf/util"
	aperLogger "github.com/omec-project/aper/logger"
	protos "github.com/omec-project/config5g/proto/sdcoreConfig"
	nasLogger "github.com/omec-project/nas/logger"
	ngapLogger "github.com/omec-project/ngap/logger"
	openapiLogger "github.com/omec-project/openapi/logger"
	"github.com/omec-project/openapi/models"
	"github.com/omec-project/openapi/nfConfigApi"
	nrfCache "github.com/omec-project/openapi/nrfcache"
	"github.com/omec-project/util/fsm"
	"github.com/omec-project/util/http2_util"
	utilLogger "github.com/omec-project/util/logger"
	"github.com/spf13/viper"
	"github.com/urfave/cli/v3"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type AMF struct{}

const IMSI_PREFIX = "imsi-"

type (
	// Config information.
	Config struct {
		cfg string
	}
)

var config Config

var amfCLi = []cli.Flag{
	&cli.StringFlag{
		Name:     "cfg",
		Usage:    "amf config file",
		Required: true,
	},
}

func (*AMF) GetCliCmd() (flags []cli.Flag) {
	return amfCLi
}

func (amf *AMF) Initialize(ctx ctxt.Context, c *cli.Command) error {
	config = Config{
		cfg: c.String("cfg"),
	}

	absPath, err := filepath.Abs(config.cfg)
	if err != nil {
		logger.CfgLog.Errorln(err)
		return err
	}

	if err := factory.InitConfigFactory(absPath); err != nil {
		return err
	}

	amf.setLogLevel()

	// Initiating a server for profiling
	if factory.AmfConfig.Configuration.DebugProfilePort != 0 {
		addr := fmt.Sprintf(":%d", factory.AmfConfig.Configuration.DebugProfilePort)
		go func() {
			if err := http.ListenAndServe(addr, nil); err != nil {
				logger.InitLog.Errorln(err)
			}
		}()
	}

	if err := factory.CheckConfigVersion(); err != nil {
		return err
	}

	if _, err := os.Stat(absPath); err == nil {
		viper.SetConfigFile(absPath)
		viper.SetConfigType("yaml")
		err = viper.ReadInConfig() // Find and read the config file
		if err != nil {            // Handle errors reading the config file
			return err
		}
	} else if os.IsNotExist(err) {
		logger.AppLog.Errorln("file %s does not exists", absPath)
		return err
	}

	factory.AmfConfig.CfgLocation = absPath

	return nil
}

func (amf *AMF) setLogLevel() {
	if factory.AmfConfig.Logger == nil {
		logger.InitLog.Warnln("AMF config without log level setting")
		return
	}

	if factory.AmfConfig.Logger.AMF != nil {
		if factory.AmfConfig.Logger.AMF.DebugLevel != "" {
			if level, err := zapcore.ParseLevel(factory.AmfConfig.Logger.AMF.DebugLevel); err != nil {
				logger.InitLog.Warnf("AMF Log level [%s] is invalid, set to [info] level",
					factory.AmfConfig.Logger.AMF.DebugLevel)
				logger.SetLogLevel(zap.InfoLevel)
			} else {
				logger.InitLog.Infof("AMF Log level is set to [%s] level", level)
				logger.SetLogLevel(level)
			}
		} else {
			logger.InitLog.Warnln("AMF Log level not set. Default set to [info] level")
			logger.SetLogLevel(zap.InfoLevel)
		}
	}

	if factory.AmfConfig.Logger.NAS != nil {
		if factory.AmfConfig.Logger.NAS.DebugLevel != "" {
			if level, err := zapcore.ParseLevel(factory.AmfConfig.Logger.NAS.DebugLevel); err != nil {
				nasLogger.NasLog.Warnf("NAS Log level [%s] is invalid, set to [info] level",
					factory.AmfConfig.Logger.NAS.DebugLevel)
				logger.SetLogLevel(zap.InfoLevel)
			} else {
				nasLogger.SetLogLevel(level)
			}
		} else {
			nasLogger.NasLog.Warnln("NAS Log level not set. Default set to [info] level")
			nasLogger.SetLogLevel(zap.InfoLevel)
		}
	}

	if factory.AmfConfig.Logger.NGAP != nil {
		if factory.AmfConfig.Logger.NGAP.DebugLevel != "" {
			if level, err := zapcore.ParseLevel(factory.AmfConfig.Logger.NGAP.DebugLevel); err != nil {
				ngapLogger.NgapLog.Warnf("NGAP Log level [%s] is invalid, set to [info] level",
					factory.AmfConfig.Logger.NGAP.DebugLevel)
				ngapLogger.SetLogLevel(zap.InfoLevel)
			} else {
				ngapLogger.SetLogLevel(level)
			}
		} else {
			ngapLogger.NgapLog.Warnln("NGAP Log level not set. Default set to [info] level")
			ngapLogger.SetLogLevel(zap.InfoLevel)
		}
	}

	if factory.AmfConfig.Logger.Aper != nil {
		if factory.AmfConfig.Logger.Aper.DebugLevel != "" {
			if level, err := zapcore.ParseLevel(factory.AmfConfig.Logger.Aper.DebugLevel); err != nil {
				aperLogger.AperLog.Warnf("Aper Log level [%s] is invalid, set to [info] level",
					factory.AmfConfig.Logger.Aper.DebugLevel)
				aperLogger.SetLogLevel(zap.InfoLevel)
			} else {
				aperLogger.SetLogLevel(level)
			}
		} else {
			aperLogger.AperLog.Warnln("Aper Log level not set. Default set to [info] level")
			aperLogger.SetLogLevel(zap.InfoLevel)
		}
	}

	if factory.AmfConfig.Logger.OpenApi != nil {
		if factory.AmfConfig.Logger.OpenApi.DebugLevel != "" {
			if level, err := zapcore.ParseLevel(factory.AmfConfig.Logger.OpenApi.DebugLevel); err != nil {
				openapiLogger.OpenapiLog.Warnf("OpenApi Log level [%s] is invalid, set to [info] level",
					factory.AmfConfig.Logger.OpenApi.DebugLevel)
				openapiLogger.SetLogLevel(zap.InfoLevel)
			} else {
				openapiLogger.SetLogLevel(level)
			}
		} else {
			openapiLogger.OpenapiLog.Warnln("OpenApi Log level not set. Default set to [info] level")
			openapiLogger.SetLogLevel(zap.InfoLevel)
		}
	}

	if factory.AmfConfig.Logger.Util != nil {
		if factory.AmfConfig.Logger.Util.DebugLevel != "" {
			if level, err := zapcore.ParseLevel(factory.AmfConfig.Logger.Util.DebugLevel); err != nil {
				utilLogger.UtilLog.Warnf("Util (drsm, fsm, etc.) Log level [%s] is invalid, set to [info] level",
					factory.AmfConfig.Logger.Util.DebugLevel)
				utilLogger.SetLogLevel(zap.InfoLevel)
			} else {
				utilLogger.SetLogLevel(level)
			}
		} else {
			utilLogger.UtilLog.Warnln("Util (drsm, fsm, etc.) Log level not set. Default set to [info] level")
			utilLogger.SetLogLevel(zap.InfoLevel)
		}
	}
}

func (amf *AMF) FilterCli(c *cli.Command) (args []string) {
	for _, flag := range amf.GetCliCmd() {
		name := flag.Names()[0]
		value := fmt.Sprint(c.Generic(name))
		if value == "" {
			continue
		}

		args = append(args, "--"+name, value)
	}
	return args
}

func (amf *AMF) Start() {
	logger.InitLog.Infoln("server started")
	var err error

	router := utilLogger.NewGinWithZap(logger.GinLog)
	router.Use(cors.New(cors.Config{
		AllowMethods: []string{"GET", "POST", "OPTIONS", "PUT", "PATCH", "DELETE"},
		AllowHeaders: []string{
			"Origin", "Content-Length", "Content-Type", "User-Agent", "Referrer", "Host",
			"Token", "X-Requested-With",
		},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		AllowAllOrigins:  true,
		MaxAge:           86400,
	}))

	httpcallback.AddService(router)
	oam.AddService(router)
	for _, serviceName := range factory.AmfConfig.Configuration.ServiceNameList {
		switch models.ServiceName(serviceName) {
		case models.ServiceName_NAMF_COMM:
			communication.AddService(router)
		case models.ServiceName_NAMF_EVTS:
			eventexposure.AddService(router)
		case models.ServiceName_NAMF_MT:
			mt.AddService(router)
		case models.ServiceName_NAMF_LOC:
			location.AddService(router)
		}
	}

	go metrics.InitMetrics()

	if err = metrics.InitialiseKafkaStream(factory.AmfConfig.Configuration); err != nil {
		logger.InitLog.Errorf("initialise kafka stream failed, %v ", err.Error())
	}

	self := amfContext.AMF_Self()
	util.InitAmfContext(self)
	if self.EnableDbStore {
		self.Drsm, err = util.InitDrsm()
		if err != nil {
			logger.InitLog.Errorf("initialise DRSM failed, %v", err.Error())
		}
	}
	ctx, cancelServices := ctxt.WithCancel(ctxt.Background())

	if factory.AmfConfig.Configuration.Telemetry != nil && factory.AmfConfig.Configuration.Telemetry.Enabled {
		var tp *sdktrace.TracerProvider
		tp, err = tracing.InitTracer(ctx, tracing.TelemetryConfig{
			OTLPEndpoint:   factory.AmfConfig.Configuration.Telemetry.OtlpEndpoint,
			ServiceName:    "amf",
			ServiceVersion: factory.AmfConfig.Info.Version,
			Ratio:          *factory.AmfConfig.Configuration.Telemetry.Ratio,
		})
		if err != nil {
			logger.InitLog.Panic("could not initialize tracer", zap.Error(err))
		}
		logger.InitLog.Infoln("tracer initialized successfully")
		defer func() {
			err = tp.Shutdown(ctx)
			if err != nil {
				logger.InitLog.Error("failed to shutdown tracer", zap.Error(err))
			} else {
				logger.InitLog.Infoln("tracer shutdown successfully")
			}
		}()
	}

	registrationChan := make(chan []nfConfigApi.AccessAndMobility, 100)
	contextUpdateChan := make(chan []nfConfigApi.AccessAndMobility, 100)
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		polling.StartPollingService(ctx, factory.AmfConfig.Configuration.WebuiUri, registrationChan, contextUpdateChan)
	}()
	go func() {
		defer wg.Done()
		nfregistration.StartNfRegistrationService(ctx, registrationChan)
	}()

	// Update AMF context using polled config
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case cfg := <-contextUpdateChan:
				err = amfContext.UpdateAmfContext(self, cfg)
				if err != nil {
					logger.PollConfigLog.Errorf("AMF context update failed: %v", err)
				} else {
					logger.PollConfigLog.Debugln("AMF context updated from WebConsole config")
				}
			}
		}
	}()

	addr := fmt.Sprintf("%s:%d", self.BindingIPv4, self.SBIPort)

	ngapHandler := ngap_service.NGAPHandler{
		HandleMessage:      ngap.Dispatch,
		HandleNotification: ngap.HandleSCTPNotification,
	}
	ngap_service.Run(self.NgapIpList, self.NgapPort, ngapHandler)

	if self.EnableNrfCaching {
		logger.InitLog.Infoln("enable NRF caching feature")
		nrfCache.InitNrfCaching(self.NrfCacheEvictionInterval*time.Second, consumer.SendNfDiscoveryToNrf)
	}

	if self.EnableSctpLb {
		go StartGrpcServer(ctx, self.SctpGrpcPort)
	}

	if self.EnableDbStore {
		go amfContext.SetupAmfCollection()
	}

	signalChannel := make(chan os.Signal, 1)
	signal.Notify(signalChannel, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-signalChannel
		amf.Terminate(cancelServices, &wg)
		os.Exit(0)
	}()

	sslLog := filepath.Dir(factory.AmfConfig.CfgLocation) + "/sslkey.log"
	server, err := http2_util.NewServer(addr, sslLog, router)

	if server == nil {
		logger.InitLog.Errorf("initialize HTTP server failed: %+v", err)
		return
	}

	if err != nil {
		logger.InitLog.Warnf("initialize HTTP server: %+v", err)
	}

	serverScheme := factory.AmfConfig.Configuration.Sbi.Scheme
	switch serverScheme {
	case "http":
		err = server.ListenAndServe()
	case "https":
		err = server.ListenAndServeTLS(self.PEM, self.Key)
	default:
		logger.InitLog.Fatalf("HTTP server setup failed: invalid server scheme %+v", serverScheme)
		return
	}

	if err != nil {
		logger.InitLog.Fatalf("HTTP server setup failed: %+v", err)
	}
}

func (amf *AMF) Exec(c *cli.Command) error {
	return nil
}

// Used in AMF planned removal procedure
func (amf *AMF) Terminate(cancelServices ctxt.CancelFunc, wg *sync.WaitGroup) {
	logger.InitLog.Infoln("terminating AMF")
	amfSelf := amfContext.AMF_Self()

	ctx := ctxt.Background()
	cancelServices()
	// TODO: forward registered UE contexts to target AMF in the same AMF set if there is one

	// deregister with NRF
	nfregistration.DeregisterNF(ctx)

	// send AMF status indication to ran to notify ran that this AMF will be unavailable
	logger.InitLog.Infoln("send AMF Status Indication to Notify RANs due to AMF terminating")
	unavailableGuamiList := ngap_message.BuildUnavailableGUAMIList(amfSelf.ServedGuamiList)
	amfSelf.AmfRanPool.Range(func(key, value any) bool {
		ran := value.(*amfContext.AmfRan)
		ngap_message.SendAMFStatusIndication(ran, unavailableGuamiList)
		return true
	})

	ngap_service.Stop()

	callback.SendAmfStatusChangeNotify((string)(models.StatusChange_UNAVAILABLE), amfSelf.ServedGuamiList)

	amfSelf.NfStatusSubscriptions.Range(func(nfInstanceId, v any) bool {
		if subscriptionId, ok := amfSelf.NfStatusSubscriptions.Load(nfInstanceId); ok {
			logger.InitLog.Debugf("SubscriptionId is %v", subscriptionId.(string))
			problemDetails, err := consumer.SendRemoveSubscription(ctx, subscriptionId.(string))
			if problemDetails != nil {
				logger.InitLog.Errorf("remove NF Subscription Failed Problem[%+v]", problemDetails)
			} else if err != nil {
				logger.InitLog.Errorf("remove NF Subscription Error[%+v]", err)
			} else {
				logger.InitLog.Infoln("remove NF Subscription successful")
			}
		}
		return true
	})

	logger.InitLog.Infoln("AMF terminated")
}

func UeConfigSliceDeleteHandler(ctx ctxt.Context, supi, sst, sd string, msg interface{}) {
	amfSelf := amfContext.AMF_Self()
	ue, _ := amfSelf.AmfUeFindBySupi(IMSI_PREFIX + supi)

	// Triggers for NwInitiatedDeRegistration
	// - Only 1 Allowed Nssai is exist and its slice information matched
	ns := msg.(*protos.NetworkSlice)
	if len(ue.AllowedNssai[models.AccessType__3_GPP_ACCESS]) == 1 {
		st, err := strconv.Atoi(ns.Nssai.Sst)
		if err != nil {
			logger.CfgLog.Errorln(err)
		}
		if ue.AllowedNssai[models.AccessType__3_GPP_ACCESS][0].AllowedSnssai.Sst == int32(st) &&
			ue.AllowedNssai[models.AccessType__3_GPP_ACCESS][0].AllowedSnssai.Sd == ns.Nssai.Sd {
			err := gmm.GmmFSM.SendEvent(ctx, ue.State[models.AccessType__3_GPP_ACCESS], gmm.NwInitiatedDeregistrationEvent, fsm.ArgsType{
				gmm.ArgAmfUe:      ue,
				gmm.ArgAccessType: models.AccessType__3_GPP_ACCESS,
			})
			if err != nil {
				logger.CfgLog.Errorln(err)
			}
		} else {
			logger.CfgLog.Infoln("deleted slice not matched with slice info in UEContext")
		}
	} else {
		var Nssai models.Snssai
		st, err := strconv.Atoi(ns.Nssai.Sst)
		if err != nil {
			logger.CfgLog.Errorln(err)
		}
		Nssai.Sst = int32(st)
		Nssai.Sd = ns.Nssai.Sd
		err = gmm.GmmFSM.SendEvent(ctx, ue.State[models.AccessType__3_GPP_ACCESS], gmm.SliceInfoDeleteEvent, fsm.ArgsType{
			gmm.ArgAmfUe:      ue,
			gmm.ArgAccessType: models.AccessType__3_GPP_ACCESS,
			gmm.ArgNssai:      Nssai,
		})
		if err != nil {
			logger.CfgLog.Errorln(err)
		}
	}
}

func UeConfigSliceAddHandler(ctx ctxt.Context, supi, sst, sd string, msg interface{}) {
	amfSelf := amfContext.AMF_Self()
	ue, _ := amfSelf.AmfUeFindBySupi(IMSI_PREFIX + supi)

	ns := msg.(*protos.NetworkSlice)
	var Nssai models.Snssai
	st, err := strconv.Atoi(ns.Nssai.Sst)
	if err != nil {
		logger.CfgLog.Errorln(err)
	}
	Nssai.Sst = int32(st)
	Nssai.Sd = ns.Nssai.Sd
	err = gmm.GmmFSM.SendEvent(ctx, ue.State[models.AccessType__3_GPP_ACCESS], gmm.SliceInfoAddEvent, fsm.ArgsType{
		gmm.ArgAmfUe:      ue,
		gmm.ArgAccessType: models.AccessType__3_GPP_ACCESS,
		gmm.ArgNssai:      Nssai,
	})
	if err != nil {
		logger.CfgLog.Errorln(err)
	}
}

func HandleImsiDeleteFromNetworkSlice(ctx ctxt.Context, slice *protos.NetworkSlice) {
	var ue *amfContext.AmfUe
	var ok bool
	logger.CfgLog.Infof("handle Subscribers Delete From Network Slice [sst:%v sd:%v]", slice.Nssai.Sst, slice.Nssai.Sd)

	for _, supi := range slice.DeletedImsis {
		amfSelf := amfContext.AMF_Self()
		ue, ok = amfSelf.AmfUeFindBySupi(IMSI_PREFIX + supi)
		if !ok {
			logger.CfgLog.Infof("the UE [%v] is not Registered with the 5G-Core", supi)
			continue
		}
		// publish the event to ue channel
		configMsg := amfContext.ConfigMsg{
			Supi: supi,
			Msg:  slice,
			Sst:  slice.Nssai.Sst,
			Sd:   slice.Nssai.Sd,
		}
		ue.SetEventChannel(ctx, nil)
		ue.EventChannel.UpdateConfigHandler(UeConfigSliceDeleteHandler)
		ue.EventChannel.SubmitMessage(configMsg)
	}
}

func HandleImsiAddInNetworkSlice(ctx ctxt.Context, slice *protos.NetworkSlice) {
	var ue *amfContext.AmfUe
	var ok bool
	logger.CfgLog.Infof("handle Subscribers Added in Network Slice [sst:%v sd:%v]", slice.Nssai.Sst, slice.Nssai.Sd)

	for _, supi := range slice.AddUpdatedImsis {
		amfSelf := amfContext.AMF_Self()
		ue, ok = amfSelf.AmfUeFindBySupi(IMSI_PREFIX + supi)
		if !ok {
			logger.CfgLog.Infof("the UE [%v] is not Registered with the 5G-Core", supi)
			continue
		}
		// publish the event to ue channel
		configMsg := amfContext.ConfigMsg{
			Supi: supi,
			Msg:  slice,
			Sst:  slice.Nssai.Sst,
			Sd:   slice.Nssai.Sd,
		}

		ue.EventChannel.UpdateConfigHandler(UeConfigSliceAddHandler)
		ue.EventChannel.SubmitMessage(configMsg)
	}
}
