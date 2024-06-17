// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0
//

package service

import (
	"fmt"
	"log"
	"net"
	"os"

	"github.com/omec-project/amf/context"
	"github.com/omec-project/amf/factory"
	"github.com/omec-project/amf/metrics"
	"github.com/omec-project/amf/ngap"
	"github.com/omec-project/amf/protos/sdcoreAmfServer"
	mi "github.com/omec-project/metricfunc/pkg/metricinfo"
	"google.golang.org/grpc"
)

type Server struct {
	sdcoreAmfServer.UnimplementedNgapServiceServer
}

func (s *Server) HandleMessage(srv sdcoreAmfServer.NgapService_HandleMessageServer) error {
	Amf2RanMsgChan := make(chan *sdcoreAmfServer.AmfMessage, 100)

	go func() {
		for {
			msg1 := <-Amf2RanMsgChan
			log.Printf("Send Response message body from client (%s): Verbose - %s, MsgType %v GnbId: %v", msg1.AmfId, msg1.VerboseMsg, msg1.Msgtype, msg1.GnbId)
			if err := srv.Send(msg1); err != nil {
				log.Println("Error in sending response")
			}
		}
	}()

	for {
		req, err := srv.Recv() /* TODO : handle errors */
		if err != nil {
			log.Println("Error in SCTPLB stream ", err)
			break
		} else {
			log.Printf("Receive message body from client (%s): GnbIp: %v, GnbId: %v, Verbose - %s, MsgType %v ", req.SctplbId, req.GnbIpAddr, req.GnbId, req.VerboseMsg, req.Msgtype)
			if req.Msgtype == sdcoreAmfServer.MsgType_INIT_MSG {
				rsp := &sdcoreAmfServer.AmfMessage{}
				rsp.VerboseMsg = "Hello From AMF Pod !"
				rsp.Msgtype = sdcoreAmfServer.MsgType_INIT_MSG
				rsp.AmfId = os.Getenv("HOSTNAME")
				log.Printf("Send Response message body from client (%s): Verbose - %s, MsgType %v ", rsp.AmfId, rsp.VerboseMsg, rsp.Msgtype)
				amfSelf := context.AMF_Self()
				var ran *context.AmfRan
				var ok bool
				if ran, ok = amfSelf.AmfRanFindByGnbId(req.GnbId); !ok {
					ran = amfSelf.NewAmfRanId(req.GnbId)
					if req.GnbId != "" {
						ran.GnbId = req.GnbId
						ran.RanId = ran.ConvertGnbIdToRanId(ran.GnbId)
						log.Printf("RanID: %v for GnbId: %v", ran.RanID(), req.GnbId)
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
								log.Printf("Error publishing NfStatusEvent: %v", err)
							}
						}
					}
				}
				ran.Amf2RanMsgChan = Amf2RanMsgChan
				if err := srv.Send(rsp); err != nil {
					log.Println("Error in sending response")
				}
			} else if req.Msgtype == sdcoreAmfServer.MsgType_GNB_DISC {
				log.Println("GNB disconnected")
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
						log.Printf("Error publishing NfStatusEvent: %v", err)
					}
				}
			} else if req.Msgtype == sdcoreAmfServer.MsgType_GNB_CONN {
				log.Println("New GNB Connected ")
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
						log.Printf("Error publishing NfStatusEvent: %v", err)
					}
				}
			} else {
				ngap.DispatchLb(req, Amf2RanMsgChan)
			}
		}
	}
	return nil
}

func StartGrpcServer(port int) {
	endpt := fmt.Sprintf(":%d", port)
	fmt.Println("Listen - ", endpt)
	lis, err := net.Listen("tcp", endpt)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	s := Server{}

	grpcServer := grpc.NewServer()

	sdcoreAmfServer.RegisterNgapServiceServer(grpcServer, &s)

	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %s", err)
	}
}
