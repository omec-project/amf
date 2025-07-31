// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2024 Canonical Ltd.
/*
 *  Tests for AMF Configuration Factory
 */

package factory

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Webui URL is not set then default Webui URL value is returned
func TestGetDefaultWebuiUrl(t *testing.T) {
	origAmfConfig := AmfConfig
	defer func() { AmfConfig = origAmfConfig }()
	if err := InitConfigFactory("../util/testdata/amfcfg.yaml"); err != nil {
		t.Errorf("Error in InitConfigFactory: %v", err)
	}
	got := AmfConfig.Configuration.WebuiUri
	want := "http://webui:5001"
	assert.Equal(t, got, want, "The webui URL is not correct.")
}

// Webui URL is set to a custom value then custom Webui URL is returned
func TestGetCustomWebuiUrl(t *testing.T) {
	origAmfConfig := AmfConfig
	defer func() { AmfConfig = origAmfConfig }()
	if err := InitConfigFactory("../util/testdata/amfcfg_with_custom_webui_url.yaml"); err != nil {
		t.Errorf("Error in InitConfigFactory: %v", err)
	}
	got := AmfConfig.Configuration.WebuiUri
	want := "https://myspecialwebui:5002"
	assert.Equal(t, got, want, "The webui URL is not correct.")
}

func TestNoTelemetryConfig(t *testing.T) {
	origAmfConfig := AmfConfig
	defer func() { AmfConfig = origAmfConfig }()
	if err := InitConfigFactory("../util/testdata/no_telemetry.yaml"); err != nil {
		t.Logf("Error in InitConfigFactory: %v", err)
	}

	if AmfConfig.Configuration.Telemetry != nil {
		t.Errorf("expected no telemetry configuration, but got: %v", AmfConfig.Configuration.Telemetry)
	}
}

func TestTelemetryConfigEnabled(t *testing.T) {
	origAmfConfig := AmfConfig
	defer func() { AmfConfig = origAmfConfig }()
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
	defer func() { AmfConfig = origAmfConfig }()
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
	defer func() { AmfConfig = origAmfConfig }()
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
	defer func() { AmfConfig = origAmfConfig }()
	if err := InitConfigFactory("../util/testdata/telemetry_no_endpoint.yaml"); err == nil {
		t.Errorf("expected error when OTLP endpoint is not set, but got none")
	} else {
		t.Logf("Received expected error: %v", err)
	}
}

func TestValidateWebuiUri(t *testing.T) {
	tests := []struct {
		name    string
		uri     string
		isValid bool
	}{
		{
			name:    "valid https URI with port",
			uri:     "https://webui:5001",
			isValid: true,
		},
		{
			name:    "valid http URI with port",
			uri:     "http://webui:5001",
			isValid: true,
		},
		{
			name:    "valid https URI without port",
			uri:     "https://webui",
			isValid: true,
		},
		{
			name:    "valid http URI without port",
			uri:     "http://webui.com",
			isValid: true,
		},
		{
			name:    "invalid host",
			uri:     "http://:8080",
			isValid: false,
		},
		{
			name:    "invalid scheme",
			uri:     "ftp://webui:21",
			isValid: false,
		},
		{
			name:    "missing scheme",
			uri:     "webui:9090",
			isValid: false,
		},
		{
			name:    "missing host",
			uri:     "https://",
			isValid: false,
		},
		{
			name:    "empty string",
			uri:     "",
			isValid: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateWebuiUri(tc.uri)
			if err == nil && !tc.isValid {
				t.Errorf("expected URI: %s to be invalid", tc.uri)
			}
			if err != nil && tc.isValid {
				t.Errorf("expected URI: %s to be valid", tc.uri)
			}
		})
	}
}
