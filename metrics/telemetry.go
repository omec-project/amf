// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

/*
 * AMF Statistics exposing to promethus
 *
 */

package metrics

import (
	"encoding/hex"
	"net/http"
	"time"
	"unicode/utf8"

	"github.com/omec-project/amf/logger"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// AmfStats captures AMF level stats
type AmfStats struct {
	ngapMsg           *prometheus.CounterVec
	gnbSessionProfile *prometheus.GaugeVec
	dbWriteDropped    prometheus.Counter
	ngapAssociations  prometheus.Gauge
	ngapLastMessage   prometheus.Gauge
}

var amfStats *AmfStats

func initAmfStats() *AmfStats {
	return &AmfStats{
		ngapMsg: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "ngap_messages_total",
			Help: "ngap interface counters",
		}, []string{"amf_id", "msg_type", "direction", "result", "reason"}),

		gnbSessionProfile: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gnb_session_profile",
			Help: "gNB session Profile",
		}, []string{"id", "ip", "state", "tac"}),

		dbWriteDropped: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "amf_db_write_dropped_total",
			Help: "Total number of UE context DB writes dropped due to a full write queue.",
		}),

		ngapAssociations: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "amf_ngap_associations",
			Help: "Number of SCTP associations this AMF terminates itself. The listener is " +
				"started in either mode, so in a deployment whose gNBs connect to the SCTP " +
				"load balancer this AMF accepts none and the value stays zero: it counts " +
				"what this process terminates, not what it serves.",
		}),

		ngapLastMessage: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "amf_ngap_last_message_timestamp_seconds",
			Help: "Unix time of the last NGAP message this AMF handled, or zero if it has " +
				"handled none. Exposed as a timestamp rather than an age so that the staleness " +
				"is computed at query time.",
		}),
	}
}

func (ps *AmfStats) register() error {
	prometheus.Unregister(ps.ngapMsg)

	if err := prometheus.Register(ps.ngapMsg); err != nil {
		return err
	}
	if err := prometheus.Register(ps.gnbSessionProfile); err != nil {
		return err
	}
	prometheus.Unregister(ps.dbWriteDropped)
	if err := prometheus.Register(ps.dbWriteDropped); err != nil {
		return err
	}
	prometheus.Unregister(ps.ngapAssociations)
	if err := prometheus.Register(ps.ngapAssociations); err != nil {
		return err
	}
	prometheus.Unregister(ps.ngapLastMessage)
	if err := prometheus.Register(ps.ngapLastMessage); err != nil {
		return err
	}
	return nil
}

func init() {
	amfStats = initAmfStats()

	if err := amfStats.register(); err != nil {
		logger.AppLog.Errorln("AMF Stats register failed", err)
	}
}

// InitMetrics initialises AMF stats
func InitMetrics() {
	http.Handle("/metrics", promhttp.Handler())
	if err := http.ListenAndServe(":9089", nil); err != nil {
		logger.InitLog.Errorf("could not open metrics port: %v", err)
	}
}

// IncrementNgapMsgStats increments message level stats
func IncrementNgapMsgStats(amfID, msgType, direction, result, reason string) {
	amfID = sanitizeLabelValue(amfID)
	msgType = sanitizeLabelValue(msgType)
	direction = sanitizeLabelValue(direction)
	result = sanitizeLabelValue(result)
	reason = sanitizeLabelValue(reason)
	amfStats.ngapMsg.WithLabelValues(amfID, msgType, direction, result, reason).Inc()
}

// sanitizeLabelValue ensures a string is valid UTF-8 for use as Prometheus label value
func sanitizeLabelValue(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	// If not valid UTF-8, convert to hex representation with prefix
	// to make it clear this is sanitized data
	return "hex:" + hex.EncodeToString([]byte(s))
}

// SetGnbSessProfileStats maintains Session profile info
func SetGnbSessProfileStats(id, ip, state, tac string, count uint64) {
	id = sanitizeLabelValue(id)
	ip = sanitizeLabelValue(ip)
	state = sanitizeLabelValue(state)
	tac = sanitizeLabelValue(tac)
	amfStats.gnbSessionProfile.WithLabelValues(id, ip, state, tac).Set(float64(count))
}

// SetNgapAssociations records how many SCTP associations the AMF currently terminates.
// An AMF that has lost every association is indistinguishable from an idle one in its
// logs, which is what this makes visible from outside the pod.
func SetNgapAssociations(count int) {
	amfStats.ngapAssociations.Set(float64(count))
}

// SetNgapLastMessage records that an NGAP message was handled at t. Paired with the
// association count, a timestamp that stops advancing while the count is non-zero is a
// different fault from the count going to zero, and the two are worth telling apart.
func SetNgapLastMessage(t time.Time) {
	amfStats.ngapLastMessage.Set(float64(t.Unix()))
}

// IncrementDbWriteDropped increments the counter of UE context writes dropped
// because the async write queue was full.
func IncrementDbWriteDropped() {
	amfStats.dbWriteDropped.Inc()
}
