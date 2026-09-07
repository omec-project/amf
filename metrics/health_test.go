// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthHandlerRefusesWhenTheCheckFails(t *testing.T) {
	handler := HealthHandler(func() (bool, string) { return false, "every NGAP association has been lost" })

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/healthz", nil))

	if recorder.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}

	// The reason travels in the body because a probe failure is otherwise a bare 503 in
	// the kubelet's events, with nothing saying which condition fired.
	if !strings.Contains(recorder.Body.String(), "every NGAP association has been lost") {
		t.Errorf("body = %q, want it to carry the check's reason", recorder.Body.String())
	}
}

func TestHealthHandlerAcceptsWhenTheCheckPasses(t *testing.T) {
	handler := HealthHandler(func() (bool, string) { return true, "serving" })

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/healthz", nil))

	if recorder.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", recorder.Code, http.StatusOK)
	}

	if !strings.Contains(recorder.Body.String(), "serving") {
		t.Errorf("body = %q, want it to carry the check's reason", recorder.Body.String())
	}
}
