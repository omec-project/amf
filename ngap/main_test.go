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
// t.Cleanup. Both writes are unsynchronised against a goroutine the tests leave running:
// Dispatch hands an NG Setup to `go DispatchNgapMsg` because it has no AmfUe, the test's
// only synchronisation is waitForConnData, and SendNGSetupResponse releases that while the
// flag read in HandleNGSetupRequest is still some lines ahead of it. So the test resumes --
// running its cleanup, or letting the next test set the flag -- with nothing ordering it
// against that read.
//
// No rate is quoted here on purpose: how often the detector catches it depends on the
// machine, and the window above does not.
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
