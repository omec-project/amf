// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package ngap

import (
	"os"
	"testing"

	"github.com/omec-project/amf/factory"
)

// TestMain disables Kafka once for the whole test binary.
//
// It used to be done per test by a helper that wrote
// factory.AmfConfig.Configuration.KafkaInfo.EnableKafka on entry and restored it from
// t.Cleanup. Both of those writes race with HandleNGSetupRequest reading the same flag
// through SendNGSetupResponse, on a goroutine an earlier test left running:
// go test -race -count=2 ./ngap/... reports it in 12 runs out of 20, and eight of those
// twelve name the cleanup rather than the set.
//
// No test in this package wants Kafka on, so setting it once removes both writes rather
// than guarding the read. TestMain runs after every package init, including the one in
// ngap_test that loads the config, so this is the last word on the flag.
func TestMain(m *testing.M) {
	if factory.AmfConfig.Configuration == nil {
		factory.AmfConfig.Configuration = &factory.Configuration{}
	}
	disabled := false
	factory.AmfConfig.Configuration.KafkaInfo.EnableKafka = &disabled

	os.Exit(m.Run())
}
