// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package gmm

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// calledIn reports the names called inside one function of a file, in source order, restricted to
// selector calls like pkg.Fn and to bare calls like Fn.
func calledIn(t *testing.T, file, fn string) []string {
	t.Helper()

	fset := token.NewFileSet()

	parsed, err := parser.ParseFile(fset, file, nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("parsing %s: %v", file, err)
	}

	var target *ast.FuncDecl

	for _, decl := range parsed.Decls {
		if fd, isFunc := decl.(*ast.FuncDecl); isFunc && fd.Name != nil && fd.Name.Name == fn {
			target = fd
			break
		}
	}

	if target == nil {
		t.Fatalf("no %s in %s; if it was renamed, move this guard with it rather than deleting it",
			fn, file)
	}

	var names []string

	ast.Inspect(target, func(n ast.Node) bool {
		call, isCall := n.(*ast.CallExpr)
		if !isCall {
			return true
		}

		switch fun := call.Fun.(type) {
		case *ast.SelectorExpr:
			if pkg, isIdent := fun.X.(*ast.Ident); isIdent && fun.Sel != nil {
				names = append(names, pkg.Name+"."+fun.Sel.Name)
			}
		case *ast.Ident:
			names = append(names, fun.Name)
		}

		return true
	})

	return names
}

func indexOf(names []string, want string) int {
	for i, n := range names {
		if n == want {
			return i
		}
	}

	return -1
}

// TestANetworkInitiatedDeregistrationIsReportedWhereItCompletes guards the placement of an
// interception obligation.
//
// NetworkInitiatedDeregistrationProcedure branches on whether the UE is connected and registered.
// The arm taken in the ordinary case only sends a DEREGISTRATION REQUEST, but it is a
// deregistration all the same: the UE answers with a DEREGISTRATION ACCEPT,
// HandleDeregistrationAccept runs, the state becomes Deregistered, and every SM context is
// released below the branch on both arms. Reported from the `else` arm alone, a target the
// network forcibly deregistered produces no AMFDeregistration and no AMFIdentifierDeassociation,
// and is indistinguishable to the agency from one that simply went quiet.
//
// The rule this encodes: the obligation belongs where the procedure *completes*, not where it is
// started. A branch chosen at the beginning of a procedure may complete it by another route.
//
// A behavioural assertion would need a production seam in this package to observe the call —
// lawfulintercept.ReportDeregistration is called directly rather than through a package variable.
// This is the cheaper half of that trade, and it fails if the report moves off the accept.
func TestANetworkInitiatedDeregistrationIsReportedWhereItCompletes(t *testing.T) {
	const file = "handler.go"

	accept := calledIn(t, file, "HandleDeregistrationAccept")

	report := indexOf(accept, "lawfulintercept.ReportDeregistration")
	if report < 0 {
		t.Fatal("HandleDeregistrationAccept does not report the deregistration. It is the point " +
			"at which a network-initiated deregistration completes — the UE has accepted and the " +
			"event below sets the state to Deregistered — so a target the network deregistered " +
			"produces no record at all and looks to the agency exactly like one that went quiet")
	}

	// Before the event that writes the state, because deregisteringEveryAccess must read the
	// state the record describes rather than the state the event is about to write.
	if event := indexOf(accept, "GmmFSM.SendEvent"); event >= 0 && report > event {
		t.Error("the deregistration is reported after the event that sets the state to " +
			"Deregistered, so the binding decision reads a state the record does not describe")
	}

	if indexOf(accept, "lawfulintercept.ReportIdentifierDeassociation") < 0 {
		t.Error("HandleDeregistrationAccept does not release the identifier binding. The " +
			"mediation function is left holding a SUPI↔5G-GUTI association this element has " +
			"abandoned")
	}

	// And the other arm still reports, because it completes the procedure itself rather than
	// waiting for an accept. Losing that would move the silence rather than remove it.
	initiating := calledIn(t, file, "NetworkInitiatedDeregistrationProcedure")
	if indexOf(initiating, "lawfulintercept.ReportDeregistration") < 0 {
		t.Error("NetworkInitiatedDeregistrationProcedure no longer reports on the arm that " +
			"completes the deregistration itself — the UE that is not connected, which is " +
			"deregistered in place without an accept ever arriving")
	}
}

// A message other than DEREGISTRATION ACCEPT while the network-initiated deregistration waits
// for one moves the access to Deregistered through the same event, which records nothing
// itself. So the branch that sends it reports the deregistration, and the binding when it goes.
func TestAnotherMessageInPlaceOfTheAcceptIsReported(t *testing.T) {
	calls := calledIn(t, "sm.go", "DeregisteredInitiated")

	for _, want := range []string{
		"lawfulintercept.ReportDeregistration",
		"lawfulintercept.ReportIdentifierDeassociation",
	} {
		if indexOf(calls, want) < 0 {
			t.Errorf("DeregisteredInitiated does not call %s on the state-mismatch branch", want)
		}
	}
}
