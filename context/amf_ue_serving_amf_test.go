// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
//
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"strings"
	"sync"
	"testing"

	"github.com/bytedance/sonic"
)

// init() points every UE at the process-wide AMF context. Persisting that pointer
// wrote the whole singleton into each UE document, and restoring a UE decoded back
// into the live one -- so a restore overwrote the running AMF's state, and two at
// once were "fatal error: concurrent map writes" with no recovery.
func TestAStoredContextDoesNotCarryTheAmf(t *testing.T) {
	ue := &AmfUe{}
	ue.init()
	ue.Supi = "imsi-208930100007487"

	stored, err := sonic.Marshal(ue)
	if err != nil {
		t.Fatalf("storing a context must not fail: %v", err)
	}

	if strings.Contains(string(stored), "servingAMF") {
		t.Error("the stored context carries the AMF singleton; restoring it will write the running AMF's own state")
	}
}

// A document written before the field was dropped still carries servingAMF. Restoring
// it must leave the running AMF alone rather than decode into it.
func TestRestoringAnOldDocumentLeavesTheRunningAmfAlone(t *testing.T) {
	self := AMF_Self()
	self.Name = "the running AMF"

	ue := &AmfUe{}
	ue.init()

	old := `{"supi":"imsi-208930100007487","servingAMF":{"Name":"a stored snapshot"}}`
	if err := sonic.Unmarshal([]byte(old), ue); err != nil {
		t.Fatalf("restoring a context must not fail: %v", err)
	}

	if self.Name != "the running AMF" {
		t.Errorf("restoring a UE overwrote the running AMF: Name = %q", self.Name)
	}
}

// Restores run on the NGAP reader goroutines, so several land at once. Under -race
// this fails if the decoder can still reach a shared map.
func TestConcurrentRestoresDoNotShareAMap(t *testing.T) {
	const restores = 400

	doc := []byte(`{"supi":"imsi-208930100007487","servingAMF":{"Name":"a stored snapshot",` +
		`"PlmnSupportList":[{"plmnId":{"mcc":"208","mnc":"93"}}]}}`)

	var wg sync.WaitGroup

	for range 8 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for range restores {
				ue := &AmfUe{}
				ue.init()

				if err := sonic.Unmarshal(doc, ue); err != nil {
					t.Errorf("restoring a context must not fail: %v", err)
					return
				}
			}
		}()
	}

	wg.Wait()
}
