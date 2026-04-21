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
	"unicode/utf8"

	"github.com/omec-project/amf/logger"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// AmfStats captures AMF level stats
type AmfStats struct {
	ngapMsg           *prometheus.CounterVec
	gnbSessionProfile *prometheus.GaugeVec
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
