// Copyright (C) 2026 Intel Corporation
// SPDX-FileCopyrightText: 2025 Canonical Ltd
// SPDX-License-Identifier: Apache-2.0

package polling

import (
	"context"
	"time"

	"github.com/omec-project/amf/logger"
	"github.com/omec-project/openapi/v2/nfConfigApi"
	"github.com/omec-project/openapi/v2/nfconfigpoll"
)

const (
	initialPollingInterval = 5 * time.Second
	pollingPath            = "/nfconfig/access-mobility"
)

// StartPollingService initializes the polling service and starts it. The polling service
// continuously makes a HTTP GET request to the webconsole and updates the network configuration
func StartPollingService(ctx context.Context, webuiUri string, registrationChannel, contextUpdateChannel chan []nfConfigApi.AccessAndMobility) {
	poller, err := newPoller(webuiUri, registrationChannel, contextUpdateChannel, initialPollingInterval)
	if err != nil {
		logger.PollConfigLog.Errorf("failed to start polling service: %v", err)
		return
	}
	poller.Run(ctx)
}

func newPoller(webuiUri string, registrationChannel, contextUpdateChannel chan<- []nfConfigApi.AccessAndMobility, interval time.Duration) (*nfconfigpoll.Poller[[]nfConfigApi.AccessAndMobility], error) {
	return nfconfigpoll.New(nfconfigpoll.Config[[]nfConfigApi.AccessAndMobility]{
		Name:      "Access and Mobility",
		Endpoint:  webuiUri + pollingPath,
		Log:       logger.PollConfigLog,
		Interval:  interval,
		Initial:   []nfConfigApi.AccessAndMobility{},
		Normalize: nfconfigpoll.JSONRoundTrip[[]nfConfigApi.AccessAndMobility],
		OnChange: func(ctx context.Context, cfg []nfConfigApi.AccessAndMobility) error {
			if err := nfconfigpoll.Send(ctx, registrationChannel, cfg); err != nil {
				return err
			}
			return nfconfigpoll.Send(ctx, contextUpdateChannel, cfg)
		},
	})
}
