// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package ngap

import (
	"github.com/omec-project/amf/lawfulintercept"
	"github.com/omec-project/util/li/iri"
)

// NGAP and TS 33.128 describe the same five Cause groups and number them differently: NGAP
// from zero, TS 33.128 from one. So a cause read off the wire is *not* the value a handover
// record carries. Emitted as though it were, every cause reads one below the value TS 33.128
// defines for it, and NGAP's `unspecified(0)` becomes a zero that the record's own enumeration
// does not include.
//
// **That failure is the worst-behaved kind available on this interface.** A misnumbered cause
// is a plausible member of the right enumeration, so it decodes, validates and is stored: a
// mediation function cannot tell it from the cause that actually occurred, and neither can this
// element. Nothing on the wire or in either party's records distinguishes "the network gave
// this reason" from "the network gave the reason one along".
//
// **The correspondence is `NGAP + 1` wherever one exists, and the interesting part is where one
// does not.** Every value TS 33.128 defines sits one above the NGAP value for the same cause —
// all 48 of the radio-network pairs, and every pair in the other four groups. What arithmetic
// cannot supply is the list below: NGAP values that TS 33.128 has no value for at all, either
// because the specification omitted one (the radio-network enumeration runs 1..52 with 28
// absent) or because NGAP has been extended since the release this element's module is from.
// Those are the values a shift would carry into a number the record's enumeration does not
// define, and they are the reason this is a function rather than an expression.
//
// The names in the comments are what makes the arithmetic checkable:
// TestTheCauseMappingIsTotalOverNGAP derives the correspondence from the two definitions *by
// name* — the TS 33.128 module carried under `li/` in the pinned util module, and the NGAP
// constants — and asserts this function agrees with it. So the implementation is arithmetic
// and a list, and its test's source of truth is neither.

// unrepresentableRadioNetwork are the NGAP CauseRadioNetwork values TS 33.128 has no value for.
//
//	27 — unkownQosFlowID. The value TS 33.128 skipped: its enumeration has no 28, and every
//	     value above it stays aligned at NGAP+1, so the omission is this one cause and not a
//	     renumbering.
//	52 — redcapUeNotSupported
//	53 — unknownMBSSessionID
//	54 — indicatedMBSSessionAreaInformationNotServedByTheGNB
//	55 — inconsistentSliceInfoForTheSession
//	56 — misalignedAssociationForMulticastUnicast
//	57 — eredcapUeNotSupported
//
// The last six are NGAP causes introduced after the TS 33.128 release this element targets. A
// later release that adds them will show up as this list being wrong, in the test rather than
// on the wire.
var unrepresentableRadioNetwork = map[int64]bool{
	27: true, 52: true, 53: true, 54: true, 55: true, 56: true, 57: true,
}

// unrepresentableNas are the NGAP CauseNas values TS 33.128 has no value for.
//
//	4 — uENotInPLMNServingArea
//	5 — mobileIABNotAuthorized
//
// TS 33.128's CauseNas has four values where NGAP has six.
var unrepresentableNas = map[int64]bool{4: true, 5: true}

// highestNGAPValue is the last value each group's NGAP enumeration defines, in the pinned
// module.
//
// Needed because the lists above enumerate the values TS 33.128 *lacks an equivalent for*, and a
// value NGAP does not define at all is in neither list — so without a bound it would be shifted
// like any other and produce a number above the group's TS 33.128 maximum. The bounds added to
// `iri.enumConstraints` then refuse it, and the record is not built at all: a delivery loss where
// an unrepresentable cause should cost only precision.
//
// **This is reachable, not merely defensive.** NGAP's Cause groups are extensible ENUMERATEDs, so
// a gNB built to a later release can send a value above what this module knows, and the decoder
// carries it up rather than rejecting it.
//
// Asserted against the module in TestTheCauseMappingIsTotalOverNGAP, so bumping the NGAP pin is
// what surfaces a group that has grown — the same treatment as the lists above.
var highestNGAPValue = map[lawfulintercept.HandoverCauseGroup]int64{
	lawfulintercept.CauseGroupRadioNetwork: 57,
	lawfulintercept.CauseGroupTransport:    1,
	lawfulintercept.CauseGroupNAS:          5,
	lawfulintercept.CauseGroupProtocol:     6,
	lawfulintercept.CauseGroupMisc:         5,
}

// unspecifiedFor is the value each group gives its own "unspecified" cause — what a cause this
// record cannot express is delivered as.
//
// Chosen because the field is mandatory, so the alternatives are a less precise cause or no
// record at all, and a handover that went unreported is a larger gap than one whose reason
// reads as unspecified. The substitution is reported by the caller, because the value it
// substitutes is itself a legitimate reading: without a report, the agency's record says the
// network gave no reason when it gave one this element could not carry.
func unspecifiedFor(group lawfulintercept.HandoverCauseGroup) int64 {
	switch group {
	case lawfulintercept.CauseGroupTransport:
		return int64(iri.CauseTransportUnspecified)
	case lawfulintercept.CauseGroupNAS:
		return int64(iri.CauseNasUnspecified)
	case lawfulintercept.CauseGroupProtocol:
		return int64(iri.CauseProtocolUnspecified)
	case lawfulintercept.CauseGroupMisc:
		return int64(iri.CauseMiscUnspecified)
	case lawfulintercept.CauseGroupRadioNetwork:
		return int64(iri.CauseRadioNetworkUnspecified)
	}

	return int64(iri.CauseRadioNetworkUnspecified)
}

// liCauseValue returns the TS 33.128 value for an NGAP cause, and whether it had to substitute.
//
// A substitution is not a failure: the record is built either way, and the second return value
// is what the caller reports so that the substituted `unspecified` is distinguishable from a
// cause the network genuinely gave as unspecified.
func liCauseValue(group lawfulintercept.HandoverCauseGroup, ngapValue int64) (value int64, substituted bool) {
	unrepresentable := false
	switch group {
	case lawfulintercept.CauseGroupRadioNetwork:
		unrepresentable = unrepresentableRadioNetwork[ngapValue]
	case lawfulintercept.CauseGroupNAS:
		unrepresentable = unrepresentableNas[ngapValue]
	case lawfulintercept.CauseGroupTransport, lawfulintercept.CauseGroupProtocol,
		lawfulintercept.CauseGroupMisc:
		// Every value of these three has a TS 33.128 equivalent.
	}
	if unrepresentable {
		return unspecifiedFor(group), true
	}

	// Outside what the pinned NGAP module defines for this group: either a value from a later
	// NGAP release, or a caller's mistake. +1 would produce a number outside the group's TS
	// 33.128 range, which the record's constraint check refuses — costing the record rather than
	// its precision. Treated as unrepresentable instead, as every cause TS 33.128 cannot express is.
	//
	// A group absent from the table has a highest of zero, so only its first value maps. That is
	// the safe direction: a group added to lawfulintercept without a bound here substitutes
	// rather than emits values nothing has checked.
	if ngapValue < 0 || ngapValue > highestNGAPValue[group] {
		return unspecifiedFor(group), true
	}

	return ngapValue + 1, false
}

// NGAP and TS 33.128 number HandoverType differently too: TS 33.128 numbers the four values
// 1..4, NGAP numbers them 0..3.
//
// **A cast here is worse than a misnumbered cause.** An intra-5GS handover — the ordinary case —
// would be emitted as `handoverType: 0`, which the arm's enumeration does not define at all. A
// misnumbered cause is at least a plausible member of the right enumeration; this is a record a
// conformant mediation function **cannot decode**, and `handoverType` is mandatory in both
// records that carry it. An ASN.1 decoder refuses it:
//
//	XIRIPayload.event.aMFRANHandoverCommand.handoverType:
//	  Expected enumeration value 1, 2, 3 or 4, but got 0
//
// The record is then discarded on receipt, with this element believing it had delivered and no
// fault raised at either end.
//
// The constraint check treats zero as absence — correctly for an optional member, whose unset
// value reads as zero in Go — so it refuses a mandatory zero only because `iri` records per type
// whether zero is a legitimate reading.
var unrepresentableHandoverType = map[int64]bool{}

// highestNGAPHandoverType is the last value NGAP defines. Both enumerations have four values, so
// there is nothing to substitute for in this release — but the bound still matters: NGAP's is an
// extensible ENUMERATED, and a value it gains later would otherwise be shifted into a number
// TS 33.128 does not define, which is the failure above in a new dress.
const highestNGAPHandoverType = 3

// liHandoverType returns the TS 33.128 value for an NGAP handover type, and whether it had to
// substitute.
//
// The substitution is `intra5GS`, which is not an "unspecified" — the enumeration has none — so
// it is the least misleading of four wrong answers rather than a neutral one. It is reported for
// that reason, like the cause substitution.
func liHandoverType(ngapValue int64) (value int64, substituted bool) {
	if ngapValue < 0 || ngapValue > highestNGAPHandoverType || unrepresentableHandoverType[ngapValue] {
		return int64(iri.HandoverIntra5GS), true
	}

	return ngapValue + 1, false
}
