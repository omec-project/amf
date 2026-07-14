// Copyright (c) 2026 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package oam

import (
	"net/http"
	"strings"
	"testing"

	"github.com/omec-project/openapi/v2/models"
	openapiUtils "github.com/omec-project/openapi/v2/utils"
)

func TestPurgeUEContextProblemDetailsResponse(t *testing.T) {
	t.Run("pointer problem details preserves status", func(t *testing.T) {
		problemDetails := openapiUtils.ProblemDetailsContextNotFound("")

		status, body := purgeUEContextProblemDetailsResponse(problemDetails)
		if status != http.StatusNotFound {
			t.Fatalf("status = %d, want %d", status, http.StatusNotFound)
		}
		returned, ok := body.(*models.ProblemDetails)
		if !ok {
			t.Fatalf("body type = %T, want *models.ProblemDetails", body)
		}
		if returned.GetCause() != "CONTEXT_NOT_FOUND" {
			t.Fatalf("cause = %q, want %q", returned.GetCause(), "CONTEXT_NOT_FOUND")
		}
	})

	t.Run("unexpected type returns fallback problem details", func(t *testing.T) {
		status, body := purgeUEContextProblemDetailsResponse("unexpected")
		if status != http.StatusInternalServerError {
			t.Fatalf("status = %d, want %d", status, http.StatusInternalServerError)
		}
		problemDetails, ok := body.(*models.ProblemDetails)
		if !ok {
			t.Fatalf("body type = %T, want *models.ProblemDetails", body)
		}
		if problemDetails.GetCause() != "SYSTEM_FAILURE" {
			t.Fatalf("cause = %q, want %q", problemDetails.GetCause(), "SYSTEM_FAILURE")
		}
		if !strings.Contains(problemDetails.GetDetail(), "unexpected ProblemDetails type string") {
			t.Fatalf("detail = %q, want unexpected type message", problemDetails.GetDetail())
		}
	})
}
