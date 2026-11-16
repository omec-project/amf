// Copyright (C) 2024-Present Intel Corporation
// SPDX-FileCopyrightText: 2025 Canonical Ltd
// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

// Package nfregistration adapts the AMF to the shared NRF registration service.
package nfregistration

import (
	"context"

	"github.com/omec-project/amf/consumer"
	"github.com/omec-project/amf/logger"
	"github.com/omec-project/openapi/v2/models"
	"github.com/omec-project/openapi/v2/nfConfigApi"
	"github.com/omec-project/openapi/v2/nrfregistration"
)

var service = newService()

func newService() *nrfregistration.Service[[]nfConfigApi.AccessAndMobility] {
	// The consumer functions are looked up on every call so tests can replace them.
	svc, err := nrfregistration.New(nrfregistration.Config[[]nfConfigApi.AccessAndMobility]{
		NfName:  "AMF",
		Log:     logger.NrfRegistrationLog,
		IsEmpty: func(cfg []nfConfigApi.AccessAndMobility) bool { return len(cfg) == 0 },
		Register: func(ctx context.Context, cfg []nfConfigApi.AccessAndMobility) (*models.NFProfile, error) {
			nfProfile, _, err := consumer.SendRegisterNFInstance(ctx, cfg)
			return nfProfile, err
		},
		Update: func(_ context.Context, patch []models.PatchItem) (*models.NFProfile, *models.ProblemDetails, error) {
			return consumer.SendUpdateNFInstance(patch)
		},
		Deregister: func(ctx context.Context) error { return consumer.SendDeregisterNFInstance(ctx) },
	})
	if err != nil {
		panic(err) // the config above is static, so this is a programming error
	}
	return svc
}

// StartNfRegistrationService starts the registration service. If the new config is empty, the NF
// deregisters from the NRF. Else, it registers to the NRF. It cancels the registration in progress
// so that only one registration process runs at the time.
func StartNfRegistrationService(ctx context.Context, accessAndMobilityConfigChan <-chan []nfConfigApi.AccessAndMobility) {
	service.Start(ctx, accessAndMobilityConfigChan)
}

// DeregisterNF stops the heartbeat and deregisters the NF from the NRF.
var DeregisterNF = func(ctx context.Context) {
	service.Deregister(ctx)
}
