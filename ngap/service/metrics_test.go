// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"net"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

// gaugeValue reads a single unlabelled gauge back out of the default registry. The
// gathered types are used through their accessors only, so this needs no new module
// dependency.
func gaugeValue(t *testing.T, name string) (float64, bool) {
	t.Helper()

	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("Gather() = %v", err)
	}

	for _, family := range families {
		if family.GetName() != name {
			continue
		}

		for _, metric := range family.GetMetric() {
			return metric.GetGauge().GetValue(), true
		}
	}

	return 0, false
}

func assertAssociations(t *testing.T, when string, want float64) {
	t.Helper()

	got, ok := gaugeValue(t, "amf_ngap_associations")
	if !ok {
		t.Fatal("amf_ngap_associations is not in the registry: a gauge that is not registered never reaches /metrics")
	}

	if got != want {
		t.Errorf("amf_ngap_associations %s = %v, want %v", when, got, want)
	}
}

// The gauge exists so that an AMF holding no association is visible from outside the pod,
// where it otherwise looks exactly like an idle one. What it must track is the map the
// listener keeps, through both transitions.
func TestAssociationCountFollowsTheConnectionMap(t *testing.T) {
	t.Cleanup(func() {
		connections.Range(func(key, _ any) bool {
			connections.Delete(key)
			return true
		})
		reportAssociationCount()
	})

	reportAssociationCount()
	assertAssociations(t, "with no associations", 0)

	first, firstPeer := net.Pipe()
	defer first.Close()
	defer firstPeer.Close()

	second, secondPeer := net.Pipe()
	defer second.Close()
	defer secondPeer.Close()

	connections.Store(first, true)
	reportAssociationCount()
	assertAssociations(t, "with one association", 1)

	connections.Store(second, true)
	reportAssociationCount()
	assertAssociations(t, "with two associations", 2)

	// The direction that matters: an association going away has to move the gauge, which
	// is the whole reason for reporting it from the map rather than from AmfRanPool.
	connections.Delete(second)
	reportAssociationCount()
	assertAssociations(t, "after one association closed", 1)

	connections.Delete(first)
	reportAssociationCount()
	assertAssociations(t, "after every association closed", 0)
}

// A gauge whose Set is wired but whose registration was forgotten reads as absent rather
// than as wrong, which is the harder failure to notice.
func TestNgapLastMessageGaugeIsRegistered(t *testing.T) {
	if _, ok := gaugeValue(t, "amf_ngap_last_message_timestamp_seconds"); !ok {
		t.Error("amf_ngap_last_message_timestamp_seconds is not in the registry")
	}
}
