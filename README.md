<!--
SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
Copyright 2019 free5GC.org

SPDX-License-Identifier: Apache-2.0

-->
# amf

It is a control plane function in the 5G core network. AMF supports termination of NAS signalling, 
NAS ciphering & integrity protection, registration management, connection management, mobility 
management, access authentication and authorization, security context management. 

## AMF Block Diagram
![AMF Block Diagram](/docs/images/README-AMF.png)

AMF takes configuration from Configuration Service. Configuration is handled at Network Slice level.
Configuration (Network Slices) can be added, removed & deleted.  AMF has prometheus interface to export
metrics. Metrics include connected gNodeB's and its status.

## The SD-Core AMF currently supports the following functionalities :
- Termination of RAN CP interface (N2). 
- Termination of NAS (N1), NAS ciphering and integrity protection. 
- Registration management. 
- Connection management. 
- Reachability management. 
- Mobility Management.
- Provide transport for SM messages between UE and SMF. 
- Transparent proxy for routing SM messages. 
- Access Authentication. 
- Access Authorization. 

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



Compliance of the 5G Network functions can be found at [5G Compliance ](https://docs.sd-core.opennetworking.org/master/overview/3gpp-compliance-5g.html)

## Reach out to us thorugh 

1. #sdcore-dev channel in [ONF Community Slack](https://onf-community.slack.com/)
2. Raise Github issues
