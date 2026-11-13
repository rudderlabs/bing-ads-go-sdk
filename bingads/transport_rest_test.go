package bingads

import (
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/oauth2"
)

const (
	testAccessToken = "secret-token"
	testTrackingID  = "header-tracking-id"
)

// fakeBingREST answers every request with status and body, and records the last request.
type fakeBingREST struct {
	*httptest.Server
	request *http.Request
	body    string
}

func newFakeBingREST(t *testing.T, status int, body string) *fakeBingREST {
	t.Helper()
	fake := &fakeBingREST{}
	fake.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestBody, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		fake.request, fake.body = r, string(requestBody)
		w.Header().Set("TrackingId", testTrackingID)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(fake.Close)
	return fake
}

func newRESTService(server *httptest.Server) *BulkService {
	return NewBulkService(&Session{
		AccountId:      "123",
		CustomerId:     "456",
		DeveloperToken: "developer-token",
		TokenSource:    oauth2.StaticTokenSource(&oauth2.Token{AccessToken: testAccessToken}),
		HTTPClient:     server.Client(),
	}, WithProtocol(ProtocolREST), WithRESTBaseURL(server.URL))
}

func requireRequest(t *testing.T, fake *fakeBingREST, wantPath, wantBody string) {
	t.Helper()
	if fake.request.Method != http.MethodPost || fake.request.URL.Path != wantPath {
		t.Errorf("request = %s %s, want POST %s", fake.request.Method, fake.request.URL.Path, wantPath)
	}
	if fake.body != wantBody {
		t.Errorf("request body = %s, want %s", fake.body, wantBody)
	}
	wantHeaders := map[string]string{
		"Content-Type":      "application/json",
		"Authorization":     "Bearer " + testAccessToken,
		"DeveloperToken":    "developer-token",
		"CustomerId":        "456",
		"CustomerAccountId": "123",
	}
	for name, want := range wantHeaders {
		if got := fake.request.Header.Get(name); got != want {
			t.Errorf("header %s = %q, want %q", name, got, want)
		}
	}
}

func TestRESTGetBulkUploadUrl(t *testing.T) {
	fake := newFakeBingREST(t, http.StatusOK, `{"RequestId":"request-1","UploadUrl":"https://upload.example"}`)

	got, err := newRESTService(fake.Server).GetBulkUploadUrl()
	if err != nil {
		t.Fatal(err)
	}

	requireRequest(t, fake, "/Bulk/v13/BulkUploadUrl/Query", `{"AccountId":123,"ResponseMode":"ErrorsOnly"}`)
	want := &GetBulkUploadUrlResponse{RequestId: "request-1", UploadUrl: "https://upload.example"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("response = %+v, want %+v", got, want)
	}
}

func TestRESTGetBulkUploadUrlWithoutUploadUrl(t *testing.T) {
	fake := newFakeBingREST(t, http.StatusOK, `{"RequestId":"request-1"}`)

	_, err := newRESTService(fake.Server).GetBulkUploadUrl()

	if err == nil || err.Error() != "unable to get bulk upload url, check your credentials" {
		t.Errorf("error = %v, want the empty upload URL error", err)
	}
}

func TestRESTGetBulkUploadStatus(t *testing.T) {
	tests := []struct {
		name string
		body string
		want *GetBulkUploadStatusResponse
	}{
		{
			name: "processing errors",
			body: `{"RequestStatus":"Failed","PercentComplete":0,"ResultFileUrl":"","Errors":[{"Code":3220,"ErrorCode":"BulkServiceFormatVersionNotSupported","Message":"format not supported","Details":"row 1"}]}`,
			want: &GetBulkUploadStatusResponse{
				RequestStatus: "Failed",
				ProcessingErrors: []OperationError{
					{Code: 3220, ErrorCode: "BulkServiceFormatVersionNotSupported", Message: "format not supported", Details: "row 1"},
				},
			},
		},
		{
			name: "errors null",
			body: `{"RequestStatus":"Completed","PercentComplete":100,"ResultFileUrl":"https://result.example","Errors":null}`,
			want: &GetBulkUploadStatusResponse{RequestStatus: "Completed", PercentComplete: 100, ResultFileUrl: "https://result.example"},
		},
		{
			name: "errors missing",
			body: `{"RequestStatus":"InProgress","PercentComplete":40}`,
			want: &GetBulkUploadStatusResponse{RequestStatus: "InProgress", PercentComplete: 40},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fake := newFakeBingREST(t, http.StatusOK, tc.body)

			got, err := newRESTService(fake.Server).GetBulkUploadStatus("request-1")
			if err != nil {
				t.Fatal(err)
			}

			requireRequest(t, fake, "/Bulk/v13/BulkUploadStatus/Query", `{"RequestId":"request-1"}`)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("response = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// TestRESTFaults covers the two documented fault shapes and bodies that match neither.
func TestRESTFaults(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		// wantCode is set when the APIError is wrapped in a baseError with this code.
		wantCode string
		want     ErrorsType
	}{
		{
			name:   "AdApiFaultDetail",
			status: http.StatusBadRequest,
			body:   `{"TrackingId":"body-tracking-id","Type":"AdApiFaultDetail","Errors":[{"Code":105,"Detail":"why","ErrorCode":"InvalidCustomerId","Message":"bad customer"}]}`,
			want: ErrorsType{
				TrackingId:  "body-tracking-id",
				AdApiErrors: []AdApiError{{Code: 105, Details: "why", ErrorCode: "InvalidCustomerId", Message: "bad customer"}},
			},
		},
		{
			name:   "ApiFaultDetail with operation and batch errors",
			status: http.StatusBadRequest,
			body:   `{"TrackingId":"body-tracking-id","Type":"ApiFaultDetail","OperationErrors":[{"Code":4201,"ErrorCode":"BulkServiceInvalidRequestId","Message":"bad request id"}],"BatchErrors":[{"Code":1,"ErrorCode":"BatchFailure","Index":0,"Message":"bad row"}]}`,
			want: ErrorsType{
				TrackingId: "body-tracking-id",
				OperationErrors: []OperationError{
					{Code: 4201, ErrorCode: "BulkServiceInvalidRequestId", Message: "bad request id"},
					{Code: 1, ErrorCode: "BatchFailure", Message: "bad row"},
				},
			},
		},
		{
			name:     "expired token is wrapped",
			status:   http.StatusUnauthorized,
			body:     `{"Type":"AdApiFaultDetail","Errors":[{"ErrorCode":"AuthenticationTokenExpired","Message":"expired"}]}`,
			wantCode: "AuthenticationTokenExpired",
			want: ErrorsType{
				TrackingId:  testTrackingID,
				AdApiErrors: []AdApiError{{ErrorCode: "AuthenticationTokenExpired", Message: "expired"}},
			},
		},
		{
			name:     "rate limit is wrapped",
			status:   http.StatusTooManyRequests,
			body:     `{"Type":"ApiFaultDetail","OperationErrors":[{"ErrorCode":"CallRateExceeded","Message":"slow down"}]}`,
			wantCode: "CallRateExceeded",
			want: ErrorsType{
				TrackingId:      testTrackingID,
				OperationErrors: []OperationError{{ErrorCode: "CallRateExceeded", Message: "slow down"}},
			},
		},
		{
			name:   "JSON without Type is not decoded",
			status: http.StatusBadRequest,
			body:   `{"Errors":[{"ErrorCode":"CallRateExceeded","Message":"slow down"}]}`,
			want:   ErrorsType{TrackingId: testTrackingID},
		},
		{
			name:   "HTML",
			status: http.StatusServiceUnavailable,
			body:   `<html>unavailable</html>`,
			want:   ErrorsType{TrackingId: testTrackingID},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fake := newFakeBingREST(t, tc.status, tc.body)

			_, err := newRESTService(fake.Server).GetBulkUploadStatus("request-1")

			apiErr, code := unwrapAPIError(t, err)
			if code != tc.wantCode {
				t.Errorf("baseError code = %q, want %q", code, tc.wantCode)
			}
			if apiErr.Operation != "GetBulkUploadStatus" || apiErr.StatusCode != tc.status || apiErr.Body != tc.body {
				t.Errorf("APIError = {%s %d %s}, want {GetBulkUploadStatus %d %s}", apiErr.Operation, apiErr.StatusCode, apiErr.Body, tc.status, tc.body)
			}
			if apiErr.TrackingID != tc.want.TrackingId {
				t.Errorf("TrackingID = %q, want %q", apiErr.TrackingID, tc.want.TrackingId)
			}
			if !reflect.DeepEqual(apiErr.Errors, tc.want) {
				t.Errorf("Errors = %+v, want %+v", apiErr.Errors, tc.want)
			}
		})
	}
}

func TestAPIErrorMessage(t *testing.T) {
	tests := []struct {
		name string
		err  *APIError
		want string
	}{
		{
			name: "decoded messages",
			err: &APIError{Operation: "GetBulkUploadStatus", StatusCode: 400, TrackingID: "t-1", Errors: ErrorsType{
				OperationErrors: []OperationError{{Message: "first"}, {Message: "second"}},
			}},
			want: "GetBulkUploadStatus: HTTP 400 (tracking ID t-1): first\nsecond",
		},
		{
			name: "no messages falls back to body",
			err:  &APIError{Operation: "GetBulkUploadUrl", StatusCode: 503, Body: "<html>unavailable</html>"},
			want: "GetBulkUploadUrl: HTTP 503: <html>unavailable</html>",
		},
		{
			name: "empty body",
			err:  &APIError{Operation: "GetBulkUploadUrl", StatusCode: 503},
			want: "GetBulkUploadUrl: HTTP 503: empty response body",
		},
		{
			name: "long body is cut to 2048 bytes",
			err:  &APIError{Operation: "GetBulkUploadUrl", StatusCode: 503, Body: strings.Repeat("x", 3000)},
			want: "GetBulkUploadUrl: HTTP 503: " + strings.Repeat("x", 2048),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.err.Error(); got != tc.want {
				t.Errorf("Error() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRESTReadFailureKeepsStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100") // promise more bytes than are sent
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("short"))
	}))
	defer server.Close()

	_, err := newRESTService(server).GetBulkUploadStatus("request-1")

	if err == nil || !strings.HasPrefix(err.Error(), "GetBulkUploadStatus: HTTP 502: read response body:") {
		t.Errorf("error = %v, want it to keep HTTP 502", err)
	}
}

func TestRESTRequiresTokenSource(t *testing.T) {
	service := NewBulkService(&Session{}, WithProtocol(ProtocolREST))

	_, err := service.GetBulkUploadUrl()

	if err == nil || err.Error() != "GetBulkUploadUrl: REST requires a token source" {
		t.Errorf("error = %v, want the missing token source error", err)
	}
}

func TestRESTDebugLogOmitsSecrets(t *testing.T) {
	fake := newFakeBingREST(t, http.StatusOK, `{"RequestId":"request-1","UploadUrl":"https://upload.example/signed"}`)
	enableDebug(t)

	output := captureStdout(t, func() {
		if _, err := newRESTService(fake.Server).GetBulkUploadUrl(); err != nil {
			t.Error(err)
		}
	})

	want := "POST /Bulk/v13/BulkUploadUrl/Query: HTTP 200 (tracking ID " + testTrackingID + ")\n"
	if output != want {
		t.Errorf("debug output = %q, want %q", output, want)
	}
}

// unwrapAPIError returns the APIError in err and the baseError code around it, if any.
func unwrapAPIError(t *testing.T, err error) (*APIError, string) {
	t.Helper()
	code := ""
	if wrapped, ok := err.(*baseError); ok {
		code, err = wrapped.Code(), wrapped.OrigErr()
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("error = %T %v, want *APIError", err, err)
	}
	return apiErr, code
}
