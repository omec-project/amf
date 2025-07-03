// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// SPDX-FileCopyrightText: 2024 Canonical Ltd.
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/omec-project/config5g/logger"
	protos "github.com/omec-project/config5g/proto/sdcoreConfig"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
)

var (
	selfRestartCounter      uint32
	configPodRestartCounter uint32 = 0
)

func init() {
	s1 := rand.NewSource(time.Now().UnixNano())
	r1 := rand.New(s1)
	selfRestartCounter = r1.Uint32()
}

type PlmnId struct {
	Mcc string
	Mnc string
}

type Nssai struct {
	Sst string
	Sd  string
}

type ConfigClient struct {
	Client            protos.ConfigServiceClient
	Conn              *grpc.ClientConn
	Channel           chan *protos.NetworkSliceResponse
	Host              string
	Version           string
	MetadataRequested bool
}

type ConfClient interface {
	// PublishOnConfigChange creates a channel to perform the subscription using it.
	// On Receiving Configuration from ConfigServer, this api publishes
	// on created channel and returns the channel
	PublishOnConfigChange(metadataRequested bool, stream protos.ConfigService_NetworkSliceSubscribeClient) chan *protos.NetworkSliceResponse

	// GetConfigClientConn returns grpc connection object
	GetConfigClientConn() *grpc.ClientConn

	// sendMessagesToChannel receives the configuration changes using stream
	// send messages to communication channel
	sendMessagesToChannel(commChan chan *protos.NetworkSliceResponse, stream protos.ConfigService_NetworkSliceSubscribeClient)

	// CheckGrpcConnectivity checks the connectivity status and returns the state of connectivity
	CheckGrpcConnectivity() string

	// SubscribeToConfigServer Subscribes to a stream of NetworkSlice
	// It returns a stream if subscription is successful else returns nil.
	SubscribeToConfigServer() (stream protos.ConfigService_NetworkSliceSubscribeClient, err error)
}

// ConnectToConfigServer this API is added to control metadata from NF clients
// Connects to the ConfigServer using host address
func ConnectToConfigServer(host string) (ConfClient, error) {
	confClient := CreateConfClient(host)
	if confClient == nil {
		return nil, fmt.Errorf("create grpc channel to config pod failed")
	}
	return confClient, nil
}

// PublishOnConfigChange creates a communication channel to publish the messages from ConfigServer to the channel
// then NFs gets the messages
func (confClient *ConfigClient) PublishOnConfigChange(metadataFlag bool, stream protos.ConfigService_NetworkSliceSubscribeClient) chan *protos.NetworkSliceResponse {
	confClient.MetadataRequested = metadataFlag
	commChan := make(chan *protos.NetworkSliceResponse)
	confClient.Channel = commChan
	logger.GrpcLog.Debugln("a communication channel is created for ConfigServer")
	go confClient.sendMessagesToChannel(commChan, stream)
	return commChan
}

// CreateConfClient creates a GRPC client by connecting to GRPC server (host).
func CreateConfClient(host string) ConfClient {
	logger.GrpcLog.Debugln("create config client")
	// Second, check to see if we can reuse the gRPC connection for a new P4RT client
	conn, err := newClientConnection(host)
	if err != nil {
		logger.GrpcLog.Errorf("grpc connection failed %v", err)
		return nil
	}

	client := &ConfigClient{
		Client: protos.NewConfigServiceClient(conn),
		Conn:   conn,
		Host:   host,
	}

	return client
}

var kacp = keepalive.ClientParameters{
	Time:                20 * time.Second, // send pings every 20 seconds if there is no activity
	Timeout:             2 * time.Second,  // wait 1 second for ping ack before considering the connection dead
	PermitWithoutStream: true,             // send pings even without active streams
}

var retryPolicy = `{
		"methodConfig": [{
		  "name": [{"service": "grpc.Config"}],
		  "waitForReady": true,
		  "retryPolicy": {
			  "MaxAttempts": 4,
			  "InitialBackoff": ".01s",
			  "MaxBackoff": ".01s",
			  "BackoffMultiplier": 1.0,
			  "RetryableStatusCodes": [ "UNAVAILABLE" ]
		  }}]}`

// newClientConnection opens a GRPC connection to the host
func newClientConnection(host string) (conn *grpc.ClientConn, err error) {
	logger.GrpcLog.Debugln("dial grpc connection:", host)

	bd := 1 * time.Second
	mltpr := 1.0
	jitter := 0.2
	MaxDelay := 5 * time.Second
	bc := backoff.Config{BaseDelay: bd, Multiplier: mltpr, Jitter: jitter, MaxDelay: MaxDelay}

	crt := grpc.ConnectParams{Backoff: bc}
	dialOptions := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithKeepaliveParams(kacp), grpc.WithDefaultServiceConfig(retryPolicy), grpc.WithConnectParams(crt)}
	conn, err = grpc.NewClient(host, dialOptions...)
	if err != nil {
		return nil, fmt.Errorf("grpc newclient creation failed: %v", err)
	}
	conn.Connect()
	return conn, nil
}

// GetConfigClientConn exposes the GRPC client connection
func (confClient *ConfigClient) GetConfigClientConn() *grpc.ClientConn {
	return confClient.Conn
}

// CheckGrpcConnectivity checks the connectivity status and returns the state of connectivity
func (confClient *ConfigClient) CheckGrpcConnectivity() string {
	logger.GrpcLog.Debugln("checking GRPC connectivity status")
	state := confClient.Conn.GetState()
	return state.String()
}

// SubscribeToConfigServer Subscribes to a stream of NetworkSlice
// It returns a stream if subscription is successful else returns nil.
func (confClient *ConfigClient) SubscribeToConfigServer() (stream protos.ConfigService_NetworkSliceSubscribeClient, err error) {
	logger.GrpcLog.Debugln("SubscribeToConfigServer")
	clientId := os.Getenv("HOSTNAME")
	rreq := &protos.NetworkSliceRequest{RestartCounter: selfRestartCounter, ClientId: clientId, MetadataRequested: confClient.MetadataRequested}
	if stream, err = confClient.Client.NetworkSliceSubscribe(context.Background(), rreq); err != nil {
		return stream, fmt.Errorf("failed to subscribe: %v", err)
	}
	logger.GrpcLog.Debugln("subscribed to config server successfully")
	return stream, nil
}

// sendMessagesToChannel receives the configuration changes using stream
// and send messages to communication channel
func (confClient *ConfigClient) sendMessagesToChannel(commChan chan *protos.NetworkSliceResponse, stream protos.ConfigService_NetworkSliceSubscribeClient) {
	for {
		if stream == nil {
			time.Sleep(time.Second * 30)
			continue
		}
		rsp, err := stream.Recv()
		if err != nil {
			stream = nil
			logger.GrpcLog.Errorf("failed to receive message: %v", err)
			time.Sleep(time.Second * 5)
			continue
		}

		if rsp != nil {
			logger.GrpcLog.Infoln("stream message received")
			logger.GrpcLog.Debugf("network slices %d, RC of config pod %d", len(rsp.NetworkSlice), rsp.RestartCounter)
		}
		if configPodRestartCounter == 0 || (configPodRestartCounter == rsp.RestartCounter) {
			// first time connection or config update
			configPodRestartCounter = rsp.RestartCounter
			if len(rsp.NetworkSlice) > 0 {
				// always carries full config copy
				logger.GrpcLog.Infoln("first time config received", rsp)
				commChan <- rsp
			} else if rsp.ConfigUpdated == 1 {
				// config delete, all slices deleted
				logger.GrpcLog.Infoln("complete config deleted")
				commChan <- rsp
			}
		} else if len(rsp.NetworkSlice) > 0 {
			logger.GrpcLog.Errorln("config received after config pod restart")
			configPodRestartCounter = rsp.RestartCounter
			commChan <- rsp
		} else {
			logger.GrpcLog.Errorln("config pod is restarted and no config received")
		}
		time.Sleep(time.Second * 5)
	}
}
