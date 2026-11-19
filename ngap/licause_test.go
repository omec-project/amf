// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package ngap

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/omec-project/amf/lawfulintercept"
	"github.com/omec-project/ngap/v2/aper"
	"github.com/omec-project/ngap/v2/ngapType"
	"github.com/omec-project/util/li/iri"
)

// The two definitions this mapping sits between, read rather than restated.
//
// **This is what makes the arithmetic checkable.** liCauseValue is `NGAP + 1` plus a list of
// values TS 33.128 has none for, and a test that asserted "+1 or in the list" would be the same
// statement twice — it would pass against a wrong list as readily as a right one. So the tests
// below derive the correspondence from the *names* in the two definitions and assert the
// implementation agrees. The implementation's source of truth is arithmetic; the test's is the
// specifications.

// causeGroups pairs each group with the ASN.1 type its TS 33.128 values live under, the file its
// NGAP constants live in (the same name), and a builder for the `Cause` CHOICE arm that carries
// it on the wire.
//
// The builder is what lets the assertion below start where the element does — at a decoded NGAP
// message — rather than at the mapping function. Group selection and value mapping are then one
// assertion, which matters because they fail independently: a radio-network cause reported under
// the misc arm is as wrong as one reported with the wrong number.
var causeGroups = []struct {
	group    lawfulintercept.HandoverCauseGroup
	asn1Type string
	cause    func(int64) *ngapType.Cause
}{
	{lawfulintercept.CauseGroupRadioNetwork, "CauseRadioNetwork", func(v int64) *ngapType.Cause {
		return &ngapType.Cause{
			Present:      ngapType.CausePresentRadioNetwork,
			RadioNetwork: &ngapType.CauseRadioNetwork{Value: aper.Enumerated(v)},
		}
	}},
	{lawfulintercept.CauseGroupTransport, "CauseTransport", func(v int64) *ngapType.Cause {
		return &ngapType.Cause{
			Present:   ngapType.CausePresentTransport,
			Transport: &ngapType.CauseTransport{Value: aper.Enumerated(v)},
		}
	}},
	{lawfulintercept.CauseGroupNAS, "CauseNas", func(v int64) *ngapType.Cause {
		return &ngapType.Cause{
			Present: ngapType.CausePresentNas,
			Nas:     &ngapType.CauseNas{Value: aper.Enumerated(v)},
		}
	}},
	{lawfulintercept.CauseGroupProtocol, "CauseProtocol", func(v int64) *ngapType.Cause {
		return &ngapType.Cause{
			Present:  ngapType.CausePresentProtocol,
			Protocol: &ngapType.CauseProtocol{Value: aper.Enumerated(v)},
		}
	}},
	{lawfulintercept.CauseGroupMisc, "CauseMisc", func(v int64) *ngapType.Cause {
		return &ngapType.Cause{
			Present: ngapType.CausePresentMisc,
			Misc:    &ngapType.CauseMisc{Value: aper.Enumerated(v)},
		}
	}},
}

// ts33128Module locates the TS 33.128 ASN.1 module under `li/` in the pinned util module.
//
// Read from the module cache rather than copied here, so the assertion follows the pin: bumping
// `li` to a release built against a later TS 33.128 changes what these tests check against,
// which is the point. A second copy in this repository would be a transcription with nothing
// keeping it honest.
func ts33128Module(t *testing.T) string {
	t.Helper()

	// **Fatal, not skipped.** These are the assertions that keep the cause mapping honest —
	// every handover and NGAP cause an agency receives depends on them — and a skip is a green
	// run that checked nothing. A hermetic build with no module cache would turn the strongest
	// tests in this package into no-ops without anybody noticing, which is the same shape as a
	// test that asserts against a stub.
	//
	// If this cannot find the module, the environment cannot check the mapping, and saying so
	// loudly is the only useful answer.
	out, err := exec.CommandContext(t.Context(),
		"go", "list", "-m", "-f", "{{.Dir}}", "github.com/omec-project/util").Output()
	if err != nil {
		t.Fatalf("cannot locate the util module (%v); this test checks the cause mapping against "+
			"the TS 33.128 module that module carries, and without it nothing here is checked", err)
	}

	dir := strings.TrimSpace(string(out))

	// **The pin, asserted rather than assumed.** Under the workspace `go list -m` answers with
	// the local ./util checkout, not the version go.mod pins — so these tests can check the
	// mapping against a module the shipped binary does not contain. Harmless while the two
	// agree, load-bearing the moment they diverge, which is exactly when a cause mapping would
	// silently stop corresponding.
	if want := pinnedUtilVersion(t); want != "" {
		switch {
		case strings.Contains(dir, "/pkg/mod/"):
			// Resolved from the module cache, so the pin is what was checked and can be
			// asserted outright. This is the shape a container build and CI see.
			if !strings.Contains(dir, want) {
				t.Fatalf("checking the cause mapping against %s while go.mod pins %s: these "+
					"assertions would then pass against a module the shipped binary does not "+
					"contain, which is exactly when a cause mapping stops corresponding", dir, want)
			}
		default:
			// A workspace or replace directory. It cannot be compared to a version string, so
			// say which tree was checked rather than implying the pin was.
			t.Logf("cause mapping checked against the local tree %s, not the pinned %s; run with "+
				"GOWORK=off to assert the pin", dir, want)
		}
	}

	path := filepath.Join(dir, "li", "iri", "testdata", "asn1", "TS33128Payloads.asn")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the util module at %s does not carry %s (%v)", dir, path, err)
	}

	return path
}

// normaliseCauseName reduces an identifier to what the two specifications agree on: letters and
// digits, lowercased.
//
// The two spell the same cause differently — NGAP writes `ReleaseDueToNgranGeneratedReason`
// where TS 33.128 writes `releaseDueToNGRANGeneratedReason` — so a comparison that respected
// case would report 29 of 58 radio-network causes as absent when only seven are.
func normaliseCauseName(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}

	return b.String()
}

// spellingAliases are the causes the two specifications name differently enough that
// normalisation does not join them.
//
// Every one is a typographical difference in one document or the other, and each is listed
// individually because the alternative — a fuzzy match — would silently pair causes that are
// not the same cause, which is the failure this whole mapping exists to prevent. The key is the
// NGAP name normalised; the value is the TS 33.128 name normalised.
var spellingAliases = map[string]string{
	// TS 33.128 has "Voicee".
	"imsvoiceepsfallbackorratfallbacktriggered": "imsvoiceeepsfallbackorratfallbacktriggered",
	// TS 33.128 has "Protectio".
	"upintegrityprotectionnotpossible": "upintegrityprotectionotpossible",
	// NGAP has NPN (non-public network); TS 33.128 has NPM. One of the two is a typo and the
	// cause is the same in both — it sits at the same position in both enumerations.
	"npnaccessdenied": "npmaccessdenied",
}

// ts33128Values parses one group's values out of the module, keyed by normalised name.
func ts33128Values(t *testing.T, modulePath, asn1Type string) map[string]int64 {
	t.Helper()

	src, err := os.ReadFile(modulePath)
	if err != nil {
		t.Fatalf("reading %s: %v", modulePath, err)
	}
	block := regexp.MustCompile(`(?ms)^` + asn1Type + ` ::= ENUMERATED\s*\{(.*?)^\}`).FindSubmatch(src)
	if block == nil {
		t.Fatalf("%s is not defined in %s", asn1Type, modulePath)
	}

	out := map[string]int64{}
	for _, m := range regexp.MustCompile(`([A-Za-z][\w-]*)\((\d+)\)`).FindAllStringSubmatch(string(block[1]), -1) {
		v, err := strconv.ParseInt(m[2], 10, 64)
		if err != nil {
			t.Fatalf("%s: %q is not a number", asn1Type, m[2])
		}
		out[normaliseCauseName(m[1])] = v
	}
	if len(out) == 0 {
		t.Fatalf("%s parsed to no values, so this test would pass against anything", asn1Type)
	}

	return out
}

// ngapModuleDir locates the pinned NGAP module, whose generated sources are the definition of
// what values this element can be handed.
func ngapModuleDir(t *testing.T) string {
	t.Helper()

	out, err := exec.CommandContext(t.Context(),
		"go", "list", "-m", "-f", "{{.Dir}}", "github.com/omec-project/ngap/v2").Output()
	if err != nil {
		t.Fatalf("cannot locate the ngap module (%v); without it the totality assertion below "+
			"has nothing to be total over", err)
	}

	return strings.TrimSpace(string(out))
}

// ngapValues is every value of one NGAP cause group, keyed by normalised name.
//
// **Parsed from the module's own generated source, not listed here.** A list would be a
// transcription of the very thing this test exists to be total over: a value NGAP adds when the
// pin is bumped would be absent from the list, the test would still pass, and the mapping would
// carry that value straight through into a number TS 33.128 does not define. Reading the source
// means bumping the pin is what surfaces it.
func ngapValues(t *testing.T, dir, asn1Type string) map[string]int64 {
	t.Helper()

	src, err := os.ReadFile(filepath.Join(dir, "ngapType", asn1Type+".go"))
	if err != nil {
		t.Fatalf("reading the NGAP definition of %s: %v", asn1Type, err)
	}

	decl := regexp.MustCompile(asn1Type + `Present(\w+)\s+aper\.Enumerated = (\d+)`)
	out := map[string]int64{}
	for _, m := range decl.FindAllStringSubmatch(string(src), -1) {
		v, err := strconv.ParseInt(m[2], 10, 64)
		if err != nil {
			t.Fatalf("%s: %q is not a number", asn1Type, m[2])
		}
		out[normaliseCauseName(m[1])] = v
	}
	if len(out) == 0 {
		t.Fatalf("no NGAP values parsed for %s, so this test would pass against anything", asn1Type)
	}

	return out
}

// TestTheCauseMappingIsTotalOverNGAP asserts the mapping over every NGAP cause.
//
// For every value of every NGAP cause group: either TS 33.128 defines a cause of the same name,
// in which case liCauseValue must produce *that* value and not substitute, or it does not, in
// which case liCauseValue must substitute. A value that falls through — mapped to something TS
// 33.128 does not define — is a record carrying a cause no receiver can interpret, and a value
// substituted when the specification had one for it is a loss of precision nobody asked for.
func TestTheCauseMappingIsTotalOverNGAP(t *testing.T) {
	modulePath := ts33128Module(t)
	ngapDir := ngapModuleDir(t)

	for _, g := range causeGroups {
		t.Run(g.asn1Type, func(t *testing.T) {
			li := ts33128Values(t, modulePath, g.asn1Type)
			ngap := ngapValues(t, ngapDir, g.asn1Type)
			if len(ngap) == 0 {
				t.Fatalf("no NGAP values for %s, so this subtest asserts nothing", g.asn1Type)
			}

			var highest int64
			for _, v := range ngap {
				if v > highest {
					highest = v
				}
			}
			if declared := highestNGAPValue[g.group]; declared != highest {
				t.Errorf("the pinned NGAP module defines %s up to %d and the mapping declares %d "+
					"as its highest: %s", g.asn1Type, highest, declared,
					map[bool]string{
						true: "values this element can now be sent are substituted rather than " +
							"carried, silently losing causes TS 33.128 may well define",
						false: "values NGAP does not define are shifted into numbers outside the " +
							"group's TS 33.128 range, which the constraint check refuses — so the " +
							"record is lost rather than its precision",
					}[declared < highest])
			}

			for name, ngapValue := range ngap {
				key := name
				if alias, ok := spellingAliases[name]; ok {
					key = alias
				}
				want, defined := li[key]

				// From the wire shape, through the function the handler calls — so the arm this
				// cause arrived on and the number the record will carry are checked together.
				gotGroup, got, substituted, ok := ngapCauseToLI(g.cause(ngapValue))
				if !ok {
					t.Errorf("%s(%d): a cause present on the wire produced no LI cause, so the "+
						"handover goes unreported entirely", name, ngapValue)

					continue
				}
				if gotGroup != g.group {
					t.Errorf("%s(%d): arrived on the %s arm and was reported under group %d — a "+
						"cause reported under the wrong group describes a different event, and the "+
						"value alone cannot tell them apart", name, ngapValue, g.asn1Type, gotGroup)
				}
				switch {
				case defined && substituted:
					t.Errorf("%s(%d): TS 33.128 defines this cause as %d and the mapping "+
						"substituted; the record loses a reason the specification could carry",
						name, ngapValue, want)
				case defined && got != want:
					t.Errorf("%s(%d): mapped to %d, and TS 33.128 defines it as %d — a record "+
						"built from this carries a cause the receiver reads as a different one",
						name, ngapValue, got, want)
				case !defined && !substituted:
					t.Errorf("%s(%d): TS 33.128 has no value for this cause and the mapping "+
						"produced %d anyway, which its enumeration does not define",
						name, ngapValue, got)
				}
			}
		})
	}
}

// TestASubstitutedCauseIsTheGroupsUnspecified pins what a substitution produces. The value has
// to be one the record's definition permits, or the substitution trades an unrepresentable
// cause for an unencodable record.
func TestASubstitutedCauseIsTheGroupsUnspecified(t *testing.T) {
	modulePath := ts33128Module(t)

	for _, g := range causeGroups {
		li := ts33128Values(t, modulePath, g.asn1Type)
		want, ok := li["unspecified"]
		if !ok {
			t.Fatalf("%s has no `unspecified` value, so there is nothing to substitute with", g.asn1Type)
		}
		// Above every group's NGAP maximum, so it is unrepresentable in every group — this is
		// the value a later NGAP release could introduce, and the one whose shift would produce
		// a record the constraint check refuses.
		if got, substituted := liCauseValue(g.group, 9999); !substituted || got != want {
			t.Errorf("%s: an unrepresentable cause mapped to (%d, substituted=%v), want (%d, true)",
				g.asn1Type, got, substituted, want)
		}
	}
}

// TestTheMappingIsNotAnOffsetAlone fails a mapping that emits the NGAP value unchanged, which
// puts every cause one below the value TS 33.128 defines.
// Asserted on the two causes that make the difference unmistakable: `unspecified`, which NGAP
// numbers 0 and TS 33.128 numbers 1 — a zero the record's own enumeration does not include —
// and a NAS cause NGAP has and TS 33.128 does not.
func TestTheMappingIsNotAnOffsetAlone(t *testing.T) {
	got, substituted := liCauseValue(lawfulintercept.CauseGroupRadioNetwork,
		int64(ngapType.CauseRadioNetworkPresentUnspecified))
	if substituted {
		t.Error("radio-network `unspecified` was substituted; TS 33.128 defines it")
	}
	if got == int64(ngapType.CauseRadioNetworkPresentUnspecified) {
		t.Errorf("radio-network `unspecified` mapped to %d, which is NGAP's numbering: the "+
			"record's enumeration starts at 1 and does not include 0, so this is the defect "+
			"this mapping exists to close", got)
	}

	if _, substituted := liCauseValue(lawfulintercept.CauseGroupNAS,
		int64(ngapType.CauseNasPresentMobileIABNotAuthorized)); !substituted {
		t.Error("a NAS cause TS 33.128 has no value for was carried through rather than substituted")
	}

	// And the hole: TS 33.128's radio-network enumeration has no 28, so nothing may map to it.
	// This is what makes the looser range bound in li/iri cost nothing in practice.
	for _, g := range causeGroups {
		if g.group != lawfulintercept.CauseGroupRadioNetwork {
			continue
		}
		for v := int64(0); v <= 57; v++ {
			if got, substituted := liCauseValue(g.group, v); !substituted && got == 28 {
				t.Errorf("NGAP radio-network %d mapped to 28, which TS 33.128 does not define", v)
			}
		}
	}
}

// TestACauseWithNoArmSetProducesNoCause covers what ReportHandoverRequest depends on: a `Cause`
// whose discriminant names an arm the sender left nil, or one this element does not handle,
// yields ok=false so no record is built.
//
// The alternative is worse than silence. `handoverCause` is mandatory and the CHOICE has no
// absent form, so a cause that cannot be read would have to be invented — and an invented cause
// is indistinguishable, to the agency, from one the network gave.
func TestACauseWithNoArmSetProducesNoCause(t *testing.T) {
	for _, c := range []struct {
		name  string
		cause *ngapType.Cause
	}{
		{"no arm at all", &ngapType.Cause{Present: ngapType.CausePresentNothing}},
		{"radioNetwork named but nil", &ngapType.Cause{Present: ngapType.CausePresentRadioNetwork}},
		{"transport named but nil", &ngapType.Cause{Present: ngapType.CausePresentTransport}},
		{"nas named but nil", &ngapType.Cause{Present: ngapType.CausePresentNas}},
		{"protocol named but nil", &ngapType.Cause{Present: ngapType.CausePresentProtocol}},
		{"misc named but nil", &ngapType.Cause{Present: ngapType.CausePresentMisc}},
		// An arm added to NGAP that this element does not read: it must not be silently
		// attributed to a group.
		{"choice extension", &ngapType.Cause{Present: ngapType.CausePresentChoiceExtensions}},
	} {
		if _, _, _, ok := ngapCauseToLI(c.cause); ok {
			t.Errorf("%s: produced a usable cause, so a record would carry a cause nobody sent", c.name)
		}
	}
}

// TestTheHandoverTypeMappingIsTotalOverNGAP is the cause test's twin, for the field beside it.
//
// The two sit in the same record and fail the same way, so the assertion is the same shape: every
// value NGAP defines, matched by name against the TS 33.128 module, with no arithmetic restated.
func TestTheHandoverTypeMappingIsTotalOverNGAP(t *testing.T) {
	modulePath := ts33128Module(t)
	li := ts33128Values(t, modulePath, "HandoverType")
	ngap := ngapValues(t, ngapModuleDir(t), "HandoverType")

	if len(li) != 4 || len(ngap) != 4 {
		t.Fatalf("parsed %d TS 33.128 and %d NGAP handover types, want 4 and 4 — a parse that "+
			"finds the wrong number is checking something other than these enumerations",
			len(li), len(ngap))
	}

	// The bound the mapping claims, against the definition it is claimed over. The count check
	// above would also catch NGAP gaining a value, but this asserts what the mapping relies on —
	// that the domain is bounded *and asserted against the source* — and it is the same shape as
	// the cause groups'. A constant that agrees with the module by coincidence is not the same as
	// one that is checked.
	var highest int64
	for _, v := range ngap {
		if v > highest {
			highest = v
		}
	}
	if highest != highestNGAPHandoverType {
		t.Errorf("the pinned NGAP module defines handover types up to %d and the mapping declares "+
			"%d: a value above the declared bound is substituted rather than carried, and a value "+
			"below it is shifted into a number TS 33.128 does not define",
			highest, highestNGAPHandoverType)
	}

	// The two spell them differently: NGAP has FivegsToEps, the module has fiveGStoEPS. The
	// same normalisation the causes use joins those, and one pair it does not — listed rather
	// than fuzzily matched, for the reason spellingAliases gives.
	aliases := map[string]string{
		// NGAP says UTRAN, the module says UTRA. Both mean the UMTS terrestrial radio access
		// network, they sit at corresponding positions in the two enumerations, and one of the
		// two documents is dropping a letter. A fuzzy match would join these *and* join things
		// that are not the same, which is the failure this mapping exists to prevent.
		"fivegstoutran": "fivegstoutra",
	}

	for name, ngapValue := range ngap {
		key := name
		if alias, ok := aliases[name]; ok {
			key = alias
		}
		want, defined := li[key]
		got, substituted := liHandoverType(ngapValue)
		switch {
		case defined && substituted:
			t.Errorf("%s(%d): TS 33.128 defines this as %d and the mapping substituted", name, ngapValue, want)
		case defined && got != want:
			t.Errorf("%s(%d): mapped to %d, and TS 33.128 defines it as %d. A record carrying "+
				"this names a different kind of handover than the one that happened — and for "+
				"the value NGAP numbers zero, a kind the enumeration does not define at all, "+
				"which makes the whole record undecodable", name, ngapValue, got, want)
		case !defined:
			t.Errorf("%s(%d): no TS 33.128 handover type of this name; the module and the NGAP "+
				"module have diverged and the mapping needs an explicit entry", name, ngapValue)
		}
	}
}

// TestAnIntra5GSHandoverIsNotEmittedAsZero pins, on its own, the value a conformant receiver
// refuses.
//
// NGAP numbers intra-5GS zero and TS 33.128 numbers it one. Zero is the ordinary case — most
// handovers are intra-5GS — so emitted as zero it is not an edge: it is the majority of
// handover records, and the receiver discards every one of them.
func TestAnIntra5GSHandoverIsNotEmittedAsZero(t *testing.T) {
	got, substituted := liHandoverType(int64(ngapType.HandoverTypePresentIntra5gs))
	if substituted {
		t.Error("intra-5GS was substituted; TS 33.128 defines it")
	}
	if got == 0 {
		t.Error("an intra-5GS handover is emitted as handoverType 0, which the record's " +
			"enumeration does not define. The published decoder refuses the whole record, so " +
			"nothing arrives and this element believes it delivered")
	}
	if got != int64(iri.HandoverIntra5GS) {
		t.Errorf("intra-5GS mapped to %d, want %d", got, iri.HandoverIntra5GS)
	}
}

// TestAHandoverTypeBeyondNGAPIsSubstituted: NGAP's enumeration is extensible, so a value from a
// later release would otherwise be shifted into a number TS 33.128 does not define.
func TestAHandoverTypeBeyondNGAPIsSubstituted(t *testing.T) {
	if got, substituted := liHandoverType(4); !substituted || got != int64(iri.HandoverIntra5GS) {
		t.Errorf("a handover type beyond what NGAP defines mapped to (%d, %v), want (%d, true)",
			got, substituted, iri.HandoverIntra5GS)
	}
}

// pinnedUtilVersion is the util version, which carries li, that this module actually builds
// against, or "" if it cannot be read. It is used to say which module these assertions ran
// against rather than leaving it assumed.
//
// **A replace directive governs, not the require.** When go.mod carries a `replace` for util,
// the replacement is what is compiled and the require line is only a floor, so reading the
// first matching line would answer with a version nothing builds against.
func pinnedUtilVersion(t *testing.T) string {
	t.Helper()

	mod, err := os.ReadFile("../go.mod")
	if err != nil {
		return ""
	}

	var required string

	for _, line := range strings.Split(string(mod), "\n") {
		line = strings.TrimSpace(line)
		if !strings.Contains(line, "omec-project/util") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}

		// A replace is definitive; keep looking past a require in case one follows.
		if strings.HasPrefix(line, "replace ") {
			return fields[len(fields)-1]
		}

		if required == "" {
			required = fields[len(fields)-1]
		}
	}

	return required
}
