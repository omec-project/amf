package service

import (
	"fmt"
	"github.com/free5gc/amf/ngap"
	"google.golang.org/grpc"
	"log"
	"net"
	"os"
	"github.com/omec-project/amf/protos/sdcoreAmfServer"
)

type Server struct {
	sdcoreAmfServer.UnimplementedNgapServiceServer
}

func (s *Server) HandleMessage(srv sdcoreAmfServer.NgapService_HandleMessageServer) error {
	var Amf2RanMsgChan chan *sdcoreAmfServer.Message
	Amf2RanMsgChan = make(chan *sdcoreAmfServer.Message, 100)

	go func() {
		for {
			select {
			case msg1 := <-Amf2RanMsgChan:
				log.Printf("Send Response message body from client (%s): Verbose - %s, MsgType %v ", msg1.AmfId, msg1.VerboseMsg, msg1.Msgtype)
				if err := srv.Send(msg1); err != nil {
					log.Println("Error in sending response")
				}
			}
		}
	}()

	for {
		req, err := srv.Recv() /* TODO : handle errors */
		if err != nil {
			log.Println("Error in SCTPLB stream ", err)
			break
		} else {
			log.Printf("Receive message body from client (%s): Verbose - %s, MsgType %v ", req.SctplbId, req.VerboseMsg, req.Msgtype)
			if req.Msgtype == sdcoreAmfServer.MsgType_INIT_MSG {
				rsp := &sdcoreAmfServer.Message{}
				rsp.VerboseMsg = "Hello From AMF Pod !"
				rsp.Msgtype = sdcoreAmfServer.MsgType_INIT_MSG
				rsp.AmfId = os.Getenv("HOSTNAME")
				log.Printf("Send Response message body from client (%s): Verbose - %s, MsgType %v ", rsp.AmfId, rsp.VerboseMsg, rsp.Msgtype)
				if err := srv.Send(rsp); err != nil {
					log.Println("Error in sending response")
				}
			} else if req.Msgtype == sdcoreAmfServer.MsgType_GNB_DISC {
					log.Println("GNB disconnected")
				ngap.HandleSCTPNotificationLb(req.GnbId)
			} else if req.Msgtype == sdcoreAmfServer.MsgType_GNB_CONN {
					log.Println("New GNB Connected ")
			} else {
				ngap.DispatchLb(req.GnbId, req.Msg, Amf2RanMsgChan)
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
