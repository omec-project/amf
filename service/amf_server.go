// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0
//

package service

import (
	"context"
	"fmt"
	"net"
	"os"

	amfContext "github.com/omec-project/amf/context"
	"github.com/omec-project/amf/factory"
	"github.com/omec-project/amf/logger"
	"github.com/omec-project/amf/metrics"
	"github.com/omec-project/amf/ngap"
	"github.com/omec-project/amf/protos/sdcoreAmfServer"
	mi "github.com/omec-project/util/metricinfo"
	"google.golang.org/grpc"
)

type Server struct {
	sdcoreAmfServer.UnimplementedNgapServiceServer
}

func (s *Server) HandleMessage(srv sdcoreAmfServer.NgapService_HandleMessageServer) error {
	ctx := srv.Context()
	Amf2RanMsgChan := make(chan *sdcoreAmfServer.AmfMessage, 100)

	go func() {
		for {
			msg1 := <-Amf2RanMsgChan
			logger.GrpcLog.Infof("send Response message body from client (%s): Verbose - %s, MsgType %v GnbId: %v", msg1.AmfId, msg1.VerboseMsg, msg1.Msgtype, msg1.GnbId)
			if err := srv.Send(msg1); err != nil {
				logger.GrpcLog.Errorln("error in sending response")
			}
		}
	}()

	for {
		req, err := srv.Recv() /* TODO : handle errors */
		if err != nil {
			logger.GrpcLog.Errorln("error in SCTPLB stream", err)
			break
		} else {
			logger.GrpcLog.Debugf("receive message body from client (%s): GnbIp: %v, GnbId: %v, Verbose - %s, MsgType %v", req.SctplbId, req.GnbIpAddr, req.GnbId, req.VerboseMsg, req.Msgtype)
			switch req.Msgtype {
			case sdcoreAmfServer.MsgType_INIT_MSG:
				rsp := &sdcoreAmfServer.AmfMessage{}
				rsp.VerboseMsg = "Hello From AMF Pod !"
				rsp.Msgtype = sdcoreAmfServer.MsgType_INIT_MSG
				rsp.AmfId = os.Getenv("HOSTNAME")
				logger.GrpcLog.Debugf("send Response message body from client (%s): Verbose - %s, MsgType %v", rsp.AmfId, rsp.VerboseMsg, rsp.Msgtype)
				amfSelf := amfContext.AMF_Self()
				var ran *amfContext.AmfRan
				var ok bool
				if ran, ok = amfSelf.AmfRanFindByGnbId(req.GnbId); !ok {
					ran = amfSelf.NewAmfRanId(req.GnbId)
					if req.GnbId != "" {
						ran.GnbId = req.GnbId
						ran.RanId = ran.ConvertGnbIdToRanId(ran.GnbId)
						logger.GrpcLog.Debugf("RanID: %v for GnbId: %v", ran.RanID(), req.GnbId)
						rsp.GnbId = req.GnbId

						// send nf(gnb) status notification
						gnbStatus := mi.MetricEvent{
							EventType: mi.CNfStatusEvt,
							NfStatusData: mi.CNfStatus{
								NfType:   mi.NfTypeGnb,
								NfStatus: mi.NfStatusConnected, NfName: req.GnbId,
							},
						}

						if *factory.AmfConfig.Configuration.KafkaInfo.EnableKafka {
							if err := metrics.StatWriter.PublishNfStatusEvent(gnbStatus); err != nil {
								logger.GrpcLog.Errorf("error publishing NfStatusEvent: %v", err)
							}
						}
					}
				}
				ran.Amf2RanMsgChan = Amf2RanMsgChan
				if err := srv.Send(rsp); err != nil {
					logger.GrpcLog.Errorln("error in sending response")
				}
			case sdcoreAmfServer.MsgType_GNB_DISC:
				logger.GrpcLog.Infoln("gNB disconnected")
				ngap.HandleSCTPNotificationLb(req.GnbId)
				// send nf(gnb) status notification
				gnbStatus := mi.MetricEvent{
					EventType: mi.CNfStatusEvt,
					NfStatusData: mi.CNfStatus{
						NfType:   mi.NfTypeGnb,
						NfStatus: mi.NfStatusDisconnected, NfName: req.GnbId,
					},
				}
				if *factory.AmfConfig.Configuration.KafkaInfo.EnableKafka {
					if err := metrics.StatWriter.PublishNfStatusEvent(gnbStatus); err != nil {
						logger.GrpcLog.Errorf("error publishing NfStatusEvent: %v", err)
					}
				}
			case sdcoreAmfServer.MsgType_GNB_CONN:
				logger.GrpcLog.Infoln("new gNB Connected")
				// send nf(gnb) status notification
				gnbStatus := mi.MetricEvent{
					EventType: mi.CNfStatusEvt,
					NfStatusData: mi.CNfStatus{
						NfType:   mi.NfTypeGnb,
						NfStatus: mi.NfStatusConnected, NfName: req.GnbId,
					},
				}
				if *factory.AmfConfig.Configuration.KafkaInfo.EnableKafka {
					if err := metrics.StatWriter.PublishNfStatusEvent(gnbStatus); err != nil {
						logger.GrpcLog.Errorf("error publishing NfStatusEvent: %v", err)
					}
				}
			default:
				ngap.DispatchLb(ctx, req, Amf2RanMsgChan)
			}
		}
	}
	return nil
}

func StartGrpcServer(ctx context.Context, port int) {
	endpt := fmt.Sprintf(":%d", port)
	logger.GrpcLog.Infof("AMF gRPC server is starting on port %s", endpt)
	lisCfg := net.ListenConfig{}
	lis, err := lisCfg.Listen(ctx, "tcp", endpt)
	if err != nil {
		logger.GrpcLog.Errorf("failed to listen: %v", err)
	}

	s := Server{}

	grpcServer := grpc.NewServer()

	sdcoreAmfServer.RegisterNgapServiceServer(grpcServer, &s)

	if err := grpcServer.Serve(lis); err != nil {
		logger.GrpcLog.Errorf("failed to serve: %v", err)
	}
}
