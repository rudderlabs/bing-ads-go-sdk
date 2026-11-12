package bingads

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type restBulkTransport struct {
	session *Session
	baseURL string
}

type restError struct {
	Code      int64  `json:"Code"`
	Details   string `json:"Details"`
	ErrorCode string `json:"ErrorCode"`
	Message   string `json:"Message"`
}

type restFault struct {
	Type            string      `json:"Type"`
	TrackingID      string      `json:"TrackingId"`
	Errors          []restError `json:"Errors"`
	OperationErrors []restError `json:"OperationErrors"`
	BatchErrors     []restError `json:"BatchErrors"`
}

func (t *restBulkTransport) getBulkUploadURL(accountID int64, responseMode string) (*GetBulkUploadUrlResponse, error) {
	body, _, err := t.query("GetBulkUploadUrl", "/Bulk/v13/BulkUploadUrl/Query", struct {
		AccountID    int64  `json:"AccountId"`
		ResponseMode string `json:"ResponseMode"`
	}{accountID, responseMode})
	if err != nil {
		return nil, err
	}
	var result struct {
		UploadURL string `json:"UploadUrl"`
		RequestID string `json:"RequestId"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	return &GetBulkUploadUrlResponse{UploadUrl: result.UploadURL, RequestId: result.RequestID}, nil
}

func (t *restBulkTransport) getBulkUploadStatus(requestID string) (*GetBulkUploadStatusResponse, error) {
	body, trackingID, err := t.query("GetBulkUploadStatus", "/Bulk/v13/BulkUploadStatus/Query", struct {
		RequestID string `json:"RequestId"`
	}{requestID})
	if err != nil {
		return nil, err
	}
	var result struct {
		RequestStatus   string      `json:"RequestStatus"`
		PercentComplete int64       `json:"PercentComplete"`
		ResultFileURL   string      `json:"ResultFileUrl"`
		Errors          []restError `json:"Errors"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	return &GetBulkUploadStatusResponse{
		RequestStatus:   result.RequestStatus,
		PercentComplete: result.PercentComplete,
		ResultFileUrl:   result.ResultFileURL,
		Errors: ErrorsType{
			TrackingId:      trackingID,
			OperationErrors: operationErrors(result.Errors),
		},
	}, nil
}

func (t *restBulkTransport) query(operation, path string, payload interface{}) ([]byte, string, error) {
	if t.session.TokenSource == nil {
		return nil, "", fmt.Errorf("%s: REST requires a token source", operation)
	}
	token, err := t.session.TokenSource.Token()
	if err != nil {
		return nil, "", err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, "", err
	}
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(t.baseURL, "/")+path, bytes.NewReader(body))
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("DeveloperToken", t.session.DeveloperToken)
	req.Header.Set("CustomerId", t.session.CustomerId)
	req.Header.Set("CustomerAccountId", t.session.AccountId)

	resp, err := t.session.HTTPClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	trackingID := resp.Header.Get("TrackingId")
	if debug != "" {
		logBody, logTrackingID := string(raw), trackingID
		if token.AccessToken != "" {
			logBody = strings.ReplaceAll(logBody, token.AccessToken, "[REDACTED]")
			logTrackingID = strings.ReplaceAll(logTrackingID, token.AccessToken, "[REDACTED]")
		}
		fmt.Printf("POST %s: HTTP %d (tracking ID %s): %s\n", req.URL.Path, resp.StatusCode, logTrackingID, logBody)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		apiErr := &APIError{Operation: operation, StatusCode: resp.StatusCode, TrackingID: trackingID, Body: string(raw)}
		fault := parseRESTFault(raw)
		if fault.TrackingID != "" {
			apiErr.TrackingID = fault.TrackingID
		}
		apiErr.Errors = ErrorsType{
			TrackingId:      apiErr.TrackingID,
			AdApiErrors:     adAPIErrorList(fault.Errors),
			OperationErrors: operationErrors(append(fault.OperationErrors, fault.BatchErrors...)),
		}
		for _, e := range append(append(fault.Errors, fault.OperationErrors...), fault.BatchErrors...) {
			switch e.ErrorCode {
			case "AuthenticationTokenExpired", "InvalidCredentials", "InternalError", "CallRateExceeded":
				return nil, "", &baseError{code: e.ErrorCode, origErr: apiErr}
			}
		}
		return nil, "", apiErr
	}
	return raw, trackingID, nil
}

func parseRESTFault(body []byte) restFault {
	var fields map[string]json.RawMessage
	if json.Unmarshal(body, &fields) != nil {
		return restFault{}
	}
	var fault restFault
	_ = json.Unmarshal(body, &fault)
	for _, value := range fields {
		var nested map[string]json.RawMessage
		if json.Unmarshal(value, &nested) != nil || !hasFaultFields(nested) {
			continue
		}
		var inner restFault
		if json.Unmarshal(value, &inner) != nil {
			continue
		}
		if fault.Type == "" {
			fault.Type = inner.Type
		}
		if fault.TrackingID == "" {
			fault.TrackingID = inner.TrackingID
		}
		fault.Errors = append(fault.Errors, inner.Errors...)
		fault.OperationErrors = append(fault.OperationErrors, inner.OperationErrors...)
		fault.BatchErrors = append(fault.BatchErrors, inner.BatchErrors...)
	}
	return fault
}

func hasFaultFields(fields map[string]json.RawMessage) bool {
	for _, key := range []string{"Type", "TrackingId", "Errors", "OperationErrors", "BatchErrors"} {
		if _, ok := fields[key]; ok {
			return true
		}
	}
	return false
}

func adAPIErrorList(errors []restError) []AdApiError {
	var result []AdApiError
	for _, e := range errors {
		result = append(result, AdApiError(e))
	}
	return result
}

func operationErrors(errors []restError) []OperationError {
	var result []OperationError
	for _, e := range errors {
		result = append(result, OperationError(e))
	}
	return result
}
