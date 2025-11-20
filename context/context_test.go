// SPDX-FileCopyrightText: 2025 Intel Corporation
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

package context

import (
	"reflect"
	"sync"
	"testing"

	"github.com/omec-project/amf/factory"
	"github.com/omec-project/openapi/models"
	"github.com/omec-project/openapi/nfConfigApi"
)

var (
	testConfig     factory.Config
	configLoadOnce sync.Once
	configLoadErr  error
)

func makeSnssaiWithSd(sst int32, sd string) nfConfigApi.Snssai {
	s := nfConfigApi.NewSnssai(sst)
	s.SetSd(sd)
	return *s
}

func loadTestConfig() error {
	configLoadOnce.Do(func() {
		origConfig := factory.AmfConfig // Save original

		configLoadErr = factory.InitConfigFactory("../util/testdata/amfcfg.yaml")
		if configLoadErr == nil {
			testConfig = factory.AmfConfig
		}

		factory.AmfConfig = origConfig // Restore original
	})
	return configLoadErr
}

// setupTestFactory sets up the factory config for testing and returns a cleanup function
func setupTestFactory(t *testing.T) func() {
	t.Helper()

	if err := loadTestConfig(); err != nil {
		t.Fatalf("failed to load test config: %v", err)
	}

	origFactory := factory.AmfConfig
	factory.AmfConfig = testConfig

	return func() {
		factory.AmfConfig = origFactory
	}
}

func TestUpdateAMFContext(t *testing.T) {
	testCases := []struct {
		name                    string
		accessAndMobilityConfig []nfConfigApi.AccessAndMobility
		expectedSupportTaiLists []models.Tai
		expectedServedGuamiList []models.Guami
		expectedPlmnSupportList []models.PlmnSnssai
	}{
		{
			name: "One Access and Mobility config",
			accessAndMobilityConfig: []nfConfigApi.AccessAndMobility{
				{
					PlmnId: nfConfigApi.PlmnId{Mcc: "001", Mnc: "01"},
					Snssai: makeSnssaiWithSd(1, "01"),
					Tacs:   []string{"1"},
				},
			},
			expectedSupportTaiLists: []models.Tai{
				{
					PlmnId: &models.PlmnId{Mcc: "001", Mnc: "01"},
					Tac:    "1",
				},
			},
			expectedServedGuamiList: []models.Guami{
				{
					PlmnId: &models.PlmnId{Mcc: "001", Mnc: "01"},
					AmfId:  "cafe00",
				},
			},
			expectedPlmnSupportList: []models.PlmnSnssai{
				{
					PlmnId: &models.PlmnId{Mcc: "001", Mnc: "01"},
					SNssaiList: []models.Snssai{
						{Sst: 1, Sd: "01"},
					},
				},
			},
		},
		{
			name: "One Access and Mobility config (no TACs)",
			accessAndMobilityConfig: []nfConfigApi.AccessAndMobility{
				{
					PlmnId: nfConfigApi.PlmnId{Mcc: "001", Mnc: "01"},
					Snssai: makeSnssaiWithSd(1, "01"),
					Tacs:   []string{},
				},
			},
			expectedSupportTaiLists: []models.Tai{},
			expectedServedGuamiList: []models.Guami{
				{
					PlmnId: &models.PlmnId{Mcc: "001", Mnc: "01"},
					AmfId:  "cafe00",
				},
			},
			expectedPlmnSupportList: []models.PlmnSnssai{
				{
					PlmnId: &models.PlmnId{Mcc: "001", Mnc: "01"},
					SNssaiList: []models.Snssai{
						{Sst: 1, Sd: "01"},
					},
				},
			},
		},
		{
			name: "Two Access and Mobility config (different PLMN)",
			accessAndMobilityConfig: []nfConfigApi.AccessAndMobility{
				{
					PlmnId: nfConfigApi.PlmnId{Mcc: "001", Mnc: "01"},
					Snssai: makeSnssaiWithSd(1, "01"),
					Tacs:   []string{"1"},
				},
				{
					PlmnId: nfConfigApi.PlmnId{Mcc: "001", Mnc: "02"},
					Snssai: makeSnssaiWithSd(2, "01"),
					Tacs:   []string{"2"},
				},
			},
			expectedServedGuamiList: []models.Guami{
				{
					PlmnId: &models.PlmnId{Mcc: "001", Mnc: "01"},
					AmfId:  "cafe00",
				},
				{
					PlmnId: &models.PlmnId{Mcc: "001", Mnc: "02"},
					AmfId:  "cafe00",
				},
			},
			expectedSupportTaiLists: []models.Tai{
				{
					PlmnId: &models.PlmnId{Mcc: "001", Mnc: "01"},
					Tac:    "1",
				},
				{
					PlmnId: &models.PlmnId{Mcc: "001", Mnc: "02"},
					Tac:    "2",
				},
			},
			expectedPlmnSupportList: []models.PlmnSnssai{
				{
					PlmnId: &models.PlmnId{Mcc: "001", Mnc: "01"},
					SNssaiList: []models.Snssai{
						{Sst: 1, Sd: "01"},
					},
				},
				{
					PlmnId: &models.PlmnId{Mcc: "001", Mnc: "02"},
					SNssaiList: []models.Snssai{
						{Sst: 2, Sd: "01"},
					},
				},
			},
		},
		{
			name: "Two Access and Mobility configs (same PLMN, different SNssai)",
			accessAndMobilityConfig: []nfConfigApi.AccessAndMobility{
				{
					PlmnId: nfConfigApi.PlmnId{Mcc: "001", Mnc: "01"},
					Snssai: makeSnssaiWithSd(1, "01"),
					Tacs:   []string{"1"},
				},
				{
					PlmnId: nfConfigApi.PlmnId{Mcc: "001", Mnc: "01"},
					Snssai: makeSnssaiWithSd(2, "01"),
					Tacs:   []string{"2"},
				},
			},
			expectedSupportTaiLists: []models.Tai{
				{
					PlmnId: &models.PlmnId{Mcc: "001", Mnc: "01"},
					Tac:    "1",
				},
				{
					PlmnId: &models.PlmnId{Mcc: "001", Mnc: "01"},
					Tac:    "2",
				},
			},
			expectedServedGuamiList: []models.Guami{
				{
					PlmnId: &models.PlmnId{Mcc: "001", Mnc: "01"},
					AmfId:  "cafe00",
				},
			},
			expectedPlmnSupportList: []models.PlmnSnssai{
				{
					PlmnId: &models.PlmnId{Mcc: "001", Mnc: "01"},
					SNssaiList: []models.Snssai{
						{Sst: 1, Sd: "01"},
						{Sst: 2, Sd: "01"},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cleanup := setupTestFactory(t)
			defer cleanup()

			amfContext := AMFContext{}
			err := UpdateAmfContext(&amfContext, tc.accessAndMobilityConfig)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(tc.expectedSupportTaiLists, amfContext.SupportTaiLists) {
				t.Errorf("expected SupportTaiLists: %#v, got: %#v", tc.expectedSupportTaiLists, amfContext.SupportTaiLists)
			}
			if !reflect.DeepEqual(tc.expectedServedGuamiList, amfContext.ServedGuamiList) {
				t.Errorf("expected ServedGuamiList: %#v, got: %#v", tc.expectedServedGuamiList, amfContext.ServedGuamiList)
			}
			if !reflect.DeepEqual(tc.expectedPlmnSupportList, amfContext.PlmnSupportList) {
				t.Errorf("expected PlmnSupportList: %#v, got: %#v", tc.expectedPlmnSupportList, amfContext.PlmnSupportList)
			}
		})
	}
}
