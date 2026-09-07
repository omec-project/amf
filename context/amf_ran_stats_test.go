// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/omec-project/openapi/v2/models"
	"github.com/prometheus/client_golang/prometheus"
)

// gnbSessionProfileSeries returns the gnb_session_profile samples as "labels=value"
// lines, sorted, so a test can state the whole family in one comparison. The gathered
// types are used through their accessors only, so this needs no new module dependency.
func gnbSessionProfileSeries(t *testing.T) string {
	t.Helper()

	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("Gather() = %v", err)
	}

	lines := []string{}

	for _, family := range families {
		if family.GetName() != "gnb_session_profile" {
			continue
		}

		for _, metric := range family.GetMetric() {
			labels := []string{}
			for _, label := range metric.GetLabel() {
				labels = append(labels, label.GetName()+"="+label.GetValue())
			}
			sort.Strings(labels)
			lines = append(lines, fmt.Sprintf("{%s} %g", strings.Join(labels, ","), metric.GetGauge().GetValue()))
		}
	}

	sort.Strings(lines)

	return strings.Join(lines, "\n")
}

// The gauge carries the RAN's state as a label, so a transition has to write both series
// or the one being left keeps the value it had. This test owns the whole metric family in
// this package: nothing else here writes it.
func TestSetRanStatsWritesBothStatesOnEveryTransition(t *testing.T) {
	ran := &AmfRan{
		Name:            "gnb-metric-test",
		GnbIp:           "10.10.10.10",
		SupportedTAList: []SupportedTAI{{Tai: models.Tai{Tac: "000001"}}},
	}

	ran.SetRanStats(RanConnected)

	want := "{id=gnb-metric-test,ip=10.10.10.10,state=Connected,tac=000001} 1\n" +
		"{id=gnb-metric-test,ip=10.10.10.10,state=Disconnected,tac=000001} 0"
	if got := gnbSessionProfileSeries(t); got != want {
		t.Errorf("gnb_session_profile after connecting:\n%s\nwant:\n%s", got, want)
	}

	ran.SetRanStats(RanDisconnected)

	// The half that was wrong: Connected stayed at 1 for the life of the process, so a
	// dashboard summing that series could not see the gNB go away.
	want = "{id=gnb-metric-test,ip=10.10.10.10,state=Connected,tac=000001} 0\n" +
		"{id=gnb-metric-test,ip=10.10.10.10,state=Disconnected,tac=000001} 1"
	if got := gnbSessionProfileSeries(t); got != want {
		t.Errorf("gnb_session_profile after disconnecting:\n%s\nwant:\n%s", got, want)
	}
}

// A RAN whose NGSetup carried no supported TA list has no series to write, and must not
// panic on the way to finding that out.
func TestSetRanStatsWithNoSupportedTAList(t *testing.T) {
	ran := &AmfRan{Name: "gnb-no-tai", GnbIp: "10.10.10.11"}

	ran.SetRanStats(RanConnected)
	ran.SetRanStats(RanDisconnected)
}
