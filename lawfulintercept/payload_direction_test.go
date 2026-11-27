// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package lawfulintercept

import (
	"testing"
	"time"

	"github.com/omec-project/nas/v2/nasMessage"
	"github.com/omec-project/openapi/v2/models"
	"github.com/omec-project/util/li/iri"
	"github.com/omec-project/util/li/store"
	"github.com/omec-project/util/li/types"
	"github.com/omec-project/util/li/x2x3"
)

// TestPayloadDirectionFollowsTheInitiator: TS 33.128 table 5.3.2-1 sets an xIRI's Payload
// Direction from the initiator of the procedure it reports, so a procedure the target UE
// initiated is "sent from the target" (3) and one the network initiated is "sent to the
// target" (2). The start-of-interception and identifier association records are "not
// applicable" (5), as their clauses say.
func TestPayloadDirectionFollowsTheInitiator(t *testing.T) {
	for _, c := range []struct {
		name   string
		report func(*subsystem)
		want   x2x3.PayloadDirection
	}{
		{"registration", func(*subsystem) { ReportRegistration(targetUE(), models.ACCESSTYPE__3_GPP_ACCESS) }, x2x3.DirectionFromTarget},
		{"service accept", func(*subsystem) { ReportServiceAccept(targetUE(), nasMessage.ServiceTypeData) }, x2x3.DirectionFromTarget},
		{"UE-initiated deregistration", func(*subsystem) {
			ReportDeregistration(targetUE(), false, iri.AccessThreeGPP, nil)
		}, x2x3.DirectionFromTarget},
		{"network-initiated deregistration", func(*subsystem) {
			ReportDeregistration(targetUE(), true, iri.AccessThreeGPP, nil)
		}, x2x3.DirectionToTarget},
		{"registration reject", func(*subsystem) {
			ue := targetUE()
			ue.SecurityContextAvailable = true
			ReportRegistrationReject(ue, 22)
		}, x2x3.DirectionFromTarget},
		{"handover command", func(*subsystem) { ReportHandoverCommand(sampleHandover()) }, x2x3.DirectionToTarget},
		{"handover request", func(*subsystem) { ReportHandoverRequest(sampleHandover()) }, x2x3.DirectionToTarget},
		{"UE policy transfer", func(*subsystem) {
			ue := targetUE()
			NoteUEPolicyCommand(ue, uePolicyCommandOf(7, 32))
			ReportUEPolicyComplete(ue, uePolicyAnswer(7, uePolicyComplete))
		}, x2x3.DirectionToTarget},
		{"location update", func(*subsystem) { ReportLocationUpdate(targetUE()) }, x2x3.DirectionToTarget},
		{"identifier association", func(*subsystem) { ReportIdentifierAssociation(targetUE()) }, x2x3.DirectionNotApplicable},
		{"identifier deassociation", func(*subsystem) { ReportIdentifierDeassociation(targetUE()) }, x2x3.DirectionNotApplicable},
		{"start of interception", func(s *subsystem) {
			registeredUE(t, testTargetSUPI)
			task, _ := s.store.Get(testSubstitutionXID)
			s.scanRegisteredUEs(task, nil, time.Now())
		}, x2x3.DirectionNotApplicable},
	} {
		t.Run(c.name, func(t *testing.T) {
			snd := &captureSender{}
			st := store.New()
			st.Activate(types.InterceptTask{
				XID:         testSubstitutionXID,
				Targets:     []types.TargetIdentifier{{Type: types.TargetSUPI, Value: testTargetSUPI}},
				Products:    []types.ProductType{types.ProductIRI},
				State:       types.TaskActive,
				RecordScope: types.RecordScopeAll,
			})
			sub := &subsystem{
				store: st, senderFor: func(string) sender { return snd },
				mdf2: testAgencyAddr,
				ids:  x2x3.NewIdentity(testNEID, amfInterceptionPoint),
			}
			active.Store(sub)
			t.Cleanup(func() { active.Store(nil) })

			c.report(sub)

			if len(snd.pdus) != 1 {
				t.Fatalf("delivered %d PDUs, want 1", len(snd.pdus))
			}
			if got := snd.pdus[0].Direction; got != c.want {
				t.Errorf("Payload Direction = %d, want %d", got, c.want)
			}
		})
	}
}

// TestAMobilityLocationUpdateIsFromTheTarget: a task scoped to IdentifierAssociation receives an
// AMFLocationUpdate in place of a mobility registration's AMFRegistration. It reports the same
// UE-initiated registration procedure, so its Payload Direction is "sent from the target" (3).
func TestAMobilityLocationUpdateIsFromTheTarget(t *testing.T) {
	snd := &captureSender{}
	st := store.New()
	st.Activate(types.InterceptTask{
		XID:         testSubstitutionXID,
		Targets:     []types.TargetIdentifier{{Type: types.TargetSUPI, Value: testTargetSUPI}},
		Products:    []types.ProductType{types.ProductIRI},
		State:       types.TaskActive,
		RecordScope: types.RecordScopeIdentifierAssociation,
	})
	active.Store(&subsystem{
		store: st, senderFor: func(string) sender { return snd },
		mdf2: testAgencyAddr,
		ids:  x2x3.NewIdentity(testNEID, amfInterceptionPoint),
	})
	t.Cleanup(func() { active.Store(nil) })

	ue := targetUE()
	ue.RegistrationType5GS = nasMessage.RegistrationType5GSMobilityRegistrationUpdating
	ReportRegistration(ue, models.ACCESSTYPE__3_GPP_ACCESS)

	if len(snd.pdus) != 1 {
		t.Fatalf("delivered %d PDUs, want the one AMFLocationUpdate", len(snd.pdus))
	}
	if got := snd.pdus[0].Direction; got != x2x3.DirectionFromTarget {
		t.Errorf("Payload Direction = %d, want %d", got, x2x3.DirectionFromTarget)
	}
}
