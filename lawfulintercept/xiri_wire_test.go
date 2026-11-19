// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package lawfulintercept

import (
	"encoding/asn1"
	"testing"
)

// The xIRI records this package delivers are read back from the wire with the standard
// library, as the bytes a mediation function receives.
//
// encoding/asn1 has no CHOICE, and unmarshalling into iri.XIRIPayload does not fail on
// that: it returns no error and leaves Event nil. So these helpers do not decode into the
// iri types at all. They unwrap the payload, and read a record's members into a map keyed by
// context tag — tags are unique within a SEQUENCE — for a test to read the one it asserts on.

// The XIRIEvent alternatives these tests read, from TS 33.128's XIRIEvent CHOICE.
const (
	eventRANHandoverCommand = 113 // aMFRANHandoverCommand
	eventRANHandoverRequest = 114 // aMFRANHandoverRequest
	eventUEPolicyTransfer   = 146 // aMFUEPolicyTransfer
	eventUEServiceAccept    = 147 // aMFUEServiceAccept
)

// wireRecord is one delivered record: its XIRIEvent alternative and its members.
type wireRecord struct {
	event   int
	members map[int]asn1.RawValue
}

// elements parses contents as a run of complete elements.
func elements(t *testing.T, what string, contents []byte) []asn1.RawValue {
	t.Helper()
	var out []asn1.RawValue
	for len(contents) > 0 {
		var rv asn1.RawValue
		rest, err := asn1.Unmarshal(contents, &rv)
		if err != nil {
			t.Fatalf("%s: %v", what, err)
		}
		out = append(out, rv)
		contents = rest
	}
	return out
}

// only returns the one element a constructed member holds — the alternative inside an
// EXPLICIT-tagged CHOICE member, or the single element of a one-member wrapper.
func only(t *testing.T, what string, rv asn1.RawValue) asn1.RawValue {
	t.Helper()
	inner := elements(t, what, rv.Bytes)
	if len(inner) != 1 {
		t.Fatalf("%s holds %d elements, want 1", what, len(inner))
	}
	return inner[0]
}

// decodeRecords reads every captured PDU's payload as an XIRIPayload: SEQUENCE {
// xIRIPayloadOID [1], event [2] EXPLICIT XIRIEvent }.
func decodeRecords(t *testing.T, snd *captureSender) []wireRecord {
	t.Helper()
	out := make([]wireRecord, 0, len(snd.pdus))
	for _, p := range snd.pdus {
		var payload asn1.RawValue
		if rest, err := asn1.Unmarshal(p.Payload, &payload); err != nil || len(rest) != 0 {
			t.Fatalf("xIRI payload is not one element: %v (%d trailing bytes)", err, len(rest))
		}
		top := elements(t, "XIRIPayload", payload.Bytes)
		if len(top) != 2 || top[0].Tag != 1 || top[1].Tag != 2 {
			t.Fatalf("XIRIPayload is not { [1] OID, [2] event }: % x", p.Payload)
		}
		ev := only(t, "event [2]", top[1])
		if ev.Class != asn1.ClassContextSpecific || !ev.IsCompound {
			t.Fatalf("event is not a constructed XIRIEvent alternative: % x", ev.FullBytes)
		}
		rec := wireRecord{event: ev.Tag, members: map[int]asn1.RawValue{}}
		for _, m := range elements(t, "record", ev.Bytes) {
			if _, dup := rec.members[m.Tag]; dup {
				t.Fatalf("record [%d] carries member [%d] twice", ev.Tag, m.Tag)
			}
			rec.members[m.Tag] = m
		}
		out = append(out, rec)
	}
	return out
}

// member returns the record's member [tag], failing if it is absent.
func (r wireRecord) member(t *testing.T, tag int) asn1.RawValue {
	t.Helper()
	m, ok := r.members[tag]
	if !ok {
		t.Fatalf("record [%d] has no member [%d]", r.event, tag)
	}
	return m
}

// integer reads an IMPLICIT-tagged INTEGER or ENUMERATED element.
func integer(t *testing.T, rv asn1.RawValue) int64 {
	t.Helper()
	if rv.IsCompound {
		t.Fatalf("[%d] is constructed, not an INTEGER", rv.Tag)
	}
	var n int64
	if _, err := asn1.Unmarshal(append([]byte{asn1.TagInteger, byte(len(rv.Bytes))}, rv.Bytes...), &n); err != nil {
		t.Fatalf("[%d] is not an INTEGER: %v", rv.Tag, err)
	}
	return n
}

// supiOf reads the IMSI from a userIdentifiers member: [1] fiveGSSubscriberIDs { [1]
// fiveGSSubscriberID SEQUENCE OF { sUPI [1] { iMSI [1] … }, … } }.
func supiOf(t *testing.T, userIdentifiers asn1.RawValue) string {
	t.Helper()
	ids := only(t, "userIdentifiers", userIdentifiers)
	if ids.Tag != 1 {
		t.Fatalf("userIdentifiers holds [%d], want fiveGSSubscriberIDs [1]", ids.Tag)
	}
	list := only(t, "fiveGSSubscriberIDs", ids)
	for _, arm := range elements(t, "fiveGSSubscriberID", list.Bytes) {
		if arm.Tag != 1 {
			continue
		}
		leaf := only(t, "sUPI [1]", arm)
		if leaf.Tag != 1 {
			t.Fatalf("sUPI holds [%d], want iMSI [1]", leaf.Tag)
		}
		return string(leaf.Bytes)
	}
	t.Fatalf("no sUPI [1] in userIdentifiers % x", userIdentifiers.FullBytes)
	return ""
}
