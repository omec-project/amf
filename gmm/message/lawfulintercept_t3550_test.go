// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package message

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
	"time"

	"github.com/omec-project/amf/context"
	"github.com/omec-project/openapi/v2/models"
	"github.com/omec-project/util/fsm"
	"go.uber.org/zap"
)

// TestT3550RunningOutReportsOnlyAProcedureStillWaiting: when the UE never answers the
// REGISTRATION ACCEPT, T3550's last expiry sets the access 5GMM-REGISTERED (TS 24.501 clause
// 5.5.1.2.8 case c), which is the trigger of TS 33.128 clause 6.2.2.2.2, so the registration is
// reported there, once. Only while that procedure is still waiting: a failure that dropped the
// access to Deregistered, a REGISTRATION COMPLETE, or a later accept with its own T3550 has
// ended or replaced it, and a record then would report a registration that was torn down.
//
// The expiry's serialized work is invoked directly: the real timer writes on its own goroutine,
// which a test driving it would race.
func TestT3550RunningOutReportsOnlyAProcedureStillWaiting(t *testing.T) {
	const access = models.ACCESSTYPE__3_GPP_ACCESS
	reported := 0
	original := liReportRegistration
	liReportRegistration = func(*context.AmfUe, models.AccessType) { reported++ }
	t.Cleanup(func() { liReportRegistration = original })

	this := context.NewTimer(time.Hour, 0, func(int32) {}, func() {})
	other := context.NewTimer(time.Hour, 0, func(int32) {}, func() {})
	t.Cleanup(func() { this.Stop(); other.Stop() })

	for _, c := range []struct {
		name   string
		state  fsm.StateType
		t3550  *context.Timer
		report bool
	}{
		{"still waiting for the answer", context.ContextSetup, this, true},
		// A release of the RAN leg stops the retransmissions and leaves T3550 running.
		{"a late expiry of a T3550 already stopped", context.ContextSetup, nil, false},
		{"a context setup failure ended the procedure", context.Deregistered, nil, false},
		{"REGISTRATION COMPLETE arrived", context.Registered, nil, false},
		{"a later accept runs its own T3550", context.ContextSetup, other, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			reported = 0
			ue := &context.AmfUe{
				GmmLog: zap.NewNop().Sugar(),
				State:  map[models.AccessType]*fsm.State{access: fsm.NewState(c.state)},
				OnGoing: map[models.AccessType]*context.OnGoingProcedureWithPrio{
					access: {Procedure: context.OnGoingProcedureRegistration},
				},
				T3550: c.t3550,
			}

			registrationAcceptUnanswered(ue, access, this)

			if want := map[bool]int{true: 1, false: 0}[c.report]; reported != want {
				t.Errorf("reported the registration %d times, want %d", reported, want)
			}
			if registered := ue.State[access].Is(context.Registered); c.report && !registered {
				t.Error("the access was not set Registered")
			} else if !c.report && c.state != context.Registered && registered {
				t.Errorf("the access went from %s to Registered", c.state)
			}
		})
	}
}

// The expiry reaches that check, on the UE's NAS goroutine: SendRegistrationAccept's T3550 hands
// its last expiry to registrationAcceptUnanswered inside ue.RunSerialized and reports nothing
// itself, and no expiry clears ue.T3550 off that goroutine, which would let a timer still
// running pass for one that was stopped.
func TestT3550RunningOutGoesThroughTheCheck(t *testing.T) {
	parsed, err := parser.ParseFile(token.NewFileSet(), "send.go", nil, 0)
	if err != nil {
		t.Fatalf("parsing send.go: %v", err)
	}
	var fn *ast.FuncDecl
	for _, d := range parsed.Decls {
		if fd, ok := d.(*ast.FuncDecl); ok && fd.Name.Name == "SendRegistrationAccept" {
			fn = fd
		}
	}
	if fn == nil {
		t.Fatal("no SendRegistrationAccept in send.go")
	}

	serialized, unserialized := 0, 0
	ast.Inspect(fn, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.CallExpr:
			if sel, ok := n.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "RunSerialized" {
				ast.Inspect(n, func(m ast.Node) bool {
					if c, ok := m.(*ast.CallExpr); ok {
						if id, ok := c.Fun.(*ast.Ident); ok && id.Name == "registrationAcceptUnanswered" {
							serialized++
						}
					}
					return true
				})
				return false
			}
			if id, ok := n.Fun.(*ast.Ident); ok &&
				(id.Name == "registrationAcceptUnanswered" || id.Name == "liReportRegistration") {
				unserialized++
			}
		case *ast.AssignStmt:
			for i, lhs := range n.Lhs {
				sel, ok := lhs.(*ast.SelectorExpr)
				if !ok || sel.Sel.Name != "T3550" {
					continue
				}
				if id, ok := n.Rhs[i].(*ast.Ident); ok && id.Name == "nil" {
					t.Error("SendRegistrationAccept clears ue.T3550 off the UE's NAS goroutine")
				}
			}
		}
		return true
	})
	if serialized != 1 || unserialized != 0 {
		t.Errorf("T3550's expiry reaches registrationAcceptUnanswered %d times through RunSerialized "+
			"and reports or checks %d times outside it, want 1 and 0", serialized, unserialized)
	}
}
