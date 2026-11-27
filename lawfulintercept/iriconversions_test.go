// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package lawfulintercept

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// Every value this element puts into a TS 33.128 record arrives through a conversion into one
// of `li/iri`'s types, and a raw cast into a record enumeration is a correspondence nobody
// checks.
//
// `handoverCause` and `handoverType` are the cases in point: NGAP numbers both differently from
// TS 33.128, so a cast carries the wrong cause, and `intra5gs(0)` makes every intra-5GS handover
// record undecodable on receipt. Defects of that kind surface only as a delivered record being
// wrong. **That is not a check**, so the next raw cast into a record enumeration needs one.
//
// **This keys on the shape, not on a list of enumerations.** The tempting design is "for each
// known `iri` enum type, require an assertion", and that list is the thing that goes stale: a
// new enumeration in `li` is invisible to a list maintained here. Inverted, the staleness works
// for us — every conversion `iri.T(x)` whose argument is not one of the package's own `iri.`
// constants must be recorded, so a new enumerated conversion arrives as an unlisted site rather
// than as a type nobody added.
//
// Same trade as `li/iri`'s `unrestrictedBareFields`: a one-time cost of writing the entries, for
// the next one being a decision rather than a diff nobody reads.
//
// **What it cannot see**, said out loud because the gap is real: an *implicit* conversion. A
// function declared to return `iri.AccessType` that writes `return 0` produces a value of that
// type with no conversion expression for this scan to find, and `DeregistrationScope` in this
// package does exactly that — deliberately, with both callers expected to notice the zero and
// substitute the arrival access. The encode-time guard in `li/iri` is what covers those, and the two halves
// are complementary rather than redundant — this one catches an unasserted correspondence, that
// one catches a value the enumeration does not define however it got there.

// conversionKind is what an entry claims about the conversion.
type conversionKind int

const (
	// carried — the target is not an enumeration, so there are no two numberings to reconcile.
	// A DNN is a DNN; a PDU session ID is an integer in both definitions.
	carried conversionKind = iota
	// mapped — the target is an enumeration, and some other definition numbers the same
	// concept. The correspondence has to be asserted somewhere, and the entry names where.
	mapped
	// built — not a conversion at all but a call into `li/iri`, whose own correctness is that
	// package's. Recorded because the scan cannot tell a type from a function, and saying which
	// is which is cheaper than a scan that guesses.
	built
)

// iriConversion is one recorded conversion target.
type iriConversion struct {
	kind conversionKind
	// why, for carried and built.
	note string
	// assertion, for mapped only: the name of the test that establishes the correspondence, so
	// a reader does not have to re-derive whether the cast is safe. Asserted to exist — a note
	// naming a test nobody wrote is the same claim-with-nothing-behind-it a cast is.
	assertion string
}

// recordedConversions are every `iri.X(...)` this package's module performs on something other
// than one of `li/iri`'s own constants.
//
// The distinction between *carried* and *mapped* is the whole point: it is the difference
// between a value that means the same thing in both definitions and a value whose meaning
// depends on two enumerations agreeing. A cast is a correspondence claim with nothing behind it,
// and it looks identical either way from the call site.
var recordedConversions = map[string]iriConversion{
	// The enumerated conversions, which are the reason this check exists. All seven are the
	// handover path, and all seven are covered by one assertion pair in amf/ngap.
	//
	// `handoverType` and `handoverCause` are recorded here as mapped and asserted there; the
	// note is what stops the next reader re-deriving whether the cast is safe.
	"HandoverType": {
		kind: mapped,
		note: "NGAP numbers the handover types from 0 and TS 33.128 from 1, so the value is " +
			"mapped in amf/ngap before it reaches the record — the defect that made every " +
			"intra-5GS handover record undecodable was this cast performed raw",
		assertion: "TestTheHandoverTypeMappingIsTotalOverNGAP",
	},
	"CauseRadioNetwork": {kind: mapped, note: causeNote, assertion: causeAssertion},
	"CauseTransport":    {kind: mapped, note: causeNote, assertion: causeAssertion},
	"CauseNas":          {kind: mapped, note: causeNote, assertion: causeAssertion},
	"CauseProtocol":     {kind: mapped, note: causeNote, assertion: causeAssertion},
	"CauseMisc":         {kind: mapped, note: causeNote, assertion: causeAssertion},

	// Identity leaves. Their values come from the UE identity snapshot, ultimately from a task
	// li/x1 validated on the decode path, and none of them is an enumeration.
	"IMSI":   {kind: carried, note: "SUPI CHOICE arm; digits, no second numbering"},
	"NAI":    {kind: carried, note: "SUPI CHOICE arm; a string, no second numbering"},
	"IMEI":   {kind: carried, note: "PEI CHOICE arm"},
	"IMEISV": {kind: carried, note: "PEI CHOICE arm"},
	"MSISDN": {kind: carried, note: "GPSI CHOICE arm"},

	// NGAP UE identifiers and the opaque containers. Integers and octet strings in both
	// definitions; what has to hold is the range, which li/iri's constraint tables enforce.
	"AMFUENGAPID":                {kind: carried, note: "INTEGER (0..1099511627775)"},
	"RANUENGAPID":                {kind: carried, note: "INTEGER (0..4294967295)"},
	"RANTargetToSourceContainer": {kind: carried, note: "OCTET STRING; the NGAP container verbatim"},
	"RANSourceToTargetContainer": {kind: carried, note: "OCTET STRING; the NGAP container verbatim"},
	"PDUSessionID":               {kind: carried, note: "INTEGER (0..255)"},
	"UEPolicy":                   {kind: carried, note: "OCTET STRING (SIZE(16..65540)); the NAS container verbatim"},

	// GUTI and GUAMI members, and the PLMN. All from this element's configuration and context.
	"FiveGTMSI":   {kind: carried, note: "INTEGER (0..4294967295)"},
	"MCC":         {kind: carried, note: "NumericString (SIZE(3)); the PLMN digits"},
	"MNC":         {kind: carried, note: "NumericString (SIZE(2..3)); the PLMN digits"},
	"AMFRegionID": {kind: carried, note: "INTEGER (0..255); a field of the GUAMI"},
	"AMFSetID":    {kind: carried, note: "INTEGER (0..1023); a field of the GUAMI"},
	"AMFPointer":  {kind: carried, note: "INTEGER (0..63); a field of the GUAMI"},

	// SUCI members, decoded from the NAS message the UE sent. RoutingIndicator,
	// ProtectionSchemeID and RoutingIndicatorLength are INTEGERs with ranges rather than
	// enumerations — ProtectionSchemeID(0) is the null scheme, a value a UE legitimately
	// sends, which is why it is carried and not guarded.
	"RoutingIndicator":       {kind: carried, note: "INTEGER (0..9999); the routing indicator digits"},
	"ProtectionSchemeID":     {kind: carried, note: "INTEGER (0..15); 0 is the null scheme, a value in range"},
	"RoutingIndicatorLength": {kind: carried, note: "INTEGER (1..4); carried explicitly for leading zeros"},
	"SchemeOutput":           {kind: carried, note: "OCTET STRING, unconstrained; the MSIN digits as characters"},

	// A 5GMM cause. INTEGER (0..255) in the module, not an ENUMERATED, so there is no second
	// enumeration to correspond to — a receiver validates any octet. TS 24.501's 5GMM causes
	// start at 3, so zero is not one of them, but guarding zero here would refuse a record a
	// conformant receiver accepts. Same reasoning as FiveGSMCause in the SMF.
	"FiveGMMCause": {kind: carried, note: "INTEGER (0..255); an integer range, not an enumeration"},

	// Calls into li/iri rather than conversions.
	"EncodeXIRI":  {kind: built, note: "the encode entry point, which is where validateConstraints runs"},
	"Identifiers": {kind: built, note: "builds the UserIdentifiers SEQUENCE from the identity snapshot"},
}

// causeNote and causeAssertion are shared by the five HandoverCause arms: one mapping, one
// assertion, five arms. Written once because five copies of the same claim is five places for it
// to drift.
const (
	causeNote = "NGAP numbers each Cause group from 0 and TS 33.128 from 1, and TS 33.128 has " +
		"no equivalent for nine NGAP values; the correspondence is established by name in " +
		"amf/ngap/licause.go, not by this cast"
	causeAssertion = "TestTheCauseMappingIsTotalOverNGAP"
)

// conversionSite is one occurrence, for the failure messages: a check that says only which type
// was unrecorded leaves the reader grepping.
type conversionSite struct {
	target string
	file   string
	line   int
}

// scanIRIConversions walks every non-test Go file in this module that imports `li/iri` and
// returns each conversion whose argument is not one of that package's own constants.
//
// **The whole module, not just this package.** The check belongs beside the conversions, and
// this package is where nearly all of them are — but `amf/ngap` imports `li/iri` too, and a
// conversion added in a sibling package would be invisible to a scan of one directory. Widening
// it costs a directory walk and closes that by construction.
func scanIRIConversions(t *testing.T) []conversionSite {
	t.Helper()

	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("resolving the module root: %v", err)
	}
	// A check that cannot run is not a check that passes.
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("%s has no go.mod, so this scan is not looking at the module it thinks: %v",
			root, err)
	}

	var sites []conversionSite
	fset := token.NewFileSet()
	filesScanned := 0

	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case "vendor", "testdata", ".git", "bin":
				return fs.SkipDir
			}

			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			// Not fatal: a file this parser cannot read is a build failure the compiler
			// reports better than a test can, and failing here would hide it behind this one.
			t.Logf("skipping %s: %v", path, err)

			return nil
		}

		alias := iriImportAlias(file)
		if alias == "" {
			return nil
		}
		filesScanned++

		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			rel = path
		}

		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok || pkg.Name != alias {
				return true
			}
			// A conversion whose argument is one of `li/iri`'s own constants —
			// `iri.AccessType(iri.AccessBoth)` — reconciles nothing and is not what this
			// check is about.
			if len(call.Args) == 1 && isQualifiedBy(call.Args[0], alias) {
				return true
			}
			sites = append(sites, conversionSite{
				target: sel.Sel.Name,
				file:   rel,
				line:   fset.Position(sel.Sel.Pos()).Line,
			})

			return true
		})

		return nil
	})
	if walkErr != nil {
		t.Fatalf("walking %s: %v", root, walkErr)
	}
	if filesScanned == 0 {
		t.Fatalf("no non-test file in %s imports li/iri, so this scan found nothing to check "+
			"and would pass against a package that had stopped building records entirely", root)
	}

	sort.Slice(sites, func(i, j int) bool {
		if sites[i].target != sites[j].target {
			return sites[i].target < sites[j].target
		}
		if sites[i].file != sites[j].file {
			return sites[i].file < sites[j].file
		}

		return sites[i].line < sites[j].line
	})

	return sites
}

// iriImportAlias returns the local name file gives `li/iri`, or "" if it does not import it.
func iriImportAlias(file *ast.File) string {
	for _, imp := range file.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil || path != "github.com/omec-project/util/li/iri" {
			continue
		}
		if imp.Name != nil {
			return imp.Name.Name
		}

		return "iri"
	}

	return ""
}

// isQualifiedBy reports whether e is `alias.Something` — one of the imported package's own
// identifiers rather than a value from elsewhere in this element.
func isQualifiedBy(e ast.Expr, alias string) bool {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)

	return ok && pkg.Name == alias
}

// declaredTests are the test functions this module declares, keyed by name, so an entry naming
// an assertion can be checked against the assertions that exist.
//
// **The module, not this package.** The conversions are here and the assertions that cover them
// need not be: the handover cause and type mappings are asserted in `amf/ngap`, beside the
// mapping functions and the two definitions they read. A per-package scan would force
// either a duplicate assertion or a note naming nothing.
func declaredTests(t *testing.T) map[string]bool {
	t.Helper()

	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("resolving the module root: %v", err)
	}

	fset := token.NewFileSet()
	out := map[string]bool{}
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case "vendor", "testdata", ".git", "bin":
				return fs.SkipDir
			}

			return nil
		}
		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			t.Logf("skipping %s: %v", path, parseErr)

			return nil
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if ok && fn.Recv == nil && strings.HasPrefix(fn.Name.Name, "Test") {
				out[fn.Name.Name] = true
			}
		}

		return nil
	})
	if walkErr != nil {
		t.Fatalf("walking %s: %v", root, walkErr)
	}
	if len(out) == 0 {
		t.Fatal("found no test functions in this module, so the assertion-exists check below " +
			"would reject every mapped entry for the wrong reason")
	}

	return out
}

// TestEveryIRIConversionIsRecorded fails on any conversion into a record type that no entry
// accounts for.
//
// The AMF's population is seven enumerated conversions across the handover path, all of them
// asserted in `amf/ngap`, plus the carried identity and container leaves. The check is written
// where the conversions are, and being green with all of them recorded is what makes the next
// one visible.
func TestEveryIRIConversionIsRecorded(t *testing.T) {
	sites := scanIRIConversions(t)
	if len(sites) == 0 {
		t.Fatal("the scan found no conversion into an li/iri type, so it is asserting nothing")
	}
	tests := declaredTests(t)

	seen := map[string]bool{}
	for _, s := range sites {
		seen[s.target] = true

		entry, recorded := recordedConversions[s.target]
		if !recorded {
			t.Errorf("%s:%d converts into iri.%s and nothing records what that conversion "+
				"claims. If iri.%s is an enumeration, this is a correspondence claim with "+
				"nothing behind it — the construct that made every handover record carry the "+
				"wrong cause — and it needs an assertion against both definitions, recorded "+
				"here as `mapped`. If it is not, record it as `carried` and say why there is "+
				"nothing to reconcile", s.file, s.line, s.target, s.target)

			continue
		}
		if entry.note == "" {
			t.Errorf("iri.%s is recorded with no note; an entry that says nothing is "+
				"indistinguishable from an omission", s.target)
		}
		if entry.kind != mapped {
			if entry.assertion != "" {
				t.Errorf("iri.%s is not recorded as mapped and names an assertion (%s); the two "+
					"fields say different things and only mapped needs the second",
					s.target, entry.assertion)
			}

			continue
		}
		switch {
		case entry.assertion == "":
			t.Errorf("iri.%s is recorded as mapped and names no assertion. `mapped` means two "+
				"enumerations number the same concept and something checks they still agree; "+
				"without naming that something the entry is a claim, not a check", s.target)
		case !tests[entry.assertion]:
			t.Errorf("iri.%s names %s as the assertion covering it and no test of that name "+
				"exists in this module. A note naming a test nobody wrote is exactly the "+
				"claim-with-nothing-behind-it that the bare cast was",
				s.target, entry.assertion)
		}
	}

	// A stale entry is the other direction: an exemption lying where a future conversion of
	// that name can find it, granted by nobody.
	for target := range recordedConversions {
		if !seen[target] {
			t.Errorf("iri.%s is recorded as a conversion this module performs and it performs "+
				"none; remove the entry rather than leaving a recorded claim for a conversion "+
				"that has gone", target)
		}
	}

	var listed []string
	for _, s := range sites {
		listed = append(listed, s.file+":"+strconv.Itoa(s.line)+" iri."+s.target)
	}
	t.Logf("%d conversion sites over %d targets:\n  %s",
		len(sites), len(seen), strings.Join(listed, "\n  "))
}
