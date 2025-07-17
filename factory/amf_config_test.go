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
	if err := InitConfigFactory("../amfTest/amfcfg.yaml"); err != nil {
		t.Logf("Error in InitConfigFactory: %v", err)
	}
	got := AmfConfig.Configuration.WebuiUri
	want := "webui:9876"
	assert.Equal(t, got, want, "The webui URL is not correct.")
}

// Webui URL is set to a custom value then custom Webui URL is returned
func TestGetCustomWebuiUrl(t *testing.T) {
	if err := InitConfigFactory("../amfTest/amfcfg_with_custom_webui_url.yaml"); err != nil {
		t.Logf("Error in InitConfigFactory: %v", err)
	}
	got := AmfConfig.Configuration.WebuiUri
	want := "myspecialwebui:9872"
	assert.Equal(t, got, want, "The webui URL is not correct.")
}

func TestNoTelemetryConfig(t *testing.T) {
	if err := InitConfigFactory("testdata/no_telemetry.yaml"); err != nil {
		t.Logf("Error in InitConfigFactory: %v", err)
	}

	if AmfConfig.Configuration.Telemetry != nil {
		t.Errorf("Expected no telemetry configuration, but got: %v", AmfConfig.Configuration.Telemetry)
	}
}

func TestTelemetryConfigEnabled(t *testing.T) {
	if err := InitConfigFactory("testdata/telemetry.yaml"); err != nil {
		t.Logf("Error in InitConfigFactory: %v", err)
	}

	if AmfConfig.Configuration.Telemetry == nil {
		t.Fatalf("Expected telemetry configuration to be present, but it is nil")
	}

	if !AmfConfig.Configuration.Telemetry.Enabled {
		t.Errorf("Expected telemetry to be enabled, but it is not")
	}

	if AmfConfig.Configuration.Telemetry.OtlpEndpoint == "" {
		t.Errorf("Expected OTLP endpoint to be set, but it is empty")
	}

	if AmfConfig.Configuration.Telemetry.Ratio == nil || *AmfConfig.Configuration.Telemetry.Ratio != 0.4 {
		t.Errorf("Expected telemetry ratio to be 0.4, but got: %v", AmfConfig.Configuration.Telemetry.Ratio)
	}
}

func TestTelemetryConfigEnabledNoRatioDefaultsTo1(t *testing.T) {
	if err := InitConfigFactory("testdata/telemetry_no_ratio.yaml"); err != nil {
		t.Logf("Error in InitConfigFactory: %v", err)
	}

	if AmfConfig.Configuration.Telemetry == nil {
		t.Fatalf("Expected telemetry configuration to be present, but it is nil")
	}

	if !AmfConfig.Configuration.Telemetry.Enabled {
		t.Errorf("Expected telemetry to be enabled, but it is not")
	}

	if AmfConfig.Configuration.Telemetry.OtlpEndpoint == "" {
		t.Errorf("Expected OTLP endpoint to be set, but it is empty")
	}

	if AmfConfig.Configuration.Telemetry.Ratio == nil || *AmfConfig.Configuration.Telemetry.Ratio != 1.0 {
		t.Errorf("Expected telemetry ratio to be 1.0, but got: %v", AmfConfig.Configuration.Telemetry.Ratio)
	}
}

func TestTelemetryConfigEnabledRatio0Stays0(t *testing.T) {
	if err := InitConfigFactory("testdata/telemetry_zero_ratio.yaml"); err != nil {
		t.Logf("Error in InitConfigFactory: %v", err)
	}

	if AmfConfig.Configuration.Telemetry == nil {
		t.Fatalf("Expected telemetry configuration to be present, but it is nil")
	}

	if !AmfConfig.Configuration.Telemetry.Enabled {
		t.Errorf("Expected telemetry to be enabled, but it is not")
	}

	if AmfConfig.Configuration.Telemetry.OtlpEndpoint == "" {
		t.Errorf("Expected OTLP endpoint to be set, but it is empty")
	}

	if AmfConfig.Configuration.Telemetry.Ratio == nil || *AmfConfig.Configuration.Telemetry.Ratio != 0.0 {
		t.Errorf("Expected telemetry ratio to be 0.0, but got: %v", AmfConfig.Configuration.Telemetry.Ratio)
	}
}

func TestTelemetryConfigEnabledNoEndpointReturnsError(t *testing.T) {
	if err := InitConfigFactory("testdata/telemetry_no_endpoint.yaml"); err == nil {
		t.Errorf("Expected error when OTLP endpoint is not set, but got none")
	} else {
		t.Logf("Received expected error: %v", err)
	}
}
