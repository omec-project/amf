// SPDX-FileCopyrightText: 2024 Intel Corporation
// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

package main

import (
	"context"
	"fmt"
	"os"

	"github.com/omec-project/amf/logger"
	"github.com/omec-project/amf/service"
	"github.com/urfave/cli/v3"
)

var AMF = &service.AMF{}

func main() {
	app := &cli.Command{}
	app.Name = "amf"
	logger.AppLog.Infoln(app.Name)
	app.Usage = "Access & Mobility Management function"
	app.UsageText = "amf -cfg <amf_config_file.conf>"
	app.Action = action
	app.Flags = AMF.GetCliCmd()
	if err := app.Run(context.Background(), os.Args); err != nil {
		logger.AppLog.Fatalf("AMF run error: %v", err)
	}
}

func action(ctx context.Context, c *cli.Command) error {
	if err := AMF.Initialize(c); err != nil {
		logger.CfgLog.Errorf("%+v", err)
		return fmt.Errorf("failed to initialize")
	}

	AMF.WatchConfig()

	AMF.Start()

	return nil
}
