// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package ngap

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// callsIn reports the calls inside one function of a file, closures included, in source
// order: selector calls as "x.Fn" and bare calls as "Fn".
func callsIn(t *testing.T, file, fn string) []string {
	t.Helper()

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

// HandleHandoverRequired never clears the handover state. This attempt's values are stored by
// SendHandoverRequest, past its own guards, so anything HandleHandoverRequired could clear
// belongs to a handover already in flight. A duplicate HANDOVER REQUIRED that ended on a
// preparation failure would otherwise leave that handover's acknowledge with no cause or
// container to report.
func TestAHandoverRequiredLeavesTheHandoverInFlightAlone(t *testing.T) {
	if n := count(callsIn(t, "handler.go", "HandleHandoverRequired"), "sourceUe.ClearHandoverState"); n != 0 {
		t.Errorf("HandleHandoverRequired clears the handover state %d times, want 0", n)
	}
}
