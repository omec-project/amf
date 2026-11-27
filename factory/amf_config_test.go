// Copyright (C) 2026 Intel Corporation
// SPDX-FileCopyrightText: 2024 Canonical Ltd.
// SPDX-License-Identifier: Apache-2.0

package factory

import (
	"os"
	"path/filepath"
	"strings"
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

// TestLiBulkSwitchesAreTriState covers the two keys that carry an agreement the standard
// leaves to the deployment, at the layer where an operator's `false` is at risk of becoming
// nothing at all.
//
// The third case is the one worth having: a value that is not a boolean must be refused,
// not read as unset. Unset is the permissive answer for bulk deactivation, so silently
// defaulting a typo would leave the element performing exactly the operation the operator
// wrote the key to withhold. It is the rule the UPF's config applies to its keepalive
// window. The refusal is recorded for the LI subsystem, like an unknown key, and the
// configuration load itself succeeds.
func TestLiBulkSwitchesAreTriState(t *testing.T) {
	load := func(t *testing.T, extra string) error {
		t.Helper()

		orig := AmfConfig
		t.Cleanup(func() { AmfConfig = orig; liBlockErr = nil })

		path := filepath.Join(t.TempDir(), "amfcfg.yaml")
		body := `configuration:
  li:
    x1Listen: ":8443"
    neId: amf-1` + extra + "\n"
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}

		return InitConfigFactory(path)
	}

	t.Run("both switches carry through", func(t *testing.T) {
		if err := load(t, `
    deactivateAllTasks: false
    removeAllDestinations: true`); err != nil {
			t.Fatalf("load: %v", err)
		}
		if err := LiBlockError(); err != nil {
			t.Fatalf("li block refused: %v", err)
		}
		if got := AmfConfig.Configuration.Li.DeactivateAllTasks; got == nil || *got {
			t.Errorf("deactivateAllTasks = %v, want a stated false — a dropped restriction leaves "+
				"the element performing the operation the operator withheld", got)
		}
		if got := AmfConfig.Configuration.Li.RemoveAllDestinations; got == nil || !*got {
			t.Errorf("removeAllDestinations = %v, want a stated true", got)
		}
	})

	t.Run("saying nothing is not saying false", func(t *testing.T) {
		if err := load(t, ""); err != nil {
			t.Fatalf("load: %v", err)
		}
		if err := LiBlockError(); err != nil {
			t.Fatalf("li block refused: %v", err)
		}
		if AmfConfig.Configuration.Li.DeactivateAllTasks != nil || AmfConfig.Configuration.Li.RemoveAllDestinations != nil {
			t.Error("an li block that states no agreement must leave both unset, so the " +
				"standard's own defaults apply")
		}
	})

	t.Run("a value that is not a boolean is refused", func(t *testing.T) {
		if err := load(t, "\n    deactivateAllTasks: perhaps"); err != nil {
			t.Fatalf("a value of the wrong type in the li block failed the whole configuration "+
				"load, which stops the AMF: %v", err)
		}
		if LiBlockError() == nil {
			t.Errorf("an unparseable value was accepted as %v; it must be refused rather than "+
				"read as unset, which is the permissive answer",
				AmfConfig.Configuration.Li.DeactivateAllTasks)
		}
	})
}

// Misspellings of the two LI keys whose defaults fail unsafely, shared by the tests that
// check a typo in either is refused without stopping the network function.
const (
	typoKeepaliveTimeout = "keepaliveTimeut"
	typoAdmfURL          = "admf_url"
)

// TestLiBlockRefusesUnknownKeys covers the whole startup path, not strictLiBlock alone: the
// property is that a mistyped LI key is refused and recorded for the LI subsystem without failing
// the configuration load, and that is only true if InitConfigFactory calls the check.
//
// Both keys below are chosen because their defaults are not what the operator asked for. A
// dropped `keepaliveTimeout` leaves the X1 keepalive procedure on the clause default rather than
// the operator's TIME_P2; a dropped `admfUrl` leaves the fault channel a no-op, so nothing it is required to
// report — including a misconfiguration — reaches the ADMF. Neither is visible from outside: the
// element runs, answers X1, and looks provisioned.
func TestLiBlockRefusesUnknownKeys(t *testing.T) {
	write := func(t *testing.T, body string) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), "amfcfg.yaml")
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}

		return path
	}

	const good = `info:
  version: 1.0.0
configuration:
  li:
    x1Listen: ":8443"
    neId: amf-1
    admfUrl: https://admf:9443
    keepaliveTimeout: 30s
`

	t.Run("a conformant li block starts", func(t *testing.T) {
		orig := AmfConfig
		t.Cleanup(func() { AmfConfig = orig })

		if err := InitConfigFactory(write(t, good)); err != nil {
			t.Fatalf("a conformant li block was refused: %v", err)
		}
		if AmfConfig.Configuration.Li.KeepaliveTimeout != "30s" {
			t.Errorf("keepaliveTimeout = %q, want 30s — the li decode must not lose a value it can read",
				AmfConfig.Configuration.Li.KeepaliveTimeout)
		}
	})

	for _, tt := range []struct {
		name string
		typo string
	}{
		{"a misspelled fail-safe window", typoKeepaliveTimeout},
		{"a misspelled ADMF endpoint", typoAdmfURL},
	} {
		t.Run(tt.name, func(t *testing.T) {
			orig := AmfConfig
			t.Cleanup(func() { AmfConfig = orig })

			body := strings.Replace(good, "admfUrl", tt.typo, 1)
			if tt.typo == typoKeepaliveTimeout {
				body = strings.Replace(good, "keepaliveTimeout", tt.typo, 1)
			}
			// **Both halves.** The key must be refused, *and* the network function must still
			// come up: a configuration load that fails takes the service-based interface, NGAP, registration and every UE this AMF
			// serves with it, over a typo in an optional subsystem — and a network function that
			// will not start announces to every operator and peer that it is LI-provisioned,
			// which is the disclosure undetectability exists to prevent. Asserting only the
			// refusal would pass on an AMF that does not start. See LiBlockError.
			t.Cleanup(func() { liBlockErr = nil })

			if err := InitConfigFactory(write(t, body)); err != nil {
				t.Fatalf("a misspelled LI key stopped the whole configuration load, which stops "+
					"the network function: %v", err)
			}

			err := LiBlockError()
			if err == nil {
				t.Fatalf("%s was accepted, so the setting the operator wrote never reached the "+
					"element and its unsafe default stands with nothing saying so", tt.typo)
			}
			if !strings.Contains(err.Error(), tt.typo) {
				t.Errorf("the refusal does not name the key that was wrong: %v", err)
			}
		})
	}

	t.Run("a key outside the li block is still tolerated", func(t *testing.T) {
		orig := AmfConfig
		t.Cleanup(func() { AmfConfig = orig })

		// The scope of the check, asserted rather than assumed. Upstream adds configuration
		// keys; if strictness leaked past the li block, a configuration carrying a key this
		// version does not model would stop the AMF from starting.
		body := strings.Replace(good, "configuration:\n", "configuration:\n  aKeyThisForkDoesNotModel: 1\n", 1)
		if err := InitConfigFactory(write(t, body)); err != nil {
			t.Fatalf("an unmodelled key outside the li block was refused, which would stop this "+
				"fork starting on the next upstream field: %v", err)
		}
	})

	// A mapping may not repeat a key, and the decode of the whole file refuses one that does,
	// which stopped the AMF with an error naming the key. The repeated block is an unusable li
	// block like any other: refused for interception, with the rest of the configuration in
	// force, and the first block read so the refusal can reach the ADMF it names.
	t.Run("a repeated li key", func(t *testing.T) {
		orig := AmfConfig
		t.Cleanup(func() { AmfConfig = orig })
		t.Cleanup(func() { liBlockErr = nil })

		body := "info:\n  version: 1.0.0\nconfiguration:\n  amfName: AMF\n" +
			strings.TrimPrefix(good, "info:\n  version: 1.0.0\nconfiguration:\n") +
			"  li:\n    neId: amf-2\n  amfId: cafe01\n"
		if err := InitConfigFactory(write(t, body)); err != nil {
			t.Fatalf("a repeated li key stopped the whole configuration load, which stops the "+
				"network function: %v", err)
		}
		if LiBlockError() == nil {
			t.Fatal("a repeated li key was accepted")
		}
		if AmfConfig.Configuration.AmfName != "AMF" || AmfConfig.Configuration.AmfId != "cafe01" {
			t.Errorf("amfName %q, amfId %q: the configuration around the li blocks was not decoded",
				AmfConfig.Configuration.AmfName, AmfConfig.Configuration.AmfId)
		}
		if li := AmfConfig.Configuration.Li; li == nil || li.AdmfURL != "https://admf:9443" {
			t.Errorf("li = %+v, want the first block read, with its admfUrl", li)
		}
	})

	t.Run("no li block at all", func(t *testing.T) {
		orig := AmfConfig
		t.Cleanup(func() { AmfConfig = orig })

		if err := InitConfigFactory(write(t, "info:\n  version: 1.0.0\nconfiguration:\n  amfName: AMF\n")); err != nil {
			t.Fatalf("a configuration without interception was refused: %v", err)
		}
	})
}
