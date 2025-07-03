<!--
SPDX-FileCopyrightText: 2022 Open Networking Foundation <info@opennetworking.org>

SPDX-License-Identifier: Apache-2.0

-->
# Distributed Resource Sharing Module (DRSM)

## Block diagram

![DRSM Usage in AMF](/drsm/images/drsm.png)

## Introduction

It is very common in service-based architecture to share resources among multiple instances of the microservices. Just to explain the DRSM concepts and design, we can take an example of the Access Management Function (AMF) network function, where gNodeB and AMF connect over the N2 interface and NGAP protocol is used for this interface. 3GPP defines the specification of this interface. NGAP uses SCTP transport protocol for communication between gNodeB and AMF. Multiple User Equipment (UE) signaling traffic is sent over the N2 interface. Users are identified by NGAPIDs. Each subscriber is identified by the NGAAP ID of AMF and NGAAP ID of gNodeB. So, it's very important for AMF and gNodeB to assign a unique NGAPP ID to UE. When single AMF instance is running in the system, then there is no issue and assigned IDs are unique. But when multiple instances of AMF are running in the system then the DRSM module helps the AMF instance for unique ID assignment.

## Module

DRSM is a go module and can be used by go program to start using the resource allocation functionality. In 5G core it is used by SCTP load balancer & AMF.

## Initialization

During DRSM module init, AMF provides all required information to DRSM and reserves a few unique chunks. This is done through mongodb operation internally. The DRSM module provides APIs to assign IDs from these chunks. When Chunk is fully utilized then AMF creates a new unique chunk and starts assigning the Ids from the newly created chunk.

## Load Balancer

SCTP load balancer keeps track of all the created chunks and their corresponding owner. When UE messages are received at SCTP load balancer then it reads the NGAPP id from the messages and finds the corresponding AMF instances using DRSM local lookup APIs.  This way the same AMF continues to get the same subscriber traffic and we achieve subscriber pinning to specific instances.

## During redundancy event

If any AMF instance crashes, then the chunks owned by the crashed AMF instances are claimed by other running AMF instances and this functionality is part of DRSM. This way load is distributed across remaining AMF instances after the redundancy event. Load balancer learns about change in owner and future messages are sent to updated owner.  In case a message is not received at the right AMF instance then for brief period of time redirect functionality gets exercised.

## Resources can be

    - integer numbers (TEID, SEID, NGAPIDs,TMSI,...)
    - IP address pool

## Modes

    - demux mode : just listen and get mapping about PODS and their resource assignments
        * Can be used by sctplb, upf-adapter

    - Client mode : Learn about other clients and their resource mappings
        * can be used by AMF pods, SMF pods

## Dependency

    - MongoDB should run in cluster(replicaset) Mode or sharded Mode

## Limitation

    - If application wants to use multiple Id for same session then its good to use single id is used for multiple purpose.
      e.g. AMF can use single id for ngapid as well as tmsi

## Testing
    
    - All the DRSM clients discover other clients through pub/sub
    - Allocate resource id ( indirectly chunk). Other Pods should get notification of newly allocated chunk
    - POD down event should be detected
    - Get candidate ORPHAN chunk list once POD down detected
    - CLAIM chunk to change owner
    - Through notification other PODS should detect if CHUNK is claimed
    - Run large number of clients and bring down replicaset by 1..All other pod would try to claim chunks of crashed pod. We should see only 1 client claiming it successfully
    - If some pod is started late and already there are number of documents in collections. Then does stream provide old docs as well ? No. Added code to read existing docs.
    - Multiple Pods trying to allocate same Chunkid. dbInsert only succeeds for one client. Does DRSM handle error and retry other Chunk
    - Clear Separation of demux API vs regular CLIENT API
    - Callback should be available where chunk scanning (resource id usage) can be done with help of application
    - Pod identity is IP address + Pod Name
    - Allocate more than 1000 ids.. See if New chunk is allocated

## TODO

    -  MongoDB instance restart
    -  Rst counter to be appended to identify pod. PodId should be = K8s Pod Id + Rst Count.
       This makes sure that restarted pod even if it comes with same name then we treat it differently
