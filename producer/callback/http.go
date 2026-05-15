// Copyright (c) 2026 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package callback

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/omec-project/amf/logger"
	"github.com/omec-project/openapi/v2"
)

const callbackHTTPTimeout = 30 * time.Second

var callbackHTTPClient = &http.Client{Timeout: callbackHTTPTimeout}

func postCallbackJSON(ctx context.Context, callbackURI string, payload any) (*http.Response, error) {
	body, err := openapi.SetBody(payload, "application/json")
	if err != nil {
		return nil, err
	}
	return postCallbackRequest(ctx, callbackURI, "application/json", body)
}

func postCallbackMultipart(ctx context.Context, callbackURI string, payload any) (*http.Response, error) {
	body := &bytes.Buffer{}
	contentType, err := openapi.MultipartEncode(payload, body)
	if err != nil {
		return nil, err
	}
	return postCallbackRequest(ctx, callbackURI, contentType, body)
}

func postCallbackRequest(ctx context.Context, callbackURI string, contentType string, body *bytes.Buffer) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, callbackURI, bytes.NewReader(body.Bytes()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "application/json, application/problem+json")
	return callbackHTTPClient.Do(req)
}

func logCallbackResponseError(httpResponse *http.Response, err error) {
	if err != nil {
		if httpResponse == nil {
			logger.HttpLog.Errorln(err.Error())
		} else if err.Error() != httpResponse.Status {
			logger.HttpLog.Errorln(err.Error())
		}
		return
	}
	if httpResponse != nil && httpResponse.StatusCode >= http.StatusMultipleChoices {
		logger.HttpLog.Errorln(fmt.Errorf("callback request failed: %s", httpResponse.Status))
	}
}

func closeCallbackResponseBody(httpResponse *http.Response) {
	if httpResponse == nil || httpResponse.Body == nil {
		return
	}
	if _, err := io.Copy(io.Discard, httpResponse.Body); err != nil {
		logger.HttpLog.Errorln(err)
	}
	if err := httpResponse.Body.Close(); err != nil {
		logger.HttpLog.Errorln(err)
	}
}
