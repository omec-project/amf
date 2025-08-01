// SPDX-FileCopyrightText: 2025 Canonical Ltd

// SPDX-License-Identifier: Apache-2.0
//

package polling

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/mohae/deepcopy"
	"github.com/omec-project/amf/logger"
	"github.com/omec-project/openapi/nfConfigApi"
)

const (
	initialPollingInterval = 5 * time.Second
	pollingMaxBackoff      = 40 * time.Second
	pollingBackoffFactor   = 2
	pollingPath            = "/nfconfig/access-mobility"
)

type nfConfigPoller struct {
	currentAccessAndMobilityConfig []nfConfigApi.AccessAndMobility
	client                         *http.Client
}

// StartPollingService initializes the polling service and starts it. The polling service
// continuously makes a HTTP GET request to the webconsole and updates the network configuration
func StartPollingService(ctx context.Context, webuiUri string, registrationChannel, contextUpdateChannel chan []nfConfigApi.AccessAndMobility) {
	poller := nfConfigPoller{
		currentAccessAndMobilityConfig: []nfConfigApi.AccessAndMobility{},
		client:                         &http.Client{Timeout: initialPollingInterval},
	}
	interval := initialPollingInterval
	pollingEndpoint := webuiUri + pollingPath
	logger.PollConfigLog.Infof("started polling service on %s every %v", pollingEndpoint, initialPollingInterval)
	for {
		select {
		case <-ctx.Done():
			logger.PollConfigLog.Infoln("polling service shutting down")
			return
		case <-time.After(interval):
			newAccessMobilityConfig, err := fetchAccessAndMobilityConfig(&poller, pollingEndpoint)
			if err != nil {
				interval = minDuration(interval*time.Duration(pollingBackoffFactor), pollingMaxBackoff)
				logger.PollConfigLog.Errorf("polling error. Retrying in %v: %+v", interval, err)
				continue
			}
			interval = initialPollingInterval
			if !reflect.DeepEqual(newAccessMobilityConfig, poller.currentAccessAndMobilityConfig) {
				logger.PollConfigLog.Infof("Access and Mobility config changed. New Access and Mobility: %+v", newAccessMobilityConfig)
				registrationChannel <- newAccessMobilityConfig
				poller.currentAccessAndMobilityConfig = deepcopy.Copy(newAccessMobilityConfig).([]nfConfigApi.AccessAndMobility)
				contextUpdateChannel <- newAccessMobilityConfig
			} else {
				logger.PollConfigLog.Debugf("Access and Mobility config did not change %+v", newAccessMobilityConfig)
			}
		}
	}
}

var fetchAccessAndMobilityConfig = func(p *nfConfigPoller, endpoint string) ([]nfConfigApi.AccessAndMobility, error) {
	return p.fetchAccessAndMobilityConfig(endpoint)
}

func (p *nfConfigPoller) fetchAccessAndMobilityConfig(pollingEndpoint string) ([]nfConfigApi.AccessAndMobility, error) {
	ctx, cancel := context.WithTimeout(context.Background(), initialPollingInterval)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pollingEndpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP GET %v failed: %w", pollingEndpoint, err)
	}
	defer resp.Body.Close()

	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		return nil, fmt.Errorf("unexpected Content-Type: got %s, want application/json", contentType)
	}

	switch resp.StatusCode {
	case http.StatusOK:
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read response body: %w", err)
		}

		var config []nfConfigApi.AccessAndMobility
		if err := json.Unmarshal(body, &config); err != nil {
			return nil, fmt.Errorf("failed to parse JSON response: %w", err)
		}
		return config, nil

	case http.StatusBadRequest, http.StatusInternalServerError:
		return nil, fmt.Errorf("server returned %d error code", resp.StatusCode)

	default:
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
