// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright (c) 2021 Intel Corporation
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

package service

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"sync"
	"syscall"

	"github.com/gin-contrib/cors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"

	"github.com/free5gc/amf/communication"
	"github.com/free5gc/amf/consumer"
	"github.com/free5gc/amf/context"
	"github.com/free5gc/amf/eventexposure"
	"github.com/free5gc/amf/factory"
	"github.com/free5gc/amf/gmm"
	"github.com/free5gc/amf/httpcallback"
	"github.com/free5gc/amf/location"
	"github.com/free5gc/amf/logger"
	"github.com/free5gc/amf/metrics"
	"github.com/free5gc/amf/mt"
	"github.com/free5gc/amf/ngap"
	ngap_message "github.com/free5gc/amf/ngap/message"
	ngap_service "github.com/free5gc/amf/ngap/service"
	"github.com/free5gc/amf/oam"
	"github.com/free5gc/amf/producer/callback"
	"github.com/free5gc/amf/util"
	aperLogger "github.com/free5gc/aper/logger"
	"github.com/free5gc/fsm"
	fsmLogger "github.com/free5gc/fsm/logger"
	"github.com/free5gc/http2_util"
	"github.com/free5gc/logger_util"
	nasLogger "github.com/free5gc/nas/logger"
	ngapLogger "github.com/free5gc/ngap/logger"
	openApiLogger "github.com/free5gc/openapi/logger"
	"github.com/free5gc/openapi/models"
	"github.com/free5gc/path_util"
	pathUtilLogger "github.com/free5gc/path_util/logger"
	"github.com/fsnotify/fsnotify"
	gClient "github.com/omec-project/config5g/proto/client"
	protos "github.com/omec-project/config5g/proto/sdcoreConfig"
	"github.com/spf13/viper"
)

type AMF struct{}

var RocUpdateConfigChannel chan bool

type (
	// Config information.
	Config struct {
		amfcfg string
	}
)

var config Config

var amfCLi = []cli.Flag{
	cli.StringFlag{
		Name:  "free5gccfg",
		Usage: "common config file",
	},
	cli.StringFlag{
		Name:  "amfcfg",
		Usage: "amf config file",
	},
}

var initLog *logrus.Entry

func init() {
	initLog = logger.InitLog
	RocUpdateConfigChannel = make(chan bool)
}

func (*AMF) GetCliCmd() (flags []cli.Flag) {
	return amfCLi
}

func (amf *AMF) Initialize(c *cli.Context) error {
	config = Config{
		amfcfg: c.String("amfcfg"),
	}

	if config.amfcfg != "" {
		if err := factory.InitConfigFactory(config.amfcfg); err != nil {
			return err
		}
	} else {
		DefaultAmfConfigPath := path_util.Free5gcPath("free5gc/config/amfcfg.yaml")
		if err := factory.InitConfigFactory(DefaultAmfConfigPath); err != nil {
			return err
		}
	}

	amf.setLogLevel()

	if err := factory.CheckConfigVersion(); err != nil {
		return err
	}

	if _, err := os.Stat("/free5gc/config/amfcfg.conf"); err == nil {
		viper.SetConfigName("amfcfg.conf")
		viper.SetConfigType("yaml")
		viper.AddConfigPath("/free5gc/config")
		err := viper.ReadInConfig() // Find and read the config file
		if err != nil {             // Handle errors reading the config file
			return err
		}
	} else if os.IsNotExist(err) {
		fmt.Println("amfcfg does not exists in /free5gc/config")
	}

	if os.Getenv("MANAGED_BY_CONFIG_POD") == "true" {
		factory.AmfConfig.Configuration.ServedGumaiList = nil
		factory.AmfConfig.Configuration.SupportTAIList = nil
		factory.AmfConfig.Configuration.PlmnSupportList = nil
		initLog.Infoln("Reading Amf related configuration from ROC")
		configChannel := gClient.ConfigWatcher()
		go amf.UpdateConfig(configChannel)
	} else {
		go func() {
			logger.GrpcLog.Infoln("Reading Amf Configuration from Helm")
			//sending true to the channel for sending NFRegistration to NRF
			RocUpdateConfigChannel <- true
		}()
	}

	return nil
}

func (amf *AMF) WatchConfig() {
	viper.WatchConfig()
	viper.OnConfigChange(func(e fsnotify.Event) {
		fmt.Println("Config file changed:", e.Name)
		if err := factory.UpdateAmfConfig("/free5gc/config/amfcfg.conf"); err != nil {
			fmt.Println("error in loading updated configuration")
		} else {
			self := context.AMF_Self()
			util.InitAmfContext(self)
			fmt.Println("successfully updated configuration")
		}
	})
}

func (amf *AMF) setLogLevel() {
	if factory.AmfConfig.Logger == nil {
		initLog.Warnln("AMF config without log level setting!!!")
		return
	}

	if factory.AmfConfig.Logger.AMF != nil {
		if factory.AmfConfig.Logger.AMF.DebugLevel != "" {
			if level, err := logrus.ParseLevel(factory.AmfConfig.Logger.AMF.DebugLevel); err != nil {
				initLog.Warnf("AMF Log level [%s] is invalid, set to [info] level",
					factory.AmfConfig.Logger.AMF.DebugLevel)
				logger.SetLogLevel(logrus.InfoLevel)
			} else {
				initLog.Infof("AMF Log level is set to [%s] level", level)
				logger.SetLogLevel(level)
			}
		} else {
			initLog.Warnln("AMF Log level not set. Default set to [info] level")
			logger.SetLogLevel(logrus.InfoLevel)
		}
		logger.SetReportCaller(factory.AmfConfig.Logger.AMF.ReportCaller)
	}

	if factory.AmfConfig.Logger.NAS != nil {
		if factory.AmfConfig.Logger.NAS.DebugLevel != "" {
			if level, err := logrus.ParseLevel(factory.AmfConfig.Logger.NAS.DebugLevel); err != nil {
				nasLogger.NasLog.Warnf("NAS Log level [%s] is invalid, set to [info] level",
					factory.AmfConfig.Logger.NAS.DebugLevel)
				logger.SetLogLevel(logrus.InfoLevel)
			} else {
				nasLogger.SetLogLevel(level)
			}
		} else {
			nasLogger.NasLog.Warnln("NAS Log level not set. Default set to [info] level")
			nasLogger.SetLogLevel(logrus.InfoLevel)
		}
		nasLogger.SetReportCaller(factory.AmfConfig.Logger.NAS.ReportCaller)
	}

	if factory.AmfConfig.Logger.NGAP != nil {
		if factory.AmfConfig.Logger.NGAP.DebugLevel != "" {
			if level, err := logrus.ParseLevel(factory.AmfConfig.Logger.NGAP.DebugLevel); err != nil {
				ngapLogger.NgapLog.Warnf("NGAP Log level [%s] is invalid, set to [info] level",
					factory.AmfConfig.Logger.NGAP.DebugLevel)
				ngapLogger.SetLogLevel(logrus.InfoLevel)
			} else {
				ngapLogger.SetLogLevel(level)
			}
		} else {
			ngapLogger.NgapLog.Warnln("NGAP Log level not set. Default set to [info] level")
			ngapLogger.SetLogLevel(logrus.InfoLevel)
		}
		ngapLogger.SetReportCaller(factory.AmfConfig.Logger.NGAP.ReportCaller)
	}

	if factory.AmfConfig.Logger.FSM != nil {
		if factory.AmfConfig.Logger.FSM.DebugLevel != "" {
			if level, err := logrus.ParseLevel(factory.AmfConfig.Logger.FSM.DebugLevel); err != nil {
				fsmLogger.FsmLog.Warnf("FSM Log level [%s] is invalid, set to [info] level",
					factory.AmfConfig.Logger.FSM.DebugLevel)
				fsmLogger.SetLogLevel(logrus.InfoLevel)
			} else {
				fsmLogger.SetLogLevel(level)
			}
		} else {
			fsmLogger.FsmLog.Warnln("FSM Log level not set. Default set to [info] level")
			fsmLogger.SetLogLevel(logrus.InfoLevel)
		}
		fsmLogger.SetReportCaller(factory.AmfConfig.Logger.FSM.ReportCaller)
	}

	if factory.AmfConfig.Logger.Aper != nil {
		if factory.AmfConfig.Logger.Aper.DebugLevel != "" {
			if level, err := logrus.ParseLevel(factory.AmfConfig.Logger.Aper.DebugLevel); err != nil {
				aperLogger.AperLog.Warnf("Aper Log level [%s] is invalid, set to [info] level",
					factory.AmfConfig.Logger.Aper.DebugLevel)
				aperLogger.SetLogLevel(logrus.InfoLevel)
			} else {
				aperLogger.SetLogLevel(level)
			}
		} else {
			aperLogger.AperLog.Warnln("Aper Log level not set. Default set to [info] level")
			aperLogger.SetLogLevel(logrus.InfoLevel)
		}
		aperLogger.SetReportCaller(factory.AmfConfig.Logger.Aper.ReportCaller)
	}

	if factory.AmfConfig.Logger.PathUtil != nil {
		if factory.AmfConfig.Logger.PathUtil.DebugLevel != "" {
			if level, err := logrus.ParseLevel(factory.AmfConfig.Logger.PathUtil.DebugLevel); err != nil {
				pathUtilLogger.PathLog.Warnf("PathUtil Log level [%s] is invalid, set to [info] level",
					factory.AmfConfig.Logger.PathUtil.DebugLevel)
				pathUtilLogger.SetLogLevel(logrus.InfoLevel)
			} else {
				pathUtilLogger.SetLogLevel(level)
			}
		} else {
			pathUtilLogger.PathLog.Warnln("PathUtil Log level not set. Default set to [info] level")
			pathUtilLogger.SetLogLevel(logrus.InfoLevel)
		}
		pathUtilLogger.SetReportCaller(factory.AmfConfig.Logger.PathUtil.ReportCaller)
	}

	if factory.AmfConfig.Logger.OpenApi != nil {
		if factory.AmfConfig.Logger.OpenApi.DebugLevel != "" {
			if level, err := logrus.ParseLevel(factory.AmfConfig.Logger.OpenApi.DebugLevel); err != nil {
				openApiLogger.OpenApiLog.Warnf("OpenAPI Log level [%s] is invalid, set to [info] level",
					factory.AmfConfig.Logger.OpenApi.DebugLevel)
				openApiLogger.SetLogLevel(logrus.InfoLevel)
			} else {
				openApiLogger.SetLogLevel(level)
			}
		} else {
			openApiLogger.OpenApiLog.Warnln("OpenAPI Log level not set. Default set to [info] level")
			openApiLogger.SetLogLevel(logrus.InfoLevel)
		}
		openApiLogger.SetReportCaller(factory.AmfConfig.Logger.OpenApi.ReportCaller)
	}
}

func (amf *AMF) FilterCli(c *cli.Context) (args []string) {
	for _, flag := range amf.GetCliCmd() {
		name := flag.GetName()
		value := fmt.Sprint(c.Generic(name))
		if value == "" {
			continue
		}

		args = append(args, "--"+name, value)
	}
	return args
}

func (amf *AMF) Start() {
	initLog.Infoln("Server started")

	router := logger_util.NewGinWithLogrus(logger.GinLog)
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

	self := context.AMF_Self()
	util.InitAmfContext(self)

	addr := fmt.Sprintf("%s:%d", self.BindingIPv4, self.SBIPort)

	ngapHandler := ngap_service.NGAPHandler{
		HandleMessage:      ngap.Dispatch,
		HandleNotification: ngap.HandleSCTPNotification,
	}
	ngap_service.Run(self.NgapIpList, 38412, ngapHandler)

	go amf.SendNFProfileUpdateToNrf()

	signalChannel := make(chan os.Signal, 1)
	signal.Notify(signalChannel, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-signalChannel
		amf.Terminate()
		os.Exit(0)
	}()

	server, err := http2_util.NewServer(addr, util.AmfLogPath, router)

	if server == nil {
		initLog.Errorf("Initialize HTTP server failed: %+v", err)
		return
	}

	if err != nil {
		initLog.Warnf("Initialize HTTP server: %+v", err)
	}

	serverScheme := factory.AmfConfig.Configuration.Sbi.Scheme
	if serverScheme == "http" {
		err = server.ListenAndServe()
	} else if serverScheme == "https" {
		err = server.ListenAndServeTLS(util.AmfPemPath, util.AmfKeyPath)
	}

	if err != nil {
		initLog.Fatalf("HTTP server setup failed: %+v", err)
	}
}

func (amf *AMF) Exec(c *cli.Context) error {
	// AMF.Initialize(cfgPath, c)

	initLog.Traceln("args:", c.String("amfcfg"))
	args := amf.FilterCli(c)
	initLog.Traceln("filter: ", args)
	command := exec.Command("./amf", args...)

	stdout, err := command.StdoutPipe()
	if err != nil {
		initLog.Fatalln(err)
	}
	wg := sync.WaitGroup{}
	wg.Add(3)
	go func() {
		in := bufio.NewScanner(stdout)
		for in.Scan() {
			fmt.Println(in.Text())
		}
		wg.Done()
	}()

	stderr, err := command.StderrPipe()
	if err != nil {
		initLog.Fatalln(err)
	}
	go func() {
		in := bufio.NewScanner(stderr)
		for in.Scan() {
			fmt.Println(in.Text())
		}
		wg.Done()
	}()

	go func() {
		if err = command.Start(); err != nil {
			initLog.Errorf("AMF Start error: %+v", err)
		}
		wg.Done()
	}()

	wg.Wait()

	return err
}

// Used in AMF planned removal procedure
func (amf *AMF) Terminate() {
	logger.InitLog.Infof("Terminating AMF...")
	amfSelf := context.AMF_Self()

	// TODO: forward registered UE contexts to target AMF in the same AMF set if there is one

	// deregister with NRF
	problemDetails, err := consumer.SendDeregisterNFInstance()
	if problemDetails != nil {
		logger.InitLog.Errorf("Deregister NF instance Failed Problem[%+v]", problemDetails)
	} else if err != nil {
		logger.InitLog.Errorf("Deregister NF instance Error[%+v]", err)
	} else {
		logger.InitLog.Infof("[AMF] Deregister from NRF successfully")
	}

	// send AMF status indication to ran to notify ran that this AMF will be unavailable
	logger.InitLog.Infof("Send AMF Status Indication to Notify RANs due to AMF terminating")
	unavailableGuamiList := ngap_message.BuildUnavailableGUAMIList(amfSelf.ServedGuamiList)
	amfSelf.AmfRanPool.Range(func(key, value interface{}) bool {
		ran := value.(*context.AmfRan)
		ngap_message.SendAMFStatusIndication(ran, unavailableGuamiList)
		return true
	})

	ngap_service.Stop()

	callback.SendAmfStatusChangeNotify((string)(models.StatusChange_UNAVAILABLE), amfSelf.ServedGuamiList)
	logger.InitLog.Infof("AMF terminated")
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
						factory.AmfConfig.Configuration.PlmnSupportList[plmnindex].SNssaiList =
							append(factory.AmfConfig.Configuration.PlmnSupportList[plmnindex].SNssaiList[:i], p.SNssaiList[i+1:]...)
						if len(factory.AmfConfig.Configuration.PlmnSupportList[plmnindex].SNssaiList) == 0 {
							factory.AmfConfig.Configuration.PlmnSupportList =
								append(factory.AmfConfig.Configuration.PlmnSupportList[:plmnindex],
									factory.AmfConfig.Configuration.PlmnSupportList[plmnindex+1:]...)

							factory.AmfConfig.Configuration.ServedGumaiList =
								append(factory.AmfConfig.Configuration.ServedGumaiList[:plmnindex],
									factory.AmfConfig.Configuration.ServedGumaiList[plmnindex+1:]...)
						}
					}
					break
				}
			}

			if !found && opType != protos.OpType_SLICE_DELETE {
				logger.GrpcLog.Infof("plmn found but slice not found in AMF Configuration")
				factory.AmfConfig.Configuration.PlmnSupportList[plmnindex].SNssaiList =
					append(factory.AmfConfig.Configuration.PlmnSupportList[plmnindex].SNssaiList, nssai_r)
			}
			break
		}
	}

	var guami = models.Guami{PlmnId: &plmn.PlmnId, AmfId: "cafe00"}
	if !plmnFound && opType != protos.OpType_SLICE_DELETE {
		factory.AmfConfig.Configuration.PlmnSupportList =
			append(factory.AmfConfig.Configuration.PlmnSupportList, plmn)
		factory.AmfConfig.Configuration.ServedGumaiList =
			append(factory.AmfConfig.Configuration.ServedGumaiList, guami)
	}
	logger.GrpcLog.Infof("SupportedPlmnLIst: %v, SupportGuamiLIst: %v received fromRoc\n", plmn, guami)
	logger.GrpcLog.Infof("SupportedPlmnLIst: %v, SupportGuamiLIst: %v in AMF\n", factory.AmfConfig.Configuration.PlmnSupportList,
		factory.AmfConfig.Configuration.ServedGumaiList)
	//same plmn received but Tacs in gnb updated
	nssai_r := plmn.SNssaiList[0]
	slice := strconv.FormatInt(int64(nssai_r.Sst), 10) + nssai_r.Sd
	delete(factory.AmfConfig.Configuration.SliceTaiList, slice)
	if opType != protos.OpType_SLICE_DELETE {
		//maintaining slice level tai List
		if factory.AmfConfig.Configuration.SliceTaiList == nil {
			factory.AmfConfig.Configuration.SliceTaiList = make(map[string][]models.Tai)
		}
		factory.AmfConfig.Configuration.SliceTaiList[slice] = taiList
	}

	amf.UpdateSupportedTaiList()
	logger.GrpcLog.Infoln("Gnb Updated in existing Plmn, SupportTAILIst received from Roc: ", taiList)
	logger.GrpcLog.Infoln("SupportTAILIst in AMF", factory.AmfConfig.Configuration.SupportTAIList)
}

func (amf *AMF) UpdateSupportedTaiList() {
	factory.AmfConfig.Configuration.SupportTAIList = nil
	for _, slice := range factory.AmfConfig.Configuration.SliceTaiList {
		for _, tai := range slice {
			logger.GrpcLog.Infoln("Tai list present in Slice", tai, factory.AmfConfig.Configuration.SupportTAIList)
			factory.AmfConfig.Configuration.SupportTAIList =
				append(factory.AmfConfig.Configuration.SupportTAIList, tai)
		}
	}
}
func (amf *AMF) UpdateConfig(commChannel chan *protos.NetworkSliceResponse) bool {
	for rsp := range commChannel {
		logger.GrpcLog.Infof("Received updateConfig in the amf app : %v", rsp)
		var tai []models.Tai
		var plmnList []*factory.PlmnSupportItem
		for _, ns := range rsp.NetworkSlice {
			var snssai *models.Snssai
			logger.GrpcLog.Infoln("Network Slice Name ", ns.Name)
			if ns.Nssai != nil {
				snssai = new(models.Snssai)
				val, _ := strconv.ParseInt(ns.Nssai.Sst, 10, 64)
				snssai.Sst = int32(val)
				snssai.Sd = ns.Nssai.Sd
			}
			//inform connected UEs with update slices
			if len(ns.DeletedImsis) > 0 {
				HandleImsiDeleteFromNetworkSlice(ns)
			}
			//TODO Inform connected UEs with update Slice
			/*if len(ns.AddUpdatedImsis) > 0 {
				HandleImsiAddInNetworkSlice(ns)
			}*/

			if ns.Site != nil {
				site := ns.Site
				logger.GrpcLog.Infoln("Network Slice has site name: ", site.SiteName)
				if site.Plmn != nil {
					plmn := new(factory.PlmnSupportItem)

					logger.GrpcLog.Infoln("Plmn mcc ", site.Plmn.Mcc)
					plmn.PlmnId.Mnc = site.Plmn.Mnc
					plmn.PlmnId.Mcc = site.Plmn.Mcc
					plmnList = append(plmnList, plmn)

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
					logger.GrpcLog.Infoln("Plmn not present in the message ")
				}

			}
		} // end of network slice for loop

		//Update PlmnSupportList/ServedGuamiList/ServedTAIList in Amf Config
		//factory.AmfConfig.Configuration.ServedGumaiList = nil
		//factory.AmfConfig.Configuration.PlmnSupportList = nil
		self := context.AMF_Self()
		util.InitAmfContext(self)
		if len(factory.AmfConfig.Configuration.ServedGumaiList) > 0 {
			RocUpdateConfigChannel <- true
		}
	}
	return true
}

func (amf *AMF) SendNFProfileUpdateToNrf() {
	for rocUpdateConfig := range RocUpdateConfigChannel {
		if rocUpdateConfig {
			self := context.AMF_Self()
			util.InitAmfContext(self)

			// Register to NRF with Updated Profile
			var profile models.NfProfile
			if profileTmp, err := consumer.BuildNFInstance(self); err != nil {
				logger.CfgLog.Error("Build AMF Profile Error")
			} else {
				profile = profileTmp
			}

			if _, nfId, err := consumer.SendRegisterNFInstance(self.NrfUri, self.NfId, profile); err != nil {
				logger.CfgLog.Warnf("Send Register NF Instance with updated profile failed: %+v", err)
			} else {
				self.NfId = nfId
				logger.CfgLog.Infof("Sent Register NF Instance with updated profile")
			}
		}
	}
}

func UeConfigSliceDeleteHandler(supi, sst, sd string, msg interface{}) {
	amfSelf := context.AMF_Self()
	ue, _ := amfSelf.AmfUeFindBySupi("imsi-" + supi)

	// Triggers for NwInitiatedDeRegistration
	// - Only 1 Allowed Nssai is exist and its slice information matched
	ns := msg.(*protos.NetworkSlice)
	if len(ue.AllowedNssai[models.AccessType__3_GPP_ACCESS]) == 1 {
		st, _ := strconv.Atoi(ns.Nssai.Sst)
		if ue.AllowedNssai[models.AccessType__3_GPP_ACCESS][0].AllowedSnssai.Sst == int32(st) &&
			ue.AllowedNssai[models.AccessType__3_GPP_ACCESS][0].AllowedSnssai.Sd == ns.Nssai.Sd {
			gmm.GmmFSM.SendEvent(ue.State[models.AccessType__3_GPP_ACCESS], gmm.NwInitiatedDeregistrationEvent, fsm.ArgsType{
				gmm.ArgAmfUe:      ue,
				gmm.ArgAccessType: models.AccessType__3_GPP_ACCESS,
			})
		} else {
			logger.CfgLog.Infof("Deleted slice not matched with slice info in UEContext")
		}
	} else {
		var Nssai models.Snssai
		st, _ := strconv.Atoi(ns.Nssai.Sst)
		Nssai.Sst = int32(st)
		Nssai.Sd = ns.Nssai.Sd
		gmm.GmmFSM.SendEvent(ue.State[models.AccessType__3_GPP_ACCESS], gmm.SliceInfoDeleteEvent, fsm.ArgsType{
			gmm.ArgAmfUe:      ue,
			gmm.ArgAccessType: models.AccessType__3_GPP_ACCESS,
			gmm.ArgNssai:      Nssai,
		})
	}
}

func UeConfigSliceAddHandler(supi, sst, sd string, msg interface{}) {
	amfSelf := context.AMF_Self()
	ue, _ := amfSelf.AmfUeFindBySupi("imsi-" + supi)

	ns := msg.(*protos.NetworkSlice)
	var Nssai models.Snssai
	st, _ := strconv.Atoi(ns.Nssai.Sst)
	Nssai.Sst = int32(st)
	Nssai.Sd = ns.Nssai.Sd
	gmm.GmmFSM.SendEvent(ue.State[models.AccessType__3_GPP_ACCESS], gmm.SliceInfoAddEvent, fsm.ArgsType{
		gmm.ArgAmfUe:      ue,
		gmm.ArgAccessType: models.AccessType__3_GPP_ACCESS,
		gmm.ArgNssai:      Nssai,
	})
}

func HandleImsiDeleteFromNetworkSlice(slice *protos.NetworkSlice) {
	var ue *context.AmfUe
	var ok bool
	logger.CfgLog.Infof("[AMF] Handle Subscribers Delete From Network Slice [sst:%v sd:%v]", slice.Nssai.Sst, slice.Nssai.Sd)

	for _, supi := range slice.DeletedImsis {
		amfSelf := context.AMF_Self()
		ue, ok = amfSelf.AmfUeFindBySupi("imsi-" + supi)
		if !ok {
			logger.CfgLog.Infof("the UE [%v] is not Registered with the 5G-Core", supi)
			continue
		}
		//publish the event to ue channel
		configMsg := context.ConfigMsg{
			Supi: supi,
			Msg:  slice,
			Sst:  slice.Nssai.Sst,
			Sd:   slice.Nssai.Sd,
		}

		ue.EventChannel.UpdateConfigHandler(UeConfigSliceDeleteHandler)
		ue.EventChannel.SubmitMessage(configMsg)
	}
}

func HandleImsiAddInNetworkSlice(slice *protos.NetworkSlice) {
	var ue *context.AmfUe
	var ok bool
	logger.CfgLog.Infof("[AMF] Handle Subscribers Added in Network Slice [sst:%v sd:%v]", slice.Nssai.Sst, slice.Nssai.Sd)

	for _, supi := range slice.AddUpdatedImsis {
		amfSelf := context.AMF_Self()
		ue, ok = amfSelf.AmfUeFindBySupi("imsi-" + supi)
		if !ok {
			logger.CfgLog.Infof("the UE [%v] is not Registered with the 5G-Core", supi)
			continue
		}
		//publish the event to ue channel
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
