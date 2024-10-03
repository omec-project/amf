// SPDX-FileCopyrightText: 2022-present Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0

package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/omec-project/amf/factory"
	"github.com/omec-project/amf/logger"
	mi "github.com/omec-project/util/metricinfo"
	"github.com/segmentio/kafka-go"
)

type Writer struct {
	kafkaWriter *kafka.Writer
}

var StatWriter Writer

func InitialiseKafkaStream(config *factory.Configuration) error {
	if !*factory.AmfConfig.Configuration.KafkaInfo.EnableKafka {
		logger.KafkaLog.Info("Kafka disabled")
		return nil
	}

	brokerUrl := "sd-core-kafka-headless:9092"
	topicName := "sdcore-data-source-amf"

	if config.KafkaInfo.BrokerUri != "" && config.KafkaInfo.BrokerPort != 0 {
		brokerUrl = fmt.Sprintf("%s:%d", config.KafkaInfo.BrokerUri, config.KafkaInfo.BrokerPort)
	}
	logger.KafkaLog.Debugf("initialise kafka broker url [%v]", brokerUrl)

	if config.KafkaInfo.Topic != "" {
		topicName = config.KafkaInfo.Topic
	}
	logger.KafkaLog.Debugf("initialise kafka Topic [%v]", config.KafkaInfo.Topic)

	producer := kafka.Writer{
		Addr:         kafka.TCP(brokerUrl),
		Topic:        topicName,
		Balancer:     &kafka.LeastBytes{},
		BatchTimeout: 10 * time.Millisecond,
	}

	StatWriter = Writer{
		kafkaWriter: &producer,
	}

	logger.KafkaLog.Debugf("initialising kafka stream with url[%v], topic[%v]", brokerUrl, topicName)
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

func (writer Writer) PublishUeCtxtEvent(ctxt mi.CoreSubscriber, op mi.SubscriberOp) error {
	smKafkaEvt := mi.MetricEvent{
		EventType:      mi.CSubscriberEvt,
		SubscriberData: mi.CoreSubscriberData{Subscriber: ctxt, Operation: op},
	}
	if msg, err := json.Marshal(smKafkaEvt); err != nil {
		logger.KafkaLog.Errorf("publishing ue context event error [%v] ", err.Error())
		return err
	} else {
		logger.KafkaLog.Debugf("publishing ue context event[%s] ", msg)
		if err := StatWriter.SendMessage(msg); err != nil {
			logger.KafkaLog.Errorf("could not publish ue context event, error [%v]", err.Error())
		}
	}
	return nil
}

func (writer Writer) PublishNfStatusEvent(msgEvent mi.MetricEvent) error {
	if msg, err := json.Marshal(msgEvent); err != nil {
		return err
	} else {
		logger.KafkaLog.Debugf("publishing nf status event[%s] ", msg)
		if err := StatWriter.SendMessage(msg); err != nil {
			logger.KafkaLog.Errorf("error publishing nf status event: %v", err)
		}
	}
	return nil
}
