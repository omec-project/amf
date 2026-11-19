// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package gmm

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// tapGuard finds the `if` that gates a registration report in one function, and returns the
// comparison operator it uses.
func tapGuard(t *testing.T, file, fn string) token.Token {
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
		t.Fatalf("no %s in %s; if a registration tap moved, move this guard with it", fn, file)
	}

	var op token.Token

	ast.Inspect(target, func(n ast.Node) bool {
		stmt, isIf := n.(*ast.IfStmt)
		if !isIf {
			return true
		}

		cmp, isCmp := stmt.Cond.(*ast.BinaryExpr)
		if !isCmp {
			return true
		}

		// The guard compares the UE's registration type against mobility-registration-updating,
		// and its body reports the registration.
		if !isRegistrationTypeComparison(cmp) {
			return true
		}

		var reports bool

		ast.Inspect(stmt.Body, func(inner ast.Node) bool {
			call, isCall := inner.(*ast.CallExpr)
			if !isCall {
				return true
			}

			if sel, isSel := call.Fun.(*ast.SelectorExpr); isSel && sel.Sel != nil &&
				sel.Sel.Name == "ReportRegistration" {
				reports = true
			}

			return true
		})

		if reports {
			op = cmp.Op
		}

		return true
	})

	if op == token.ILLEGAL {
		t.Fatalf("no registration-reporting guard found in %s. The tap is what decides whether a "+
			"registration is reported once, twice or not at all", fn)
	}

	return op
}

func isRegistrationTypeComparison(cmp *ast.BinaryExpr) bool {
	lhs, isSel := cmp.X.(*ast.SelectorExpr)
	if !isSel || lhs.Sel == nil || lhs.Sel.Name != "RegistrationType5GS" {
		return false
	}

	rhs, isSel := cmp.Y.(*ast.SelectorExpr)

	return isSel && rhs.Sel != nil && rhs.Sel.Name == "RegistrationType5GSMobilityRegistrationUpdating"
}

// TestTheTwoRegistrationTapsPartitionTheTypes reads the production guards instead of restating
// them. A test that declared its own copies of the two comparisons and asserted they differ
// would pass whatever the handlers compare: both guards could be changed to the same comparison
// while every mobility registration is reported twice and the agency receives two location
// records for one movement.
//
// The property is real and worth pinning: a periodic registration update is reported exactly
// once only if the two taps partition the registration types between them. So this asserts
// what the handlers actually compare — one `==`, the other `!=`, on the same two operands — which
// is the only shape that partitions.
func TestTheTwoRegistrationTapsPartitionTheTypes(t *testing.T) {
	const file = "handler.go"

	mobility := tapGuard(t, file, "HandleMobilityAndPeriodicRegistrationUpdating")
	complete := tapGuard(t, file, "HandleRegistrationComplete")

	if mobility == complete {
		t.Fatalf("both registration taps gate on %s, so they do not partition the registration "+
			"types: a type matching both is reported twice — two location records for one "+
			"movement — and a type matching neither is never reported at all", mobility)
	}

	if mobility != token.EQL {
		t.Errorf("the mobility-and-periodic tap gates on %s, want ==: it reports the type it "+
			"handles, and the other tap reports the rest", mobility)
	}

	if complete != token.NEQ {
		t.Errorf("the registration-complete tap gates on %s, want !=: it reports every type the "+
			"other tap does not", complete)
	}
}
