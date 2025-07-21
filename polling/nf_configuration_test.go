// SPDX-FileCopyrightText: 2025 Canonical Ltd.
//
// SPDX-License-Identifier: Apache-2.0
//
/*
 * NF Polling Unit Tests
 *
 */

package polling

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/omec-project/amf/factory"
	"github.com/omec-project/openapi/models"
	"github.com/omec-project/openapi/nfConfigApi"
)

func makeAccessMobilityConfig(mcc, mnc, sst string, sd string, tacs []string) nfConfigApi.AccessAndMobility {
	sstUint64, err := strconv.ParseUint(sst, 10, 8)
	if err != nil {
		panic("invalid SST value: " + sst)
	}
	sstint := int32(sstUint64)
	return nfConfigApi.AccessAndMobility{
		PlmnId: nfConfigApi.PlmnId{
			Mcc: mcc,
			Mnc: mnc,
		},
		Snssai: nfConfigApi.Snssai{
			Sst: sstint,
			Sd:  &sd,
		},
		Tacs: tacs,
	}
}

func TstStartPollingService_Success(t *testing.T) {
	ctx := t.Context()
	originalFetchAccessAndMobilityConfig := fetchAccessAndMobilityConfig
	originalFactoryConfig := factory.AmfConfig
	defer func() {
		fetchAccessAndMobilityConfig = originalFetchAccessAndMobilityConfig
		factory.AmfConfig = originalFactoryConfig
	}()
	factory.AmfConfig = factory.Config{
		Configuration: &factory.Configuration{
			SupportTAIList:  []models.Tai{},
			PlmnSupportList: []factory.PlmnSupportItem{},
		},
	}

	expectedConfig := []nfConfigApi.AccessAndMobility{
		{
			PlmnId: nfConfigApi.PlmnId{Mcc: "001", Mnc: "01"},
			Snssai: nfConfigApi.Snssai{Sst: 1},
			Tacs:   []string{"1"},
		},
	}

	fetchAccessAndMobilityConfig = func(poller *nfConfigPoller, pollingEndpoint string) ([]nfConfigApi.AccessAndMobility, error) {
		return expectedConfig, nil
	}
	accessMobilityChan := make(chan []nfConfigApi.AccessAndMobility, 1)
	go StartPollingService(ctx, "http://dummy", func(cfg []nfConfigApi.AccessAndMobility) {
		accessMobilityChan <- cfg
	})
	time.Sleep(initialPollingInterval)

	select {
	case result := <-accessMobilityChan:
		if !reflect.DeepEqual(result, expectedConfig) {
			t.Errorf("Expected %+v, got %+v", expectedConfig, result)
		}
	case <-time.After(100 * time.Millisecond):
		t.Errorf("Timeout waiting for PLMN config")
	}
}

func TstStartPollingService_RetryAfterFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	originalFetchAccessAndMobilityConfig := fetchAccessAndMobilityConfig
	defer func() { fetchAccessAndMobilityConfig = originalFetchAccessAndMobilityConfig }()

	callCount := 0
	fetchAccessAndMobilityConfig = func(poller *nfConfigPoller, pollingEndpoint string) ([]nfConfigApi.AccessAndMobility, error) {
		callCount++
		return nil, errors.New("mock failure")
	}
	go StartPollingService(ctx, "http://dummy", func([]nfConfigApi.AccessAndMobility) {})

	time.Sleep(4 * initialPollingInterval)
	cancel()
	<-ctx.Done()

	if callCount < 2 {
		t.Error("Expected to retry after failure")
	}
	t.Logf("Tried %v times", callCount)
}

func TstStartPollingService_NoUpdateOnIdenticalConfig(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	originalFetcher := fetchAccessAndMobilityConfig
	defer func() { fetchAccessAndMobilityConfig = originalFetcher }()
	accessMobility1 := makeAccessMobilityConfig("222", "02", "1", "1", []string{"1"})
	callCount := 0
	expectedConfig := []nfConfigApi.AccessAndMobility{accessMobility1}
	fetchAccessAndMobilityConfig = func(poller *nfConfigPoller, endpoint string) ([]nfConfigApi.AccessAndMobility, error) {
		return expectedConfig, nil
	}

	ch := make(chan struct{}, 1)
	go StartPollingService(ctx, "http://dummy", func(_ []nfConfigApi.AccessAndMobility) {
		callCount++
		ch <- struct{}{}
	})

	time.Sleep(2 * initialPollingInterval)
	cancel()
	<-ctx.Done()

	if callCount != 1 {
		t.Errorf("Expected callback to be called once for new config, got %d", callCount)
	}
}

func TstStartPollingService_UpdateOnDifferentConfig(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	originalFetcher := fetchAccessAndMobilityConfig
	defer func() { fetchAccessAndMobilityConfig = originalFetcher }()
	accessMobility1 := makeAccessMobilityConfig("111", "01", "1", "1", []string{"1"})
	accessMobility2 := makeAccessMobilityConfig("111", "02", "1", "1", []string{"2"})
	callCount := 0

	fetchAccessAndMobilityConfig = func(poller *nfConfigPoller, endpoint string) ([]nfConfigApi.AccessAndMobility, error) {
		if callCount == 0 {
			return []nfConfigApi.AccessAndMobility{accessMobility1}, nil
		}
		return []nfConfigApi.AccessAndMobility{accessMobility2}, nil
	}

	ch := make(chan struct{}, 2)
	go StartPollingService(ctx, "http://dummy", func(_ []nfConfigApi.AccessAndMobility) {
		callCount++
		ch <- struct{}{}
	})

	timeout := time.After(5 * initialPollingInterval)
	for i := 0; i < 2; i++ {
		select {
		case <-ch:
			// expected update
		case <-timeout:
			t.Fatalf("Timed out waiting for config update #%d", i+1)
		}
	}

	cancel()
	<-ctx.Done()

	if callCount != 2 {
		t.Errorf("Expected callback to be called twice for different configs, got %d", callCount)
	}
}

func TestFetchAccessAndMobilityConfig(t *testing.T) {
	var accessMobilityConfigs []nfConfigApi.AccessAndMobility
	accessMobility1 := makeAccessMobilityConfig("111", "01", "1", "1", []string{"1"})
	accessMobilityConfigs = append(accessMobilityConfigs, accessMobility1)
	validJson, err := json.Marshal(accessMobilityConfigs)
	if err != nil {
		t.Fail()
	}

	tests := []struct {
		name           string
		statusCode     int
		contentType    string
		responseBody   string
		expectedError  string
		expectedResult []nfConfigApi.AccessAndMobility
	}{
		{
			name:           "200 OK with valid JSON",
			statusCode:     http.StatusOK,
			contentType:    "application/json",
			responseBody:   string(validJson),
			expectedError:  "",
			expectedResult: accessMobilityConfigs,
		},
		{
			name:          "200 OK with invalid Content-Type",
			statusCode:    http.StatusOK,
			contentType:   "text/plain",
			responseBody:  string(validJson),
			expectedError: "unexpected Content-Type: got text/plain, want application/json",
		},
		{
			name:          "400 Bad Request",
			statusCode:    http.StatusBadRequest,
			contentType:   "application/json",
			responseBody:  "",
			expectedError: "server returned 400 error code",
		},
		{
			name:          "500 Internal Server Error",
			statusCode:    http.StatusInternalServerError,
			contentType:   "application/json",
			responseBody:  "",
			expectedError: "server returned 500 error code",
		},
		{
			name:          "Unexpected Status Code 418",
			statusCode:    http.StatusTeapot,
			contentType:   "application/json",
			responseBody:  "",
			expectedError: "unexpected status code: 418",
		},
		{
			name:          "200 OK with invalid JSON",
			statusCode:    http.StatusOK,
			contentType:   "application/json",
			responseBody:  "{invalid-json}",
			expectedError: "failed to parse JSON response:",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			handler := func(w http.ResponseWriter, r *http.Request) {
				accept := r.Header.Get("Accept")
				if accept != "application/json" {
					t.Errorf("expected Accept header 'application/json', got '%s'", accept)
				}

				w.Header().Set("Content-Type", tc.contentType)
				w.WriteHeader(tc.statusCode)
				_, err = w.Write([]byte(tc.responseBody))
				if err != nil {
					t.Fail()
				}
			}
			server := httptest.NewServer(http.HandlerFunc(handler))
			poller := nfConfigPoller{
				currentAccessAndMobilityConfig: accessMobilityConfigs,
				client:                         server.Client(),
			}
			defer server.Close()

			fetchedConfig, err := poller.fetchAccessAndMobilityConfig(server.URL)

			if tc.expectedError == "" {
				if err != nil {
					t.Errorf("expected no error, got `%v`", err)
				}
				if !reflect.DeepEqual(tc.expectedResult, fetchedConfig) {
					t.Errorf("error in fetched config: expected `%v`, got `%v`", tc.expectedResult, fetchedConfig)
				}
			} else {
				if err == nil {
					t.Errorf("expected error `%v`, got nil", tc.expectedError)
				}
				if !strings.Contains(err.Error(), tc.expectedError) {
					t.Errorf("expected error `%v`, got `%v`", tc.expectedError, err)
				}
			}
		})
	}
}
