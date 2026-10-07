// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
//
// SPDX-License-Identifier: Apache-2.0

package gmm

import (
	ctxt "context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/omec-project/amf/context"
	"github.com/omec-project/nas/v2/nasMessage"
	"github.com/omec-project/openapi/v2/models"
	"github.com/omec-project/openapi/v2/utils"
)

// The cause tests stub the slice fetch. This one runs the real fetch, through NRF
// discovery and the consumer's HTTP call, against a UDM that answers each way. The
// classification can be right on its own and the cause still wrong, if the consumer
// hands it something other than what the UDM sent.
func TestAnEmptyAllowedNssaiIsCause7OnlyWhenTheUdmAnswered_OverHTTP(t *testing.T) {
	tests := []struct {
		name      string
		answer    *udmAnswer // nil: no UDM is listening
		wantCause uint8
	}{
		{
			// The body the UDM sends when the subscription holds no NSSAI.
			name:      "the UDM answered 404: the subscriber has no slice data",
			answer:    problemAnswer(t, utils.ProblemDetailsDataNotFound()),
			wantCause: nasMessage.Cause5GMM5GSServicesNotAllowed,
		},
		{
			// The status line answers even when the problem leaves its status out.
			name: "the UDM answered 404 with a problem that has no status",
			answer: &udmAnswer{
				status:      http.StatusNotFound,
				contentType: "application/problem+json",
				body:        `{"title":"Data not found","cause":"DATA_NOT_FOUND"}`,
			},
			wantCause: nasMessage.Cause5GMM5GSServicesNotAllowed,
		},
		{
			name:      "the UDM answered 404 with no body",
			answer:    &udmAnswer{status: http.StatusNotFound},
			wantCause: nasMessage.Cause5GMM5GSServicesNotAllowed,
		},
		{
			name: "the UDM answered 404 with a body that is not a problem",
			answer: &udmAnswer{
				status:      http.StatusNotFound,
				contentType: "text/plain; charset=utf-8",
				body:        "404 page not found",
			},
			wantCause: nasMessage.Cause5GMM5GSServicesNotAllowed,
		},
		{
			// About the AMF's request, not the subscriber.
			name:      "the UDM answered 403",
			answer:    problemAnswer(t, utils.ProblemDetails("Forbidden", http.StatusForbidden, "")),
			wantCause: nasMessage.Cause5GMMCongestion,
		},
		{
			name:      "the UDM answered 503",
			answer:    problemAnswer(t, utils.ProblemDetails("Service unavailable", http.StatusServiceUnavailable, "")),
			wantCause: nasMessage.Cause5GMMCongestion,
		},
		{
			name:      "the UDM answered 503 with no body",
			answer:    &udmAnswer{status: http.StatusServiceUnavailable},
			wantCause: nasMessage.Cause5GMMCongestion,
		},
		{
			name:      "no UDM is listening",
			wantCause: nasMessage.Cause5GMMCongestion,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			disableKafkaForRelayTest(t)
			udmUri := udmAnswering(t, tt.answer)
			nrfDiscovering(t, udmUri)

			originalNssai := handleRequestedNssaiForRegistration
			t.Cleanup(func() { handleRequestedNssaiForRegistration = originalNssai })
			// Leaves the allowed NSSAI empty, which is where every case arrives.
			handleRequestedNssaiForRegistration = func(
				ctxt.Context, *context.AmfUe, *nasMessage.RegistrationRequest, models.AccessType,
			) error {
				return nil
			}
			cause := captureRejectCause(t)

			if err := HandleInitialRegistration(ctxt.Background(), pooledRejectTestUe(t),
				models.ACCESSTYPE__3_GPP_ACCESS); err == nil {
				t.Fatal("HandleInitialRegistration succeeded with an empty allowed NSSAI")
			}

			if *cause == nil {
				t.Fatal("no registration reject was sent for an empty allowed NSSAI")
			}
			if **cause != tt.wantCause {
				t.Errorf("reject cause = %d, want %d", **cause, tt.wantCause)
			}
		})
	}
}

// udmAnswer is what the UDM sends back: a status line and, if contentType is set, a body.
type udmAnswer struct {
	status      int
	contentType string
	body        string
}

// problemAnswer is problem sent as the UDM sends one, under its own status.
func problemAnswer(t *testing.T, problem *models.ProblemDetails) *udmAnswer {
	t.Helper()

	body, err := json.Marshal(problem)
	if err != nil {
		t.Fatalf("encoding the UDM's problem: %v", err)
	}

	return &udmAnswer{
		status:      int(problem.GetStatus()),
		contentType: "application/problem+json",
		body:        string(body),
	}
}

// udmAnswering returns the URI of a UDM that answers every request with answer, or, for a
// nil answer, of one that is no longer listening.
func udmAnswering(t *testing.T, answer *udmAnswer) string {
	t.Helper()

	udm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if answer.contentType != "" {
			w.Header().Set("Content-Type", answer.contentType)
		}
		w.WriteHeader(answer.status)
		if _, err := w.Write([]byte(answer.body)); err != nil {
			t.Errorf("writing the UDM's answer: %v", err)
		}
	}))
	if answer == nil {
		udm.Close()
	} else {
		t.Cleanup(udm.Close)
	}

	return udm.URL
}

// nrfDiscovering points the AMF at an NRF whose discovery finds one UDM, serving Nudm_SDM
// at udmUri. The NRF answers nothing else, so the AMF's status subscription to that UDM
// fails, which it only logs.
func nrfDiscovering(t *testing.T, udmUri string) {
	t.Helper()

	sdm := models.NewNFService("sdm", models.SERVICENAME_NUDM_SDM,
		[]models.NFServiceVersion{*models.NewNFServiceVersion("v2", "2.0.0")},
		models.URISCHEME_HTTP, models.NFSERVICESTATUS_REGISTERED)
	sdm.SetApiPrefix(udmUri)
	profile := models.NewNFProfileDiscovery("udm-instance", models.NFTYPE_UDM, models.NFSTATUS_REGISTERED)
	profile.SetNfServiceList(map[string]models.NFService{"sdm": *sdm})
	result, err := json.Marshal(models.NewSearchResult(300, []models.NFProfileDiscovery{*profile}))
	if err != nil {
		t.Fatalf("encoding the NRF's search result: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /nnrf-disc/v1/nf-instances", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write(result); err != nil {
			t.Errorf("writing the NRF's search result: %v", err)
		}
	})
	nrf := httptest.NewServer(mux)
	t.Cleanup(nrf.Close)

	self := context.AMF_Self()
	originalNrfUri, originalCaching := self.NrfUri, self.EnableNrfCaching
	t.Cleanup(func() { self.NrfUri, self.EnableNrfCaching = originalNrfUri, originalCaching })
	self.NrfUri, self.EnableNrfCaching = nrf.URL, false
}
