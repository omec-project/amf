// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package gmm

import (
	ctxt "context"
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
	"time"

	"github.com/omec-project/amf/context"
	"github.com/omec-project/nas/v2/nasMessage"
	"github.com/omec-project/openapi/v2/models"
	"github.com/omec-project/util/fsm"
	"go.uber.org/zap"
)

// TestRegistrationCompleteReportsEveryRegistrationType: TS 33.128 clause 6.2.2.2.2 reports
// every successful registration as an AMFRegistration when the access becomes
// 5GMM-REGISTERED after the REGISTRATION ACCEPT, mobility registration updating included.
// REGISTRATION COMPLETE is that point, so its handler reports each type once.
func TestRegistrationCompleteReportsEveryRegistrationType(t *testing.T) {
	for _, rt := range []uint8{
		nasMessage.RegistrationType5GSInitialRegistration,
		nasMessage.RegistrationType5GSMobilityRegistrationUpdating,
		nasMessage.RegistrationType5GSPeriodicRegistrationUpdating,
		nasMessage.RegistrationType5GSEmergencyRegistration,
	} {
		called := 0
		restore := reportRegistration
		reportRegistration = func(*context.AmfUe, models.AccessType) { called++ }

		req := nasMessage.NewRegistrationRequest(0)
		req.SetFOR(nasMessage.FollowOnRequestPending)
		ue := &context.AmfUe{
			GmmLog:              zap.NewNop().Sugar(),
			RegistrationType5GS: rt,
			RegistrationRequest: req,
			// A state with no transition for the success event: the test is about the
			// report, which comes before it.
			State: map[models.AccessType]*fsm.State{
				models.ACCESSTYPE__3_GPP_ACCESS: fsm.NewState(context.Deregistered),
			},
		}
		//nolint:errcheck // the state has no transition for the success event; the report precedes it
		_ = HandleRegistrationComplete(ctxt.Background(), ue, models.ACCESSTYPE__3_GPP_ACCESS,
			nasMessage.NewRegistrationComplete(0))
		reportRegistration = restore

		if called != 1 {
			t.Errorf("registration type %d reported %d times at REGISTRATION COMPLETE, want 1", rt, called)
		}
	}
}

// TestOnlyRegistrationCompleteReportsARegistration: the mobility and periodic handler sends
// the REGISTRATION ACCEPT, and on some paths returns right after; the access is not yet
// registered there, so no registration is reported from it.
func TestOnlyRegistrationCompleteReportsARegistration(t *testing.T) {
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, "handler.go", nil, 0)
	if err != nil {
		t.Fatalf("parsing handler.go: %v", err)
	}
	for _, decl := range parsed.Decls {
		fd, isFunc := decl.(*ast.FuncDecl)
		if !isFunc {
			continue
		}
		ast.Inspect(fd, func(n ast.Node) bool {
			call, isCall := n.(*ast.CallExpr)
			if !isCall {
				return true
			}
			var name string
			switch fun := call.Fun.(type) {
			case *ast.Ident:
				name = fun.Name
			case *ast.SelectorExpr:
				name = fun.Sel.Name
			}
			if (name == "reportRegistration" || name == "ReportRegistration") &&
				fd.Name.Name != "HandleRegistrationComplete" {
				t.Errorf("%s reports a registration at %s; only HandleRegistrationComplete does",
					fd.Name.Name, fset.Position(call.Pos()))
			}
			return true
		})
	}
}

// TestRegistrationCompleteStopsT3550BeforeReporting: T3550 running out also reports the
// registration, so REGISTRATION COMPLETE stops it before its own report; otherwise a last
// expiry between the two would report the registration a second time.
func TestRegistrationCompleteStopsT3550BeforeReporting(t *testing.T) {
	var runningAtReport bool
	restore := reportRegistration
	t.Cleanup(func() { reportRegistration = restore })

	req := nasMessage.NewRegistrationRequest(0)
	req.SetFOR(nasMessage.FollowOnRequestPending)
	ue := &context.AmfUe{
		GmmLog:              zap.NewNop().Sugar(),
		RegistrationRequest: req,
		State: map[models.AccessType]*fsm.State{
			models.ACCESSTYPE__3_GPP_ACCESS: fsm.NewState(context.Deregistered),
		},
		T3550: context.NewTimer(time.Hour, 0, func(int32) {}, func() {}),
	}
	reportRegistration = func(ue *context.AmfUe, _ models.AccessType) { runningAtReport = ue.T3550 != nil }

	//nolint:errcheck // the state has no transition for the success event; the report precedes it
	_ = HandleRegistrationComplete(ctxt.Background(), ue, models.ACCESSTYPE__3_GPP_ACCESS,
		nasMessage.NewRegistrationComplete(0))

	if runningAtReport {
		t.Error("the registration was reported while T3550 could still report it")
	}
	if ue.T3550 != nil {
		ue.T3550.Stop()
	}
}
