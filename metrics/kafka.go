package metrics

import (
	"context"
	"encoding/json"
	"os"

	"github.com/omec-project/amf/factory"
	"github.com/omec-project/amf/logger"
	"github.com/segmentio/kafka-go"
)

type Writer struct {
	kafkaWriter kafka.Writer
}

var StatWriter Writer

type CoreMsgType struct {
	MsgType    string `json:"msgType,omitempty"`
	SourceNfIp string `json:"sourceNfIp,omitempty"`
}

type CoreEventType int64

const (
	CSubscriberEvt CoreEventType = iota
	CMsgTypeEvt
	CNfStatusEvt
)

func (e CoreEventType) String() string {
	switch e {
	case CSubscriberEvt:
		return "SubscriberEvt"
	case CMsgTypeEvt:
		return "MsgTypeEvt"
	case CNfStatusEvt:
		return "CNfStatusEvt"
	}
	return "Unknown"
}

type NfStatusType string

const (
	NfStatusConnected    NfStatusType = "Connected"
	NfStatusDisconnected NfStatusType = "Disconnected"
)

type NfType string

const (
	NfTypeSmf NfType = "SMF"
	NfTypeAmf NfType = "AMF"
	NfTypeUPF NfType = "UPF"
	NfTypeGnb NfType = "GNB"
	NfTypeEnd NfType = "Invalid"
)

type CNfStatus struct {
	NfType   NfType       `json:"nfType,omitempty"`
	NfStatus NfStatusType `json:"nfStatus,omitempty"`
	NfName   string       `json:"nfName,omitempty"`
}

type SubscriberOp uint

const (
	SubsOpAdd SubscriberOp = iota + 1
	SubsOpMod
	SubsOpDel
)

type CoreSubscriberData struct {
	Subscriber CoreSubscriber `json:"subscriber,omitempty"`
	Operation  SubscriberOp   `json:"subsOp,omitempty"`
}

//Sent by NFs(Producers) and received by Metric Function
type MetricEvent struct {
	EventType      CoreEventType      `json:"eventType,omitempty"`
	SubscriberData CoreSubscriberData `json:"subscriberData,omitempty"`
	MsgType        CoreMsgType        `json:"coreMsgType,omitempty"`
	NfStatusData   CNfStatus          `json:"nfStatusData"`
}

type CoreSubscriber struct {
	Version     int    `json:"version,omitempty"`
	Imsi        string `json:"imsi,omitempty"` //key
	SmfId       string `json:"smfId,omitempty"`
	SmfIp       string `json:"smfIp,omitempty"`
	SmfSubState string `json:"smfSubState,omitempty"` //Connected, Idle, DisConnected
	IPAddress   string `json:"ipaddress,omitempty"`
	Dnn         string `json:"dnn,omitempty"`
	Slice       string `json:"slice,omitempty"`
	LSEID       int    `json:"lseid,omitempty"`
	RSEID       int    `json:"rseid,omitempty"`
	UpfName     string `json:"upfid,omitempty"`
	UpfAddr     string `json:"upfAddr,omitempty"`
	AmfId       string `json:"amfId,omitempty"`
	Guti        string `json:"guti,omitempty"`
	Tmsi        int32  `json:"tmsi,omitempty"`
	AmfNgapId   int64  `json:"amfngapId,omitempty"`
	RanNgapId   int64  `json:"ranngapId,omitempty"`
	AmfSubState string `json:"amfSubState,omitempty"` //RegisteredC, RegisteredI, DeRegistered, Deleted
	GnbId       string `json:"gnbid,omitempty"`
	TacId       string `json:"tacid,omitempty"`
	AmfIp       string `json:"amfIp,omitempty"`
	UeState     string `json:"ueState,omitempty"`
}

type AmfMsgType uint64

const (
	Amf_msg_type_invalid AmfMsgType = iota
	Amf_msg_type_test
)

func (t AmfMsgType) String() string {
	switch t {

	}
	return "error"
}

func InitialiseKafkaStream(config *factory.Configuration) error {

	brokerUrl := "sd-core-kafka-headless:9092"
	topicName := "sdcore-data-source-amf"

	if config.KafkaInfo.BrokerUrl != "" {
		logger.KafkaLog.Debugf("initialise kafka broker url [%v]", config.KafkaInfo.BrokerUrl)
		brokerUrl = config.KafkaInfo.BrokerUrl
	}

	if config.KafkaInfo.Topic != "" {
		logger.KafkaLog.Debugf("initialise kafka Topic [%v]", config.KafkaInfo.Topic)
		topicName = config.KafkaInfo.Topic
	}

	producer := kafka.Writer{
		Addr:     kafka.TCP(brokerUrl),
		Topic:    topicName,
		Balancer: &kafka.LeastBytes{},
	}

	StatWriter = Writer{
		kafkaWriter: producer,
	}
	return nil
}

func GetWriter() Writer {

	return StatWriter
}

func (writer Writer) SendMessage(message []byte) error {
	msg := kafka.Message{Value: message}
	err := writer.kafkaWriter.WriteMessages(context.Background(), msg)
	return err
}

func (writer Writer) PublishUeCtxtEvent(ctxt CoreSubscriber, op SubscriberOp) error {

	smKafkaEvt := MetricEvent{EventType: CSubscriberEvt,
		SubscriberData: CoreSubscriberData{Subscriber: ctxt, Operation: op}}
	if msg, err := json.Marshal(smKafkaEvt); err != nil {
		logger.KafkaLog.Errorf("publishing pdu sess event error [%v] ", err.Error())
		return err
	} else {
		logger.KafkaLog.Debugf("publishing pdu sess event[%s] ", msg)
		StatWriter.SendMessage(msg)
	}
	return nil
}

func PublishMsgEvent(msgType AmfMsgType) error {

	amfIp := os.Getenv("POD_IP")
	smKafkaMsgEvt := MetricEvent{EventType: CMsgTypeEvt, MsgType: CoreMsgType{MsgType: msgType.String(), SourceNfIp: amfIp}}
	if msg, err := json.Marshal(smKafkaMsgEvt); err != nil {
		return err
	} else {
		logger.KafkaLog.Debugf("publishing msg event[%s] ", msg)
		StatWriter.SendMessage(msg)
	}
	return nil
}

func (writer Writer) PublishNfStatusEvent(msgEvent MetricEvent) error {

	if msg, err := json.Marshal(msgEvent); err != nil {
		return err
	} else {
		logger.KafkaLog.Debugf("publishing nf status event[%s] ", msg)
		StatWriter.SendMessage(msg)
	}
	return nil
}
