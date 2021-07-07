/*
 * AMF Configuration Factory
 */

package factory

import (
<<<<<<< HEAD
	"fmt"
	"strconv"
=======
>>>>>>> parent of bb87454... SDCORE-187: AMF config update through RoC interface
	"time"

	"github.com/free5gc/logger_util"
	"github.com/free5gc/openapi/models"
)

<<<<<<< HEAD
var MinConfigAvailable bool = false

=======
>>>>>>> parent of bb87454... SDCORE-187: AMF config update through RoC interface
const (
	AMF_EXPECTED_CONFIG_VERSION = "1.0.0"
)

type Config struct {
	Info          *Info               `yaml:"info"`
	Configuration *Configuration      `yaml:"configuration"`
	Logger        *logger_util.Logger `yaml:"logger"`
}

type Info struct {
	Version     string `yaml:"version,omitempty"`
	Description string `yaml:"description,omitempty"`
}

const (
	AMF_DEFAULT_IPV4     = "127.0.0.18"
	AMF_DEFAULT_PORT     = "8000"
	AMF_DEFAULT_PORT_INT = 8000
	AMF_DEFAULT_NRFURI   = "https://127.0.0.10:8000"
)

type Configuration struct {
	AmfName                         string                    `yaml:"amfName,omitempty"`
	NgapIpList                      []string                  `yaml:"ngapIpList,omitempty"`
	Sbi                             *Sbi                      `yaml:"sbi,omitempty"`
	NetworkFeatureSupport5GS        *NetworkFeatureSupport5GS `yaml:"networkFeatureSupport5GS,omitempty"`
	ServiceNameList                 []string                  `yaml:"serviceNameList,omitempty"`
	ServedGumaiList                 []models.Guami            `yaml:"servedGuamiList,omitempty"`
	SupportTAIList                  []models.Tai              `yaml:"supportTaiList,omitempty"`
	PlmnSupportList                 []PlmnSupportItem         `yaml:"plmnSupportList,omitempty"`
	SupportDnnList                  []string                  `yaml:"supportDnnList,omitempty"`
	NrfUri                          string                    `yaml:"nrfUri,omitempty"`
	Security                        *Security                 `yaml:"security,omitempty"`
	NetworkName                     NetworkName               `yaml:"networkName,omitempty"`
	T3502Value                      int                       `yaml:"t3502Value,omitempty"`
	T3512Value                      int                       `yaml:"t3512Value,omitempty"`
	Non3gppDeregistrationTimerValue int                       `yaml:"non3gppDeregistrationTimerValue,omitempty"`
	T3513                           TimerValue                `yaml:"t3513"`
	T3522                           TimerValue                `yaml:"t3522"`
	T3550                           TimerValue                `yaml:"t3550"`
	T3560                           TimerValue                `yaml:"t3560"`
	T3565                           TimerValue                `yaml:"t3565"`
}

func (c *Configuration) Get5gsNwFeatSuppEnable() bool {
	if c.NetworkFeatureSupport5GS != nil {
		return c.NetworkFeatureSupport5GS.Enable
	}
	return true
}

func (c *Configuration) Get5gsNwFeatSuppImsVoPS() uint8 {
	if c.NetworkFeatureSupport5GS != nil {
		return c.NetworkFeatureSupport5GS.ImsVoPS
	}
	return 0
}

func (c *Configuration) Get5gsNwFeatSuppEmc() uint8 {
	if c.NetworkFeatureSupport5GS != nil {
		return c.NetworkFeatureSupport5GS.Emc
	}
	return 0
}

func (c *Configuration) Get5gsNwFeatSuppEmf() uint8 {
	if c.NetworkFeatureSupport5GS != nil {
		return c.NetworkFeatureSupport5GS.Emf
	}
	return 0
}

func (c *Configuration) Get5gsNwFeatSuppIwkN26() uint8 {
	if c.NetworkFeatureSupport5GS != nil {
		return c.NetworkFeatureSupport5GS.IwkN26
	}
	return 0
}

func (c *Configuration) Get5gsNwFeatSuppMpsi() uint8 {
	if c.NetworkFeatureSupport5GS != nil {
		return c.NetworkFeatureSupport5GS.Mpsi
	}
	return 0
}

func (c *Configuration) Get5gsNwFeatSuppEmcN3() uint8 {
	if c.NetworkFeatureSupport5GS != nil {
		return c.NetworkFeatureSupport5GS.EmcN3
	}
	return 0
}

func (c *Configuration) Get5gsNwFeatSuppMcsi() uint8 {
	if c.NetworkFeatureSupport5GS != nil {
		return c.NetworkFeatureSupport5GS.Mcsi
	}
	return 0
}

type NetworkFeatureSupport5GS struct {
	Enable  bool  `yaml:"enable"`
	ImsVoPS uint8 `yaml:"imsVoPS"`
	Emc     uint8 `yaml:"emc"`
	Emf     uint8 `yaml:"emf"`
	IwkN26  uint8 `yaml:"iwkN26"`
	Mpsi    uint8 `yaml:"mpsi"`
	EmcN3   uint8 `yaml:"emcN3"`
	Mcsi    uint8 `yaml:"mcsi"`
}

type Sbi struct {
	Scheme       string `yaml:"scheme"`
	RegisterIPv4 string `yaml:"registerIPv4,omitempty"` // IP that is registered at NRF.
	BindingIPv4  string `yaml:"bindingIPv4,omitempty"`  // IP used to run the server in the node.
	Port         int    `yaml:"port,omitempty"`
}

type Security struct {
	IntegrityOrder []string `yaml:"integrityOrder,omitempty"`
	CipheringOrder []string `yaml:"cipheringOrder,omitempty"`
}

type PlmnSupportItem struct {
	PlmnId     models.PlmnId   `yaml:"plmnId"`
	SNssaiList []models.Snssai `yaml:"snssaiList,omitempty"`
}

type NetworkName struct {
	Full  string `yaml:"full"`
	Short string `yaml:"short,omitempty"`
}

type TimerValue struct {
	Enable        bool          `yaml:"enable"`
	ExpireTime    time.Duration `yaml:"expireTime"`
	MaxRetryTimes int           `yaml:"maxRetryTimes,omitempty"`
}

func (c *Config) GetVersion() string {
	if c.Info != nil && c.Info.Version != "" {
		return c.Info.Version
	}
	return ""
}
<<<<<<< HEAD

func (c *Config) updateConfig(commChannel chan *protos.NetworkSliceResponse) bool {
	for {
		select {
		case rsp := <-commChannel:
			fmt.Println("Received updateConfig in the amf app : ", rsp)
			MinConfigAvailable = true
			for i := 0; i < len(rsp.NetworkSlice); i++ {
				var plmn *PlmnSupportItem
				var tai *models.Tai
				var guami *models.Guami
				var snssai *models.Snssai
				ns := rsp.NetworkSlice[i]
				logger.GrpcLog.Infoln("Network Slice Name ", ns.Name)
				if ns.Nssai != nil {
					snssai = new(models.Snssai)
					val, _ := strconv.ParseInt(ns.Nssai.Sst, 10, 64)
					snssai.Sst = int32(val)
					snssai.Sd = ns.Nssai.Sd
				}
				if ns.Site != nil {
					logger.GrpcLog.Infoln("Network Slice has site name present ")
					site := ns.Site
					logger.GrpcLog.Infoln("Site name ", site.SiteName)
					if site.Plmn != nil {
						plmn = new(PlmnSupportItem)
						guami = new(models.Guami)
						tai = new(models.Tai)
						tai.PlmnId = new(models.PlmnId)
						guami.PlmnId = new(models.PlmnId)
						guami.PlmnId.Mnc = site.Plmn.Mnc
						guami.PlmnId.Mcc = site.Plmn.Mcc

						logger.GrpcLog.Infoln("Plmn mcc ", site.Plmn.Mcc)
						plmn.PlmnId.Mnc = site.Plmn.Mnc
						plmn.PlmnId.Mcc = site.Plmn.Mcc
						//AmfConfig.Configuration.PlmnSupportList =
						//		append(AmfConfig.Configuration.PlmnSupportList, plmn)
						tai.PlmnId.Mnc = site.Plmn.Mnc
						tai.PlmnId.Mcc = site.Plmn.Mcc
						if ns.Nssai != nil {
							plmn.SNssaiList = append(plmn.SNssaiList, *snssai)
						}
						if site.Gnb != nil {
							for _, gnb := range site.Gnb {
								tai.Tac = strconv.Itoa(int(gnb.Tac))
							}
						}

					} else {
						logger.GrpcLog.Infoln("Plmn not present in the message ")
					}

				}

				//Update PlmnSupportList/ServedGuamiList/ServedTAIList in Amf Config
				if ns.Site != nil && ns.Site.Plmn != nil {
					site := ns.Site
					var plmnfound bool
					for _, plmnExist := range AmfConfig.Configuration.PlmnSupportList {
						if plmnExist.PlmnId.Mnc == site.Plmn.Mnc &&
							plmnExist.PlmnId.Mcc == site.Plmn.Mcc {
							plmnfound = true
							break
						}
					}
					if !plmnfound {
						AmfConfig.Configuration.PlmnSupportList =
							append(AmfConfig.Configuration.PlmnSupportList, *plmn)
						logger.GrpcLog.Infoln("SupportedPlmnLIst received from Roc: ", *plmn)
						AmfConfig.Configuration.SupportTAIList =
							append(AmfConfig.Configuration.SupportTAIList, *tai)
						logger.GrpcLog.Infoln("SupportTAILIst received from Roc: ", *tai)
						guami.AmfId = "cafe00"
						AmfConfig.Configuration.ServedGumaiList =
							append(AmfConfig.Configuration.ServedGumaiList, *guami)
						logger.GrpcLog.Infoln("SupportGuamiLIst received from Roc: ", *guami)
					}

				}
			}
		}
	}
	return true
}
=======
>>>>>>> parent of bb87454... SDCORE-187: AMF config update through RoC interface
