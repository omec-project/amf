// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package lawfulintercept

import (
	"testing"

	amfctx "github.com/omec-project/amf/context"
	"github.com/omec-project/util/li/iri"
	"github.com/omec-project/util/li/store"
	"github.com/omec-project/util/li/types"
	"github.com/omec-project/util/li/x2x3"
)

const testExternalID = "subscriber@ext.example.org"

// TestAnExternalIdentifierIsTheRecordsGPSI: a GPSI held as "extid-" is a target identifier
// targetsOf matches, so it must also be what the record names. Left out, a subject tasked by
// its External Identifier alone produces records that name nobody — which the encoder refuses
// wherever the user identifiers are mandatory, so the warrant's matched events are lost.
func TestAnExternalIdentifierIsTheRecordsGPSI(t *testing.T) {
	id := amfctx.UeIdentity{Gpsi: "extid-" + testExternalID}
	if got, ok := gpsiChoice(id).(iri.NAI); !ok || got != iri.NAI(testExternalID) {
		t.Fatalf("gpsiChoice(%q) = %#v, want the NAI arm %q", id.Gpsi, gpsiChoice(id), testExternalID)
	}

	st := store.New()
	if !st.Activate(types.InterceptTask{
		XID:      testSubstitutionXID,
		Targets:  []types.TargetIdentifier{{Type: types.TargetGPSI, Value: testExternalID}},
		Products: []types.ProductType{types.ProductIRI},
		State:    types.TaskActive,
	}) {
		t.Fatal("activate")
	}
	snd := &captureSender{}
	active.Store(&subsystem{
		store: st, senderFor: func(string) sender { return snd },
		mdf2: testAgencyAddr,
		ids:  x2x3.NewIdentity(testNEID, amfInterceptionPoint),
	})
	t.Cleanup(func() { active.Store(nil) })

	// Known by its External Identifier only: AMFUEServiceAccept's user identifiers are
	// mandatory and must not be empty.
	ReportServiceAccept(&amfctx.AmfUe{Gpsi: "extid-" + testExternalID, Guti: testGUTI, Tmsi: 42})

	if records := decodeRecords(t, snd); len(records) != 1 {
		t.Fatalf("delivered %d records for a subject tasked by its External Identifier, want 1", len(records))
	}
}
