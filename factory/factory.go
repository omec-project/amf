/*
 * AMF Configuration Factory
 */

package factory

import (
	"fmt"
	"reflect"
	"io/ioutil"

	"gopkg.in/yaml.v2"

	"github.com/free5gc/amf/logger"
)

var AmfConfig Config

// TODO: Support configuration update from REST api
func InitConfigFactory(f string) error {
	if content, err := ioutil.ReadFile(f); err != nil {
		return err
	} else {
		AmfConfig = Config{}

		if yamlErr := yaml.Unmarshal(content, &AmfConfig); yamlErr != nil {
			return yamlErr
		}
	}

	return nil
}

func UpdateAmfConfig(f string) error {
	if content, err := ioutil.ReadFile(f); err != nil {
		return err
	} else {
		var amfConfig Config

		if yamlErr := yaml.Unmarshal(content, &amfConfig); yamlErr != nil {
			return yamlErr
		}
		//Checking which config has been changed
		if reflect.DeepEqual(AmfConfig.Configuration.AmfName, amfConfig.Configuration.AmfName) == false {
			fmt.Println("updated AMF Name is changed to ", amfConfig.Configuration.AmfName)
		} 
		if reflect.DeepEqual(AmfConfig.Configuration.NgapIpList, amfConfig.Configuration.NgapIpList) == false {
			fmt.Println("updated NgapList ", amfConfig.Configuration.NgapIpList)
		} 
		if reflect.DeepEqual(AmfConfig.Configuration.Sbi, amfConfig.Configuration.Sbi) == false {
			fmt.Println("updated Sbi ", amfConfig.Configuration.Sbi)
		} 
		if reflect.DeepEqual(AmfConfig.Configuration.NetworkFeatureSupport5GS, amfConfig.Configuration.NetworkFeatureSupport5GS) == false {
			fmt.Println("updated NetworkFeatureSupport5GS ", amfConfig.Configuration.NetworkFeatureSupport5GS)
		} 
		if reflect.DeepEqual(AmfConfig.Configuration.ServiceNameList, amfConfig.Configuration.ServiceNameList) == false {
			fmt.Println("updated ServiceNameList ", amfConfig.Configuration.ServiceNameList)
		} 
		if reflect.DeepEqual(AmfConfig.Configuration.ServedGumaiList, amfConfig.Configuration.ServedGumaiList) == false {
			fmt.Println("updated ServedGumaiList ", amfConfig.Configuration.ServedGumaiList)
		} 
		if reflect.DeepEqual(AmfConfig.Configuration.SupportTAIList, amfConfig.Configuration.SupportTAIList) == false {
			fmt.Println("updated SupportTAIList ", amfConfig.Configuration.SupportTAIList)
		} 
		if reflect.DeepEqual(AmfConfig.Configuration.PlmnSupportList, amfConfig.Configuration.PlmnSupportList) == false {
			fmt.Println("updated PlmnSupportList ", amfConfig.Configuration.PlmnSupportList)
		} 
		if reflect.DeepEqual(AmfConfig.Configuration.SupportDnnList, amfConfig.Configuration.SupportDnnList) == false {
			fmt.Println("updated SupportDnnList ", amfConfig.Configuration.SupportDnnList)
		} 
		if reflect.DeepEqual(AmfConfig.Configuration.NrfUri, amfConfig.Configuration.NrfUri) == false {
			fmt.Println("updated NrfUri ", amfConfig.Configuration.NrfUri)
		} 
		if reflect.DeepEqual(AmfConfig.Configuration.Security, amfConfig.Configuration.Security) == false {
			fmt.Println("updated Security ", amfConfig.Configuration.Security)
		} 
		if reflect.DeepEqual(AmfConfig.Configuration.NetworkName, amfConfig.Configuration.NetworkName) == false {
			fmt.Println("updated NetworkName ", amfConfig.Configuration.NetworkName)
		} 
		if reflect.DeepEqual(AmfConfig.Configuration.T3502Value, amfConfig.Configuration.T3502Value) == false {
			fmt.Println("updated T3502Value ", amfConfig.Configuration.T3502Value)
		} 
		if reflect.DeepEqual(AmfConfig.Configuration.T3512Value, amfConfig.Configuration.T3512Value) == false {
			fmt.Println("updated T3512Value ", amfConfig.Configuration.T3512Value)
		} 
		if reflect.DeepEqual(AmfConfig.Configuration.Non3gppDeregistrationTimerValue, amfConfig.Configuration.Non3gppDeregistrationTimerValue) == false {
			fmt.Println("updated Non3gppDeregistrationTimerValue ", amfConfig.Configuration.Non3gppDeregistrationTimerValue)
		} 
		if reflect.DeepEqual(AmfConfig.Configuration.T3513, amfConfig.Configuration.T3513) == false {
			fmt.Println("updated T3513 ", amfConfig.Configuration.T3513)
		} 
		if reflect.DeepEqual(AmfConfig.Configuration.T3522, amfConfig.Configuration.T3522) == false {
			fmt.Println("updated T3522 ", amfConfig.Configuration.T3522)
		} 
		if reflect.DeepEqual(AmfConfig.Configuration.T3550, amfConfig.Configuration.T3550) == false {
			fmt.Println("updated T3550 ", amfConfig.Configuration.T3550)
		} 
		if reflect.DeepEqual(AmfConfig.Configuration.T3560, amfConfig.Configuration.T3560) == false {
			fmt.Println("updated T3560 ", amfConfig.Configuration.T3560)
		} 
		if reflect.DeepEqual(AmfConfig.Configuration.T3565, amfConfig.Configuration.T3565) == false {
			fmt.Println("updated T3565 ", amfConfig.Configuration.T3565)
		}
		
		AmfConfig = amfConfig
	}
	return nil
}

func CheckConfigVersion() error {
	currentVersion := AmfConfig.GetVersion()

	if currentVersion != AMF_EXPECTED_CONFIG_VERSION {
		return fmt.Errorf("config version is [%s], but expected is [%s].",
			currentVersion, AMF_EXPECTED_CONFIG_VERSION)
	}

	logger.CfgLog.Infof("config version [%s]", currentVersion)

	return nil
}
