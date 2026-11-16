// Copyright (C) 2026 Intel Corporation
// SPDX-FileCopyrightText: 2024 Canonical Ltd.
// SPDX-License-Identifier: Apache-2.0

package factory

import (
	"testing"

	"github.com/omec-project/util/nfconfig"
)

func TestWebuiUrl(t *testing.T) {
	tests := []struct {
		name       string
		configFile string
		want       string
	}{
		{
			name:       "default webui URL",
			configFile: "../util/testdata/amfcfg.yaml",
			want:       nfconfig.DefaultWebuiUri,
		},
		{
			name:       "custom webui URL",
			configFile: "../util/testdata/amfcfg_with_custom_webui_url_and_amfid.yaml",
			want:       "https://myspecialwebui:5002",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origAmfConfig := AmfConfig
			t.Cleanup(func() { AmfConfig = origAmfConfig })

			if err := InitConfigFactory(tt.configFile); err != nil {
				t.Fatalf("Error in InitConfigFactory: %v", err)
			}

			got := AmfConfig.Configuration.WebuiUri
			if got != tt.want {
				t.Errorf("WebuiUri is not correct. got = %q, want = %q", got, tt.want)
			}
		})
	}
}

func TestAmfId(t *testing.T) {
	tests := []struct {
		name       string
		configFile string
		want       string
	}{
		{
			name:       "default AMF ID",
			configFile: "../util/testdata/amfcfg.yaml",
			want:       defaultAmfID,
		},
		{
			name:       "custom AMF ID",
			configFile: "../util/testdata/amfcfg_with_custom_webui_url_and_amfid.yaml",
			want:       "cafe01",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origAmfConfig := AmfConfig
			t.Cleanup(func() { AmfConfig = origAmfConfig })

			if err := InitConfigFactory(tt.configFile); err != nil {
				t.Fatalf("Error in InitConfigFactory: %v", err)
			}

			got := AmfConfig.Configuration.AmfId
			if got != tt.want {
				t.Errorf("AmfId is not correct. got = %q, want = %q", got, tt.want)
			}
		})
	}
}

func TestNoTelemetryConfig(t *testing.T) {
	origAmfConfig := AmfConfig
	t.Cleanup(func() { AmfConfig = origAmfConfig })
	if err := InitConfigFactory("../util/testdata/no_telemetry.yaml"); err != nil {
		t.Logf("Error in InitConfigFactory: %v", err)
	}

	if AmfConfig.Configuration.Telemetry != nil {
		t.Errorf("expected no telemetry configuration, but got: %v", AmfConfig.Configuration.Telemetry)
	}
}

func TestTelemetryConfigEnabled(t *testing.T) {
	origAmfConfig := AmfConfig
	t.Cleanup(func() { AmfConfig = origAmfConfig })
	if err := InitConfigFactory("../util/testdata/telemetry.yaml"); err != nil {
		t.Logf("Error in InitConfigFactory: %v", err)
	}

	if AmfConfig.Configuration.Telemetry == nil {
		t.Fatalf("expected telemetry configuration to be present, but it is nil")
	}

	if !AmfConfig.Configuration.Telemetry.Enabled {
		t.Errorf("expected telemetry to be enabled, but it is not")
	}

	if AmfConfig.Configuration.Telemetry.OtlpEndpoint == "" {
		t.Errorf("expected OTLP endpoint to be set, but it is empty")
	}

	if AmfConfig.Configuration.Telemetry.Ratio == nil || *AmfConfig.Configuration.Telemetry.Ratio != 0.4 {
		t.Errorf("expected telemetry ratio to be 0.4, but got: %v", AmfConfig.Configuration.Telemetry.Ratio)
	}
}

func TestTelemetryConfigEnabledNoRatioDefaultsTo1(t *testing.T) {
	origAmfConfig := AmfConfig
	t.Cleanup(func() { AmfConfig = origAmfConfig })
	if err := InitConfigFactory("../util/testdata/telemetry_no_ratio.yaml"); err != nil {
		t.Logf("Error in InitConfigFactory: %v", err)
	}

	if AmfConfig.Configuration.Telemetry == nil {
		t.Fatalf("expected telemetry configuration to be present, but it is nil")
	}

	if !AmfConfig.Configuration.Telemetry.Enabled {
		t.Errorf("expected telemetry to be enabled, but it is not")
	}

	if AmfConfig.Configuration.Telemetry.OtlpEndpoint == "" {
		t.Errorf("expected OTLP endpoint to be set, but it is empty")
	}

	if AmfConfig.Configuration.Telemetry.Ratio == nil || *AmfConfig.Configuration.Telemetry.Ratio != 1.0 {
		t.Errorf("expected telemetry ratio to be 1.0, but got: %v", AmfConfig.Configuration.Telemetry.Ratio)
	}
}

func TestTelemetryConfigEnabledRatio0Stays0(t *testing.T) {
	origAmfConfig := AmfConfig
	t.Cleanup(func() { AmfConfig = origAmfConfig })
	if err := InitConfigFactory("../util/testdata/telemetry_zero_ratio.yaml"); err != nil {
		t.Logf("Error in InitConfigFactory: %v", err)
	}

	if AmfConfig.Configuration.Telemetry == nil {
		t.Fatalf("expected telemetry configuration to be present, but it is nil")
	}

	if !AmfConfig.Configuration.Telemetry.Enabled {
		t.Errorf("expected telemetry to be enabled, but it is not")
	}

	if AmfConfig.Configuration.Telemetry.OtlpEndpoint == "" {
		t.Errorf("expected OTLP endpoint to be set, but it is empty")
	}

	if AmfConfig.Configuration.Telemetry.Ratio == nil || *AmfConfig.Configuration.Telemetry.Ratio != 0.0 {
		t.Errorf("expected telemetry ratio to be 0.0, but got: %v", AmfConfig.Configuration.Telemetry.Ratio)
	}
}

func TestTelemetryConfigEnabledNoEndpointReturnsError(t *testing.T) {
	origAmfConfig := AmfConfig
	t.Cleanup(func() { AmfConfig = origAmfConfig })
	if err := InitConfigFactory("../util/testdata/telemetry_no_endpoint.yaml"); err == nil {
		t.Errorf("expected error when OTLP endpoint is not set, but got none")
	} else {
		t.Logf("Received expected error: %v", err)
	}
}

func TestValidateAmfId(t *testing.T) {
	tests := []struct {
		name    string
		amfId   string
		isValid bool
	}{
		{
			name:    "valid amfId",
			amfId:   defaultAmfID,
			isValid: true,
		},
		{
			name:    "invalid amfId (shorter than 6 chars)",
			amfId:   "cafe",
			isValid: false,
		},
		{
			name:    "invalid amfId (longer 6 chars)",
			amfId:   "cafe00cafe00",
			isValid: false,
		},
		{
			name:    "invalid amfId (invalid chars)",
			amfId:   "cafe!0",
			isValid: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateAmfId(tc.amfId)
			if err == nil && !tc.isValid {
				t.Errorf("expected amfId: %s to be invalid", tc.amfId)
			}
			if err != nil && tc.isValid {
				t.Errorf("expected amfId: %s to be valid", tc.amfId)
			}
		})
	}
}
