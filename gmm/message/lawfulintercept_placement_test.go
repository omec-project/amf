// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package message

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// callsIn reports the calls inside one function of send.go, closures included, in source
// order: selector calls as "x.Fn" and bare calls as "Fn".
func callsIn(t *testing.T, fn string) []string {
	t.Helper()

	const file = "send.go"

	parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", file, err)
	}

	var target *ast.FuncDecl
	for _, decl := range parsed.Decls {
		if fd, isFunc := decl.(*ast.FuncDecl); isFunc && fd.Name.Name == fn {
			target = fd
			break
		}
	}
	if target == nil {
		t.Fatalf("no %s in %s; if it was renamed, move this guard with it", fn, file)
	}

	var names []string
	ast.Inspect(target, func(n ast.Node) bool {
		call, isCall := n.(*ast.CallExpr)
		if !isCall {
			return true
		}
		switch fun := call.Fun.(type) {
		case *ast.SelectorExpr:
			if x, isIdent := fun.X.(*ast.Ident); isIdent {
				names = append(names, x.Name+"."+fun.Sel.Name)
			}
		case *ast.Ident:
			names = append(names, fun.Name)
		}
		return true
	})

	return names
}

func count(names []string, want string) int {
	n := 0
	for _, name := range names {
		if name == want {
			n++
		}
	}
	return n
}

func firstIndex(names []string, want string) int {
	for i, name := range names {
		if name == want {
			return i
		}
	}
	return -1
}

// T3522 running out ends a network-initiated deregistration without an accept: each branch of
// the abort sets the access Deregistered and removes the UE. TS 33.128 clause 6.2.2.2.3 reports
// a UE that "has deregistered from the 5GS over at least one access type", so every branch that
// removes the UE reports the deregistration, and the end of its identifier binding.
func TestEveryT3522AbortReportsTheDeregistration(t *testing.T) {
	calls := callsIn(t, "SendDeregistrationRequest")

	removals := count(calls, "amfUe.Remove")
	if removals == 0 {
		t.Fatal("SendDeregistrationRequest removes no UE; this guard no longer describes the code")
	}
	if got := count(calls, "lawfulintercept.ReportDeregistration"); got != removals {
		t.Errorf("%d T3522 abort branches remove the UE and %d report the deregistration", removals, got)
	}
	if got := count(calls, "lawfulintercept.ReportIdentifierDeassociation"); got != removals {
		t.Errorf("%d T3522 abort branches remove the UE and %d report the deassociation", removals, got)
	}
}

// A reject that could not be built is not sent, so it is not reported either.
func TestARegistrationRejectIsReportedOnceBuilt(t *testing.T) {
	calls := callsIn(t, "SendRegistrationReject")

	build := firstIndex(calls, "BuildRegistrationReject")
	report := firstIndex(calls, "lawfulintercept.ReportRegistrationReject")
	if build < 0 || report < 0 {
		t.Fatalf("SendRegistrationReject builds at %d and reports at %d; both must be present", build, report)
	}
	if report < build {
		t.Error("the reject is reported before it is built, so one that fails to build is reported as sent")
	}
}

// T3560 running out removes the UE in the middle of a registration procedure, which ends a
// registration the procedure began from. Each removal hands the procedure to
// lawfulintercept.ReportAbortedRegistration first, which reports the deregistration when the
// UE was registered (TS 33.128 clause 6.2.2.2.3).
func TestEveryT3560AbortAccountsForTheRegistrationItEnds(t *testing.T) {
	for _, fn := range []string{"SendAuthenticationRequest", "SendSecurityModeCommand"} {
		calls := callsIn(t, fn)

		remove := firstIndex(calls, "amfUe.Remove")
		report := firstIndex(calls, "lawfulintercept.ReportAbortedRegistration")
		if remove < 0 {
			t.Fatalf("%s removes no UE; this guard no longer describes the code", fn)
		}
		if report < 0 || report > remove {
			t.Errorf("%s reports the aborted registration at %d and removes the UE at %d; "+
				"the report must come first", fn, report, remove)
		}
	}
}
