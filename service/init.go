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

	"github.com/fsnotify/fsnotify"
	"github.com/gin-contrib/cors"
	"github.com/omec-project/amf/communication"
	"github.com/omec-project/amf/consumer"
	"github.com/omec-project/amf/context"
	"github.com/omec-project/amf/eventexposure"
	"github.com/omec-project/amf/factory"
	"github.com/omec-project/amf/gmm"
	"github.com/omec-project/amf/httpcallback"
	"github.com/omec-project/amf/location"
	"github.com/omec-project/amf/logger"
	"github.com/omec-project/amf/metrics"
	"github.com/omec-project/amf/mt"
	"github.com/omec-project/amf/ngap"
	ngap_message "github.com/omec-project/amf/ngap/message"
	ngap_service "github.com/omec-project/amf/ngap/service"
	"github.com/omec-project/amf/oam"
	"github.com/omec-project/amf/producer/callback"
	"github.com/omec-project/amf/tracing"
	"github.com/omec-project/amf/util"
	aperLogger "github.com/omec-project/aper/logger"
	grpcClient "github.com/omec-project/config5g/proto/client"
	protos "github.com/omec-project/config5g/proto/sdcoreConfig"
	nasLogger "github.com/omec-project/nas/logger"
	ngapLogger "github.com/omec-project/ngap/logger"
	openapiLogger "github.com/omec-project/openapi/logger"
	"github.com/omec-project/openapi/models"
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

var RocUpdateConfigChannel chan bool

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

var (
	KeepAliveTimer      *time.Timer
	KeepAliveTimerMutex sync.Mutex
)

func init() {
	RocUpdateConfigChannel = make(chan bool)
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

	if os.Getenv("MANAGED_BY_CONFIG_POD") == "true" {
		factory.AmfConfig.Configuration.ServedGumaiList = nil
		factory.AmfConfig.Configuration.SupportTAIList = nil
		factory.AmfConfig.Configuration.PlmnSupportList = nil
		logger.InitLog.Infoln("Reading Amf related configuration from ROC")
		go manageGrpcClient(ctx, factory.AmfConfig.Configuration.WebuiUri, amf)
	} else {
		go func() {
			logger.GrpcLog.Infoln("reading Amf Configuration from Helm")
			// sending true to the channel for sending NFRegistration to NRF
			RocUpdateConfigChannel <- true
		}()
	}

	return nil
}

// manageGrpcClient connects the config pod GRPC server and subscribes the config changes.
// Then it updates AMF configuration.
func manageGrpcClient(ctx ctxt.Context, webuiUri string, amf *AMF) {
	var configChannel chan *protos.NetworkSliceResponse
	var client grpcClient.ConfClient
	var stream protos.ConfigService_NetworkSliceSubscribeClient
	var err error
	count := 0
	for {
		if client != nil {
			if client.CheckGrpcConnectivity() != "READY" {
				time.Sleep(time.Second * 30)
				count++
				if count > 5 {
					err = client.GetConfigClientConn().Close()
					if err != nil {
						logger.InitLog.Infof("failing ConfigClient is not closed properly: %+v", err)
					}
					client = nil
					count = 0
				}
				logger.InitLog.Infoln("checking the connectivity readiness")
				continue
			}

			if stream == nil {
				stream, err = client.SubscribeToConfigServer()
				if err != nil {
					logger.InitLog.Infof("failing SubscribeToConfigServer: %+v", err)
					continue
				}
			}

			if configChannel == nil {
				configChannel = client.PublishOnConfigChange(true, stream)
				logger.InitLog.Infoln("PublishOnConfigChange is triggered")
				go amf.UpdateConfig(ctx, configChannel)
				logger.InitLog.Infoln("AMF updateConfig is triggered")
			}

			time.Sleep(time.Second * 5) // Fixes (avoids) 100% CPU utilization
		} else {
			client, err = grpcClient.ConnectToConfigServer(webuiUri)
			stream = nil
			configChannel = nil
			logger.InitLog.Infoln("connecting to config server")
			if err != nil {
				logger.InitLog.Errorf("%+v", err)
			}
			continue
		}
	}
}

func (amf *AMF) WatchConfig() {
	viper.WatchConfig()
	viper.OnConfigChange(func(e fsnotify.Event) {
		logger.AppLog.Infoln("config file changed:", e.Name)
		if err := factory.UpdateConfig(factory.AmfConfig.CfgLocation); err != nil {
			logger.AppLog.Errorln("error in loading updated configuration")
		} else {
			self := context.AMF_Self()
			util.InitAmfContext(self)
			logger.AppLog.Infoln("successfully updated configuration")
		}
	})
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

	self := context.AMF_Self()
	util.InitAmfContext(self)
	if self.EnableDbStore {
		self.Drsm, err = util.InitDrsm()
		if err != nil {
			logger.InitLog.Errorf("initialise DRSM failed, %v", err.Error())
		}
	}

	addr := fmt.Sprintf("%s:%d", self.BindingIPv4, self.SBIPort)

	ngapHandler := ngap_service.NGAPHandler{
		HandleMessage:      ngap.Dispatch,
		HandleNotification: ngap.HandleSCTPNotification,
	}
	ngap_service.Run(self.NgapIpList, self.NgapPort, ngapHandler)
	ctx := ctxt.Background()
	go amf.SendNFProfileUpdateToNrf(ctx)

	if self.EnableNrfCaching {
		logger.InitLog.Infoln("enable NRF caching feature")
		nrfCache.InitNrfCaching(self.NrfCacheEvictionInterval*time.Second, consumer.SendNfDiscoveryToNrf)
	}

	if self.EnableSctpLb {
		go StartGrpcServer(ctx, self.SctpGrpcPort)
	}

	if self.EnableDbStore {
		go context.SetupAmfCollection()
	}

	var tracerProvider *sdktrace.TracerProvider

	signalChannel := make(chan os.Signal, 1)
	signal.Notify(signalChannel, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-signalChannel
		amf.Terminate(tracerProvider)
		os.Exit(0)
	}()

	if factory.AmfConfig.Configuration.Telemetry != nil && factory.AmfConfig.Configuration.Telemetry.Enabled {
		tracerProvider, err = tracing.InitTracer(ctx, tracing.TelemetryConfig{
			OTLPEndpoint:   factory.AmfConfig.Configuration.Telemetry.OtlpEndpoint,
			ServiceName:    "amf",
			ServiceVersion: factory.AmfConfig.Info.Version,
			Ratio:          *factory.AmfConfig.Configuration.Telemetry.Ratio,
		})
		if err != nil {
			logger.InitLog.Panic("could not initialize tracer", zap.Error(err))
		}
		logger.InitLog.Infoln("tracer initialized successfully")
	}

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
func (amf *AMF) Terminate(tracerProvider *sdktrace.TracerProvider) {
	logger.InitLog.Infoln("terminating AMF")
	amfSelf := context.AMF_Self()

	ctx := ctxt.Background()

	// TODO: forward registered UE contexts to target AMF in the same AMF set if there is one

	// deregister with NRF
	problemDetails, err := consumer.SendDeregisterNFInstance(ctx)
	if problemDetails != nil {
		logger.InitLog.Errorf("deregister NF instance Failed Problem[%+v]", problemDetails)
	} else if err != nil {
		logger.InitLog.Errorf("deregister NF instance Error[%+v]", err)
	} else {
		logger.InitLog.Infoln("deregister from NRF successfully")
	}

	// send AMF status indication to ran to notify ran that this AMF will be unavailable
	logger.InitLog.Infoln("send AMF Status Indication to Notify RANs due to AMF terminating")
	unavailableGuamiList := ngap_message.BuildUnavailableGUAMIList(amfSelf.ServedGuamiList)
	amfSelf.AmfRanPool.Range(func(key, value interface{}) bool {
		ran := value.(*context.AmfRan)
		ngap_message.SendAMFStatusIndication(ran, unavailableGuamiList)
		return true
	})

	ngap_service.Stop()

	callback.SendAmfStatusChangeNotify((string)(models.StatusChange_UNAVAILABLE), amfSelf.ServedGuamiList)

	amfSelf.NfStatusSubscriptions.Range(func(nfInstanceId, v interface{}) bool {
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
	if tracerProvider != nil {
		err := tracerProvider.Shutdown(ctx)
		if err != nil {
			logger.InitLog.Error("failed to shutdown tracer", zap.Error(err))
		} else {
			logger.InitLog.Infoln("tracer shutdown successfully")
		}
	}

	logger.InitLog.Infoln("AMF terminated")
}

func (amf *AMF) StartKeepAliveTimer(nfProfile models.NfProfile) {
	KeepAliveTimerMutex.Lock()
	defer KeepAliveTimerMutex.Unlock()
	amf.StopKeepAliveTimer()
	if nfProfile.HeartBeatTimer == 0 {
		nfProfile.HeartBeatTimer = 60
	}
	logger.InitLog.Infof("started KeepAlive Timer: %v sec", nfProfile.HeartBeatTimer)
	// AfterFunc starts timer and waits for KeepAliveTimer to elapse and then calls amf.UpdateNF function
	KeepAliveTimer = time.AfterFunc(time.Duration(nfProfile.HeartBeatTimer)*time.Second, amf.UpdateNF)
}

func (amf *AMF) StopKeepAliveTimer() {
	if KeepAliveTimer != nil {
		logger.InitLog.Infof("stopped KeepAlive Timer")
		KeepAliveTimer.Stop()
		KeepAliveTimer = nil
	}
}

func (amf *AMF) BuildAndSendRegisterNFInstance(ctx ctxt.Context) (models.NfProfile, error) {
	self := context.AMF_Self()
	profile, err := consumer.BuildNFInstance(self)
	if err != nil {
		logger.InitLog.Errorf("build AMF Profile Error: %v", err)
		return profile, err
	}
	logger.InitLog.Infof("AMF Profile Registering to NRF: %v", profile)
	// Indefinite attempt to register until success
	profile, _, self.NfId, err = consumer.SendRegisterNFInstance(ctx, self.NrfUri, self.NfId, profile)
	return profile, err
}

// UpdateNF is the callback function, this is called when keepalivetimer elapsed
func (amf *AMF) UpdateNF() {
	ctx := ctxt.Background()
	KeepAliveTimerMutex.Lock()
	defer KeepAliveTimerMutex.Unlock()
	if KeepAliveTimer == nil {
		logger.InitLog.Warnf("KeepAlive timer has been stopped")
		return
	}
	// setting default value 30 sec
	var heartBeatTimer int32 = 60
	pitem := models.PatchItem{
		Op:    "replace",
		Path:  "/nfStatus",
		Value: "REGISTERED",
	}
	var patchItem []models.PatchItem
	patchItem = append(patchItem, pitem)
	nfProfile, problemDetails, err := consumer.SendUpdateNFInstance(patchItem)
	if problemDetails != nil {
		logger.InitLog.Errorf("AMF update to NRF ProblemDetails[%v]", problemDetails)
		// 5xx response from NRF, 404 Not Found, 400 Bad Request
		if (problemDetails.Status/100) == 5 ||
			problemDetails.Status == 404 || problemDetails.Status == 400 {
			// register with NRF full profile
			nfProfile, err = amf.BuildAndSendRegisterNFInstance(ctx)
			if err != nil {
				logger.InitLog.Errorf("could not register to NRF Error[%s]", err.Error())
			}
		}
	} else if err != nil {
		logger.InitLog.Errorf("AMF update to NRF Error[%s]", err.Error())
		nfProfile, err = amf.BuildAndSendRegisterNFInstance(ctx)
		if err != nil {
			logger.InitLog.Errorf("could not register to NRF Error[%s]", err.Error())
		}
	}

	if nfProfile.HeartBeatTimer != 0 {
		// use hearbeattimer value with received timer value from NRF
		heartBeatTimer = nfProfile.HeartBeatTimer
	}
	logger.InitLog.Debugf("restarted KeepAlive Timer: %v sec", heartBeatTimer)
	// restart timer with received HeartBeatTimer value
	KeepAliveTimer = time.AfterFunc(time.Duration(heartBeatTimer)*time.Second, amf.UpdateNF)
}

func (amf *AMF) UpdateAmfConfiguration(plmn factory.PlmnSupportItem, taiList []models.Tai, opType protos.OpType) {
	var plmnFound bool
	for plmnindex, p := range factory.AmfConfig.Configuration.PlmnSupportList {
		if p.PlmnId == plmn.PlmnId {
			plmnFound = true
			var found bool
			nssai_r := plmn.SNssaiList[0]
			for i, nssai := range p.SNssaiList {
				if nssai_r == nssai {
					found = true
					if opType == protos.OpType_SLICE_DELETE {
						factory.AmfConfig.Configuration.PlmnSupportList[plmnindex].SNssaiList = append(factory.AmfConfig.Configuration.PlmnSupportList[plmnindex].SNssaiList[:i], p.SNssaiList[i+1:]...)
						if len(factory.AmfConfig.Configuration.PlmnSupportList[plmnindex].SNssaiList) == 0 {
							factory.AmfConfig.Configuration.PlmnSupportList = append(factory.AmfConfig.Configuration.PlmnSupportList[:plmnindex],
								factory.AmfConfig.Configuration.PlmnSupportList[plmnindex+1:]...)

							factory.AmfConfig.Configuration.ServedGumaiList = append(factory.AmfConfig.Configuration.ServedGumaiList[:plmnindex],
								factory.AmfConfig.Configuration.ServedGumaiList[plmnindex+1:]...)
						}
					}
					break
				}
			}

			if !found && opType != protos.OpType_SLICE_DELETE {
				logger.GrpcLog.Infoln("plmn found but slice not found in AMF Configuration")
				factory.AmfConfig.Configuration.PlmnSupportList[plmnindex].SNssaiList = append(factory.AmfConfig.Configuration.PlmnSupportList[plmnindex].SNssaiList, nssai_r)
			}
			break
		}
	}

	guami := models.Guami{PlmnId: &plmn.PlmnId, AmfId: "cafe00"}
	if !plmnFound && opType != protos.OpType_SLICE_DELETE {
		factory.AmfConfig.Configuration.PlmnSupportList = append(factory.AmfConfig.Configuration.PlmnSupportList, plmn)
		factory.AmfConfig.Configuration.ServedGumaiList = append(factory.AmfConfig.Configuration.ServedGumaiList, guami)
	}
	logger.GrpcLog.Infof("SupportedPlmnLIst: %v, SupportGuamiLIst: %v received fromRoc", plmn, guami)
	logger.GrpcLog.Infof("SupportedPlmnLIst: %v, SupportGuamiLIst: %v in AMF", factory.AmfConfig.Configuration.PlmnSupportList,
		factory.AmfConfig.Configuration.ServedGumaiList)
	// same plmn received but Tacs in gnb updated
	nssai_r := plmn.SNssaiList[0]
	slice := strconv.FormatInt(int64(nssai_r.Sst), 10) + nssai_r.Sd
	delete(factory.AmfConfig.Configuration.SliceTaiList, slice)
	if opType != protos.OpType_SLICE_DELETE {
		// maintaining slice level tai List
		if factory.AmfConfig.Configuration.SliceTaiList == nil {
			factory.AmfConfig.Configuration.SliceTaiList = make(map[string][]models.Tai)
		}
		factory.AmfConfig.Configuration.SliceTaiList[slice] = taiList
	}

	amf.UpdateSupportedTaiList()
	logger.GrpcLog.Infoln("gnb updated in existing Plmn, SupportTAILIst received from Roc: ", taiList)
	logger.GrpcLog.Infoln("SupportTAILIst in AMF", factory.AmfConfig.Configuration.SupportTAIList)
}

func (amf *AMF) UpdateSupportedTaiList() {
	factory.AmfConfig.Configuration.SupportTAIList = nil
	for _, slice := range factory.AmfConfig.Configuration.SliceTaiList {
		for _, tai := range slice {
			logger.GrpcLog.Infoln("Tai list present in Slice", tai, factory.AmfConfig.Configuration.SupportTAIList)
			factory.AmfConfig.Configuration.SupportTAIList = append(factory.AmfConfig.Configuration.SupportTAIList, tai)
		}
	}
}

func (amf *AMF) UpdateConfig(ctx ctxt.Context, commChannel chan *protos.NetworkSliceResponse) bool {
	for rsp := range commChannel {
		logger.GrpcLog.Infof("received updateConfig in the amf app: %v", rsp)
		var tai []models.Tai
		for _, ns := range rsp.NetworkSlice {
			var snssai *models.Snssai
			logger.GrpcLog.Infoln("network Slice Name", ns.Name)
			if ns.Nssai != nil {
				snssai = new(models.Snssai)
				val, err := strconv.ParseInt(ns.Nssai.Sst, 10, 64)
				if err != nil {
					logger.GrpcLog.Errorln(err)
				}
				snssai.Sst = int32(val)
				snssai.Sd = ns.Nssai.Sd
			}
			// inform connected UEs with update slices
			if len(ns.DeletedImsis) > 0 {
				HandleImsiDeleteFromNetworkSlice(ctx, ns)
			}
			//TODO Inform connected UEs with update Slice
			/*if len(ns.AddUpdatedImsis) > 0 {
				HandleImsiAddInNetworkSlice(ns)
			}*/

			if ns.Site != nil {
				site := ns.Site
				logger.GrpcLog.Infoln("network Slice has site name:", site.SiteName)
				if site.Plmn != nil {
					plmn := new(factory.PlmnSupportItem)

					logger.GrpcLog.Infoln("Plmn mcc", site.Plmn.Mcc)
					plmn.PlmnId.Mnc = site.Plmn.Mnc
					plmn.PlmnId.Mcc = site.Plmn.Mcc

					if ns.Nssai != nil {
						plmn.SNssaiList = append(plmn.SNssaiList, *snssai)
					}
					if site.Gnb != nil {
						for _, gnb := range site.Gnb {
							var t models.Tai
							t.PlmnId = new(models.PlmnId)
							t.PlmnId.Mnc = site.Plmn.Mnc
							t.PlmnId.Mcc = site.Plmn.Mcc
							t.Tac = strconv.Itoa(int(gnb.Tac))
							tai = append(tai, t)
						}
					}

					amf.UpdateAmfConfiguration(*plmn, tai, ns.OperationType)
				} else {
					logger.GrpcLog.Infoln("Plmn not present in the message")
				}
			}
		}

		// Update PlmnSupportList/ServedGuamiList/ServedTAIList in Amf Config
		// factory.AmfConfig.Configuration.ServedGumaiList = nil
		// factory.AmfConfig.Configuration.PlmnSupportList = nil
		if len(factory.AmfConfig.Configuration.ServedGumaiList) > 0 {
			RocUpdateConfigChannel <- true
		}
		factory.AmfConfig.Rcvd = true
	}
	return true
}

func (amf *AMF) SendNFProfileUpdateToNrf(ctx ctxt.Context) {
	// for rocUpdateConfig := range RocUpdateConfigChannel {
	for rocUpdateConfig := range RocUpdateConfigChannel {
		if rocUpdateConfig {
			self := context.AMF_Self()
			util.InitAmfContext(self)

			// Register to NRF with Updated Profile
			var profile models.NfProfile
			if profileTmp, err := consumer.BuildNFInstance(self); err != nil {
				logger.CfgLog.Errorf("build AMF Profile Error: %v", err)
				continue
			} else {
				profile = profileTmp
			}

			if prof, _, nfId, err := consumer.SendRegisterNFInstance(ctx, self.NrfUri, self.NfId, profile); err != nil {
				logger.CfgLog.Warnf("send Register NF Instance with updated profile failed: %+v", err)
			} else {
				// stop keepAliveTimer if its running and start the timer
				amf.StartKeepAliveTimer(prof)
				self.NfId = nfId
				logger.CfgLog.Infoln("sent Register NF Instance with updated profile")
			}
		}
	}
}

func UeConfigSliceDeleteHandler(ctx ctxt.Context, supi, sst, sd string, msg interface{}) {
	amfSelf := context.AMF_Self()
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
	amfSelf := context.AMF_Self()
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
	var ue *context.AmfUe
	var ok bool
	logger.CfgLog.Infof("handle Subscribers Delete From Network Slice [sst:%v sd:%v]", slice.Nssai.Sst, slice.Nssai.Sd)

	for _, supi := range slice.DeletedImsis {
		amfSelf := context.AMF_Self()
		ue, ok = amfSelf.AmfUeFindBySupi(IMSI_PREFIX + supi)
		if !ok {
			logger.CfgLog.Infof("the UE [%v] is not Registered with the 5G-Core", supi)
			continue
		}
		// publish the event to ue channel
		configMsg := context.ConfigMsg{
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
	var ue *context.AmfUe
	var ok bool
	logger.CfgLog.Infof("handle Subscribers Added in Network Slice [sst:%v sd:%v]", slice.Nssai.Sst, slice.Nssai.Sd)

	for _, supi := range slice.AddUpdatedImsis {
		amfSelf := context.AMF_Self()
		ue, ok = amfSelf.AmfUeFindBySupi(IMSI_PREFIX + supi)
		if !ok {
			logger.CfgLog.Infof("the UE [%v] is not Registered with the 5G-Core", supi)
			continue
		}
		// publish the event to ue channel
		configMsg := context.ConfigMsg{
			Supi: supi,
			Msg:  slice,
			Sst:  slice.Nssai.Sst,
			Sd:   slice.Nssai.Sd,
		}

		ue.EventChannel.UpdateConfigHandler(UeConfigSliceAddHandler)
		ue.EventChannel.SubmitMessage(configMsg)
	}
}
