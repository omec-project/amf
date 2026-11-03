// Copyright (C) 2026 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package gmm

import (
	ctxt "context"
	"strings"
	"testing"

	"github.com/omec-project/amf/context"
	"github.com/omec-project/nas/v2/nasMessage"
	"github.com/omec-project/openapi/v2/models"
	"github.com/omec-project/util/fsm"
	"go.uber.org/zap"
)

// newAuthenticationUe returns a UE in the Authentication state whose authentication context has no
// _links. Any attempt to confirm with the AUSF therefore fails with an error instead of succeeding, so a
// nil error from the handlers shows that no AUSF request was built.
func newAuthenticationUe(authType models.AuthType) *context.AmfUe {
	authCtx := models.NewUEAuthenticationCtxWithDefaults()
	authCtx.SetAuthType(authType)
	av5gAka := models.NewAv5gAkaWithDefaults()
	av5gAka.SetRand("00112233445566778899aabbccddeeff")
	av5gAka.SetAutn("00112233445566778899aabbccddeeff")
	authCtx.Var5gAuthData = models.UEAuthenticationCtx5gAuthData{
		Av5gAka: av5gAka,
	}
	ue := &context.AmfUe{
		GmmLog: zap.NewNop().Sugar(),
		NASLog: zap.NewNop().Sugar(),
		RanUe:  make(map[models.AccessType]*context.RanUe),
		State:  make(map[models.AccessType]*fsm.State),
		OnGoing: map[models.AccessType]*context.OnGoingProcedureWithPrio{
			models.ACCESSTYPE__3_GPP_ACCESS: {},
		},
		AuthenticationCtx: authCtx,
	}
	ue.State[models.ACCESSTYPE__3_GPP_ACCESS] = fsm.NewState(context.Authentication)
	return ue
}

func TestHandleAuthenticationResponseMissingIEs(t *testing.T) {
	tests := []struct {
		name     string
		authType models.AuthType
	}{
		{"5G AKA without Authentication Response Parameter", models.AUTHTYPE__5_G_AKA},
		{"EAP-AKA' without EAP message", models.AUTHTYPE_EAP_AKA_PRIME},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ue := newAuthenticationUe(tt.authType)
			response := nasMessage.NewAuthenticationResponse(0)

			if err := HandleAuthenticationResponse(ctxt.Background(), ue, models.ACCESSTYPE__3_GPP_ACCESS, response); err != nil {
				t.Fatalf("expected the message to be rejected without error, got %v", err)
			}
			if got := ue.State[models.ACCESSTYPE__3_GPP_ACCESS].Current(); got != context.Deregistered {
				t.Fatalf("state = %v, want Deregistered", got)
			}
		})
	}
}

func TestHandleAuthenticationResponseNilAuthenticationCtx(t *testing.T) {
	ue := newAuthenticationUe(models.AUTHTYPE__5_G_AKA)
	ue.AuthenticationCtx = nil

	err := HandleAuthenticationResponse(ctxt.Background(), ue, models.ACCESSTYPE__3_GPP_ACCESS,
		nasMessage.NewAuthenticationResponse(0))
	if err == nil || !strings.Contains(err.Error(), "authentication context is nil") {
		t.Fatalf("expected nil authentication context error, got %v", err)
	}
	if got := ue.State[models.ACCESSTYPE__3_GPP_ACCESS].Current(); got != context.Authentication {
		t.Fatalf("state = %v, want Authentication", got)
	}
}

func TestHandleAuthenticationFailureSynchFailureWithoutAuts(t *testing.T) {
	ue := newAuthenticationUe(models.AUTHTYPE__5_G_AKA)
	failure := nasMessage.NewAuthenticationFailure(0)
	failure.SetCauseValue(nasMessage.Cause5GMMSynchFailure)

	// With the AUTS missing no resynchronisation request may reach the AUSF; the UE is rejected instead.
	if err := HandleAuthenticationFailure(ctxt.Background(), ue, models.ACCESSTYPE__3_GPP_ACCESS, failure); err != nil {
		t.Fatalf("expected the message to be rejected without error, got %v", err)
	}
	if got := ue.State[models.ACCESSTYPE__3_GPP_ACCESS].Current(); got != context.Deregistered {
		t.Fatalf("state = %v, want Deregistered", got)
	}
}

func TestHandleAuthenticationFailureNilAuthenticationCtx(t *testing.T) {
	ue := newAuthenticationUe(models.AUTHTYPE__5_G_AKA)
	ue.AuthenticationCtx = nil
	failure := nasMessage.NewAuthenticationFailure(0)
	failure.SetCauseValue(nasMessage.Cause5GMMMACFailure)

	err := HandleAuthenticationFailure(ctxt.Background(), ue, models.ACCESSTYPE__3_GPP_ACCESS, failure)
	if err == nil || !strings.Contains(err.Error(), "authentication context is nil") {
		t.Fatalf("expected nil authentication context error, got %v", err)
	}
	if got := ue.State[models.ACCESSTYPE__3_GPP_ACCESS].Current(); got != context.Authentication {
		t.Fatalf("state = %v, want Authentication", got)
	}
}
