package bingads

import (
	"encoding/xml"
	"fmt"
	"strings"
)

// APIError describes a failed REST bulk API request.
type APIError struct {
	Operation  string
	StatusCode int
	TrackingID string
	Errors     ErrorsType
	Body       string
}

func (e *APIError) Error() string {
	message := e.Errors.Error()
	if strings.TrimSpace(message) == "" {
		message = e.Body
		if len(message) > 2048 {
			message = message[:2048]
		}
		if message == "" {
			message = "empty response body"
		}
	}
	if e.TrackingID != "" {
		return fmt.Sprintf("%s: HTTP %d (tracking ID %s): %s", e.Operation, e.StatusCode, e.TrackingID, message)
	}
	return fmt.Sprintf("%s: HTTP %d: %s", e.Operation, e.StatusCode, message)
}

type baseError struct {
	code    string
	origErr error
}

func (b baseError) Error() string {
	return b.origErr.Error()
}

func (b baseError) String() string {
	return b.Error()
}

func (b baseError) Code() string {
	return b.code
}

func (b baseError) OrigErr() error {
	return b.origErr
}

type AdApiError struct {
	Code      int64  `xml:"AdApiError>Code"`
	Details   string `xml:"AdApiError>Details"`
	ErrorCode string `xml:"AdApiError>ErrorCode"`
	Message   string `xml:"AdApiError>Message"`
}

type OperationError struct {
	Code      int64  `xml:"OperationError>Code" json:"Code"`
	Details   string `xml:"OperationError>Details" json:"Details"`
	ErrorCode string `xml:"OperationError>ErrorCode" json:"ErrorCode"`
	Message   string `xml:"OperationError>Message" json:"Message"`
}

type Fault struct {
	FaultCode   string `xml:"faultcode"`
	FaultString string `xml:"faultstring"`
	Detail      struct {
		XMLName xml.Name   `xml:"detail"`
		Errors  ErrorsType `xml:",any"`
	}
}

type ErrorsType struct {
	TrackingId      string           `xml:"TrackingId"`
	AdApiErrors     []AdApiError     `xml:"Errors"`
	OperationErrors []OperationError `xml:"OperationErrors"`
}

func (f *ErrorsType) Error() string {
	errors := []string{}
	for _, e := range f.AdApiErrors {
		errors = append(errors, e.Message)
	}

	for _, e := range f.OperationErrors {
		errors = append(errors, e.Message)
	}
	return strings.Join(errors, "\n")
}
