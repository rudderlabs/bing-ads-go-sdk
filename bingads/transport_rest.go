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

type restGetBulkUploadURLRequest struct {
	AccountID    int64  `json:"AccountId"`
	ResponseMode string `json:"ResponseMode"`
}

type restGetBulkUploadURLResponse struct {
	RequestID string `json:"RequestId"`
	UploadURL string `json:"UploadUrl"`
}

type restGetBulkUploadStatusRequest struct {
	RequestID string `json:"RequestId"`
}

type restGetBulkUploadStatusResponse struct {
	Errors          []OperationError `json:"Errors"`
	PercentComplete int64            `json:"PercentComplete"`
	RequestStatus   string           `json:"RequestStatus"`
	ResultFileURL   string           `json:"ResultFileUrl"`
}

// restAdAPIError is AdApiError in JSON, where the detail field is named Detail.
type restAdAPIError struct {
	Code      int64  `json:"Code"`
	Detail    string `json:"Detail"`
	ErrorCode string `json:"ErrorCode"`
	Message   string `json:"Message"`
}

// restFault covers AdApiFaultDetail and ApiFaultDetail, told apart by Type.
type restFault struct {
	Type            string           `json:"Type"`
	TrackingID      string           `json:"TrackingId"`
	Errors          []restAdAPIError `json:"Errors"`
	OperationErrors []OperationError `json:"OperationErrors"`
	BatchErrors     []OperationError `json:"BatchErrors"`
}

func (t *restBulkTransport) getBulkUploadURL(accountID int64, responseMode string) (*GetBulkUploadUrlResponse, error) {
	var result restGetBulkUploadURLResponse
	if err := t.postJSON("GetBulkUploadUrl", "/Bulk/v13/BulkUploadUrl/Query",
		restGetBulkUploadURLRequest{AccountID: accountID, ResponseMode: responseMode}, &result); err != nil {
		return nil, err
	}
	return &GetBulkUploadUrlResponse{UploadUrl: result.UploadURL, RequestId: result.RequestID}, nil
}

func (t *restBulkTransport) getBulkUploadStatus(requestID string) (*GetBulkUploadStatusResponse, error) {
	var result restGetBulkUploadStatusResponse
	if err := t.postJSON("GetBulkUploadStatus", "/Bulk/v13/BulkUploadStatus/Query",
		restGetBulkUploadStatusRequest{RequestID: requestID}, &result); err != nil {
		return nil, err
	}
	return &GetBulkUploadStatusResponse{
		RequestStatus:    result.RequestStatus,
		PercentComplete:  result.PercentComplete,
		ResultFileUrl:    result.ResultFileURL,
		ProcessingErrors: result.Errors,
	}, nil
}

// postJSON sends payload to path and decodes a 2xx body into out. Other statuses return a fault error.
func (t *restBulkTransport) postJSON(operation, path string, payload, out interface{}) error {
	if t.session.TokenSource == nil {
		return fmt.Errorf("%s: REST requires a token source", operation)
	}
	token, err := t.session.TokenSource.Token()
	if err != nil {
		return err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(t.baseURL, "/")+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("DeveloperToken", t.session.DeveloperToken)
	req.Header.Set("CustomerId", t.session.CustomerId)
	req.Header.Set("CustomerAccountId", t.session.AccountId)

	resp, err := t.session.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	trackingID := resp.Header.Get("TrackingId")
	// Bodies carry signed upload and result URLs, so debug output omits them.
	if debug != "" {
		fmt.Printf("POST %s: HTTP %d (tracking ID %s)\n", req.URL.Path, resp.StatusCode, trackingID)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("%s: HTTP %d: read response body: %w", operation, resp.StatusCode, err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return faultError(operation, resp.StatusCode, trackingID, raw)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("%s: decode response: %w", operation, err)
	}
	return nil
}

// faultError builds an APIError from a documented fault body, wrapping known codes in baseError.
func faultError(operation string, status int, trackingID string, raw []byte) error {
	apiErr := &APIError{Operation: operation, StatusCode: status, TrackingID: trackingID, Body: string(raw)}
	apiErr.Errors.TrackingId = trackingID
	var fault restFault
	if json.Unmarshal(raw, &fault) != nil {
		return apiErr
	}
	if fault.TrackingID != "" {
		apiErr.TrackingID = fault.TrackingID
		apiErr.Errors.TrackingId = fault.TrackingID
	}
	var codes []string
	switch fault.Type {
	case "AdApiFaultDetail":
		for _, e := range fault.Errors {
			apiErr.Errors.AdApiErrors = append(apiErr.Errors.AdApiErrors, AdApiError{Code: e.Code, Details: e.Detail, ErrorCode: e.ErrorCode, Message: e.Message})
			codes = append(codes, e.ErrorCode)
		}
	case "ApiFaultDetail":
		apiErr.Errors.OperationErrors = append(fault.OperationErrors, fault.BatchErrors...)
		for _, e := range apiErr.Errors.OperationErrors {
			codes = append(codes, e.ErrorCode)
		}
	}
	for _, code := range codes {
		switch code {
		case "AuthenticationTokenExpired", "InvalidCredentials", "InternalError", "CallRateExceeded":
			return &baseError{code: code, origErr: apiErr}
		}
	}
	return apiErr
}
