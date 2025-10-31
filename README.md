<!--
SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
SPDX-FileCopyrightText: 2025 Canonical Ltd
Copyright 2019 free5GC.org

SPDX-License-Identifier: Apache-2.0
-->
[![Go Report Card](https://goreportcard.com/badge/github.com/omec-project/amf)](https://goreportcard.com/report/github.com/omec-project/amf)

# amf
It is a control plane function in the 5G core network. AMF supports termination
of NAS signalling, NAS ciphering & integrity protection, registration
management, connection management, mobility management, access authentication
and authorization, security context management.

## AMF flow Diagram
![AMF Flow Diagram](/docs/images/README-AMF.png)

AMF takes configuration from NFConfig Service. Configuration is handled at
 the Network Slice level. Configuration (Network Slices) can be added, removed and
deleted. AMF has a prometheus interface to export metrics. Metrics include
connected gNodeB's and its status.

## Dynamic Network configuration (via webconsole)

AMF polls the webconsole every 5 seconds to fetch the latest Access and Mobility (PLMN, SNssai, TACs) configuration.

### Setting Up Polling

Include the `webuiUri` of the webconsole in the configuration file
```
configuration:
  ...
  webuiUri: https://webui:5001 # or http://webui:5001
  ...
```
The scheme (http:// or https://) must be explicitly specified. If no parameter is specified,
AMF will use `http://webui:5001` by default.

### HTTPS Support

If the webconsole is served over HTTPS and uses a custom or self-signed certificate,
you must install the root CA certificate into the trust store of the AMF environment.

Check the official guide for installing root CA certificates on Ubuntu:
[Install a Root CA Certificate in the Trust Store](https://documentation.ubuntu.com/server/how-to/security/install-a-root-ca-certificate-in-the-trust-store/index.html)

## The SD-Core AMF currently supports the following functionalities:
- Termination of RAN CP interface (N2)
- Termination of NAS (N1), NAS ciphering and integrity protection
- Registration management
- Connection management
- Reachability management
- Mobility Management
- Provide transport for SM messages between UE and SMF
- Transparent proxy for routing SM messages
- Access Authentication
- Access Authorization
- Tracing with OpenTelemetry

## Supported Procedures:
- Registration/Deregistration
- Registration update
- UE initiated Service request
- N2 Handover
- Xn handover
- PDU Establishment Request/Release
- Paging
- CN High Availibilty and Stateless session support
- AMF metrics are available via metricfunc on the 5g Grafana dashboard

## Upcoming Changes in AMF

Compliance of the 5G Network functions can be found at [5G Compliance](https://docs.sd-core.opennetworking.org/main/overview/3gpp-compliance-5g.html)

## Reach out to us through

1. #sdcore-dev channel in [Aether Community Slack](https://aether5g-project.slack.com)
2. Raise Github [issues](https://github.com/omec-project/amf/issues/new)
