// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package message

import (
	"testing"

	"github.com/omec-project/amf/context"
	"github.com/omec-project/amf/factory"
	"github.com/omec-project/nas/v2"
	"github.com/omec-project/openapi/v2/models"
	"github.com/omec-project/util/fsm"
)

// TestRegistrationAcceptCarriesGUTIWheneverTheUEHasOne pins the fact the split
// of Lawful Interception registration reporting rests on.
//
// Registration records are emitted from two taps: one reports a mobility update,
// the other reports what completes a registration. Which tap a periodic update
// reaches is not a property of the procedure — it is a property of whether the
// registration accept carried a 5G-GUTI, because TS 24.501 clause 5.5.1.3.4 makes
// the UE answer with Registration Complete only then.
//
// So if this ever becomes conditional, periodic updates stop reaching the second
// tap and are reported by neither: a silent omission, indistinguishable to an
// agency from a subject who did nothing. Nothing in the LI code would fail; the
// reporting would simply move.
func TestRegistrationAcceptCarriesGUTIWheneverTheUEHasOne(t *testing.T) {
	ue := &context.AmfUe{Guti: "20893cafe0000001"}
	ue.PlmnId = models.PlmnId{Mcc: "208", Mnc: "93"}
	// The registration result reads both access states, which AmfUe.init would
	// normally have populated.
	ue.State = map[models.AccessType]*fsm.State{
		models.ACCESSTYPE__3_GPP_ACCESS:    fsm.NewState(context.Deregistered),
		models.ACCESSTYPE_NON_3_GPP_ACCESS: fsm.NewState(context.Deregistered),
	}

	// BuildRegistrationAccept consults the network-feature-support config; these
	// tests do not otherwise build one.
	if factory.AmfConfig.Configuration == nil {
		factory.AmfConfig.Configuration = &factory.Configuration{}
		t.Cleanup(func() { factory.AmfConfig.Configuration = nil })
	}

	nasPdu, err := BuildRegistrationAccept(ue, models.ACCESSTYPE__3_GPP_ACCESS, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("BuildRegistrationAccept: %v", err)
	}
	if len(nasPdu) == 0 {
		t.Fatal("BuildRegistrationAccept produced no PDU")
	}

	// **Decoded, not scanned.** A search of the whole encoded PDU for the IEI byte (0x77)
	// passes if it occurs anywhere — including inside the value of some other information
	// element, or inside the GUTI's own digits — so it can pass on a PDU carrying no 5G-GUTI
	// IE at all.
	m := nas.NewMessage()
	if err := m.PlainNasDecode(&nasPdu); err != nil {
		t.Fatalf("decoding the registration accept: %v", err)
	}

	// The embedded GmmMessage is checked on its own because the promoted selectors below
	// dereference it.
	if m.GmmMessage == nil {
		t.Fatal("the built PDU carries no GMM message")
	}

	if m.RegistrationAccept == nil {
		t.Fatal("the built PDU is not a registration accept")
	}

	if m.RegistrationAccept.GUTI5G == nil {
		t.Error("the registration accept carries no 5G-GUTI IE for a UE that has a GUTI — " +
			"the UE will send no Registration Complete, so a periodic registration " +
			"update reaches neither reporting tap and is never reported")
	}
}
