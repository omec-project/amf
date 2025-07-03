// Copyright 2019 Communication Service/Software Laboratory, National Chiao Tung University (free5gc.org)
//
// SPDX-License-Identifier: Apache-2.0

package openapi

import (
	"net/http"

	"github.com/omec-project/openapi/models"
)

func ProblemDetailsSystemFailure(detail string) *models.ProblemDetails {
	return &models.ProblemDetails{
		Title:  "System failure",
		Status: http.StatusInternalServerError,
		Detail: detail,
		Cause:  "SYSTEM_FAILURE",
	}
}

func ProblemDetailsMalformedReqSyntax(detail string) *models.ProblemDetails {
	return &models.ProblemDetails{
		Title:  "Malformed request syntax",
		Status: http.StatusBadRequest,
		Detail: detail,
	}
}

func ProblemDetailsDataNotFound(detail string) *models.ProblemDetails {
	return &models.ProblemDetails{
		Title:  "Data not found",
		Status: http.StatusNotFound,
		Detail: detail,
	}
}
