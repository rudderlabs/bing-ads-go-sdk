package bingads

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"golang.org/x/oauth2"
)

func restTestService(server *httptest.Server) *BulkService {
	return NewBulkService(&Session{
		AccountId: "123", CustomerId: "customer", DeveloperToken: "developer",
		TokenSource: oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "secret-token"}),
		HTTPClient:  server.Client(),
	}, WithProtocol(ProtocolREST), WithRESTBaseURL(server.URL+"/"))
}

func TestBulkRESTQueries(t *testing.T) {
	for _, tc := range []struct {
		name, path, body, response string
		call                       func(*BulkService) error
	}{
		{
			name: "url", path: "/Bulk/v13/BulkUploadUrl/Query",
			body:     `{"AccountId":123,"ResponseMode":"ErrorsOnly"}`,
			response: `{"UploadUrl":"https://upload.example","RequestId":"id"}`,
			call: func(b *BulkService) error {
				result, err := b.GetBulkUploadUrl()
				if err == nil && (result.UploadUrl != "https://upload.example" || result.RequestId != "id") {
					t.Errorf("unexpected URL response: %+v", result)
				}
				return err
			},
		},
		{
			name: "status", path: "/Bulk/v13/BulkUploadStatus/Query",
			body:     `{"RequestId":"id"}`,
			response: `{"RequestStatus":"Completed","PercentComplete":95,"ResultFileUrl":"https://result.example","Errors":[{"Code":11,"ErrorCode":"SomeError","Message":"details","Details":"detail"}]}`,
			call: func(b *BulkService) error {
				result, err := b.GetBulkUploadStatus("id")
				if err == nil && (result.RequestStatus != "Completed" || result.PercentComplete != 95 || result.ResultFileUrl != "https://result.example" || result.Errors.TrackingId != "tracking-header" || len(result.Errors.OperationErrors) != 1 || result.Errors.OperationErrors[0].Code != 11 || result.Errors.OperationErrors[0].Message != "details" || result.Errors.OperationErrors[0].Details != "detail") {
					t.Errorf("unexpected status response: %+v", result)
				}
				return err
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != tc.path {
					t.Errorf("request: %s %s", r.Method, r.URL.Path)
				}
				for header, want := range map[string]string{
					"Content-Type": "application/json", "Authorization": "Bearer secret-token",
					"DeveloperToken": "developer", "CustomerId": "customer", "CustomerAccountId": "123",
				} {
					if got := r.Header.Get(header); got != want {
						t.Errorf("%s: got %q, want %q", header, got, want)
					}
				}
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
				}
				if string(body) != tc.body {
					t.Errorf("body: got %q, want %q", body, tc.body)
				}
				w.Header().Set("TrackingId", "tracking-header")
				_, _ = w.Write([]byte(tc.response))
			}))
			defer server.Close()
			if err := tc.call(restTestService(server)); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestBulkRESTEmptyUploadURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"RequestId":"id"}`))
	}))
	defer server.Close()
	_, err := restTestService(server).GetBulkUploadUrl()
	if err == nil || err.Error() != "unable to get bulk upload url, check your credentials" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBulkRESTFaults(t *testing.T) {
	for _, tc := range []struct {
		name, body, code, message string
		status                    int
	}{
		{"top-level", `{"Type":"AdApiError","TrackingId":"json-id","Errors":[{"Code":42,"ErrorCode":"Other","Message":"top failure"}]}`, "", "top failure", 400},
		{"wrapped", `{"Error":{"Type":"ApiFault","OperationErrors":[{"ErrorCode":"Other","Message":"nested failure"}],"BatchErrors":[{"ErrorCode":"Other","Message":"batch failure"}]}}`, "", "nested failure\nbatch failure", 400},
		{"mixed-level", `{"TrackingId":"outer-id","Type":"ApiFault","Error":{"OperationErrors":[{"ErrorCode":"AuthenticationTokenExpired","Message":"expired"}]}}`, "AuthenticationTokenExpired", "expired", 401},
		{"missing-messages", `{"OperationErrors":[{"ErrorCode":"Other","Details":"first detail"},{"ErrorCode":"Other","Details":"second detail"}]}`, "", `"second detail"`, 400},
		{"expired", `{"Errors":[{"ErrorCode":"AuthenticationTokenExpired","Message":"expired"}]}`, "AuthenticationTokenExpired", "expired", 401},
		{"credentials", `{"OperationErrors":[{"ErrorCode":"InvalidCredentials","Message":"credentials"}]}`, "InvalidCredentials", "credentials", 403},
		{"internal", `{"Error":{"BatchErrors":[{"ErrorCode":"InternalError","Message":"internal"}]}}`, "InternalError", "internal", 500},
		{"rate", `{"Errors":[{"ErrorCode":"CallRateExceeded","Message":"rate"}]}`, "CallRateExceeded", "rate", 429},
		{"html", `<html>unavailable</html>`, "", "<html>unavailable</html>", 503},
		{"empty", "", "", "empty response body", 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("TrackingId", "header-id")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			_, err := restTestService(server).GetBulkUploadStatus("id")
			apiErr, ok := err.(*APIError)
			if tc.code != "" {
				wrapped, isBase := err.(*baseError)
				if !isBase || wrapped.Code() != tc.code {
					t.Fatalf("expected %s baseError; got %T: %v", tc.code, err, err)
				}
				apiErr, ok = wrapped.OrigErr().(*APIError)
			}
			if !ok || apiErr.StatusCode != tc.status || apiErr.Operation != "GetBulkUploadStatus" || apiErr.Body != tc.body || !strings.Contains(apiErr.Error(), tc.message) {
				t.Fatalf("unexpected APIError: %T: %v", err, err)
			}
			wantTracking := "header-id"
			if tc.name == "top-level" {
				wantTracking = "json-id"
				if len(apiErr.Errors.AdApiErrors) != 1 || apiErr.Errors.AdApiErrors[0].Code != 42 {
					t.Errorf("fault errors: %+v", apiErr.Errors)
				}
			}
			if tc.name == "mixed-level" {
				wantTracking = "outer-id"
				if len(apiErr.Errors.OperationErrors) != 1 || apiErr.Errors.OperationErrors[0].ErrorCode != tc.code {
					t.Errorf("wrapped operation errors: %+v", apiErr.Errors)
				}
			}
			if apiErr.TrackingID != wantTracking || apiErr.Errors.TrackingId != wantTracking || !strings.Contains(apiErr.Error(), "tracking ID "+wantTracking) {
				t.Errorf("tracking ID: %+v", apiErr)
			}
			if tc.name == "wrapped" && len(apiErr.Errors.OperationErrors) != 2 {
				t.Errorf("wrapped errors: %+v", apiErr.Errors)
			}
		})
	}
}

func TestBulkRESTNoTrackingIDAndBoundedBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(503)
		_, _ = w.Write([]byte(strings.Repeat("x", 3000)))
	}))
	defer server.Close()
	_, err := restTestService(server).GetBulkUploadUrl()
	apiErr, ok := err.(*APIError)
	if !ok || apiErr.TrackingID != "" || strings.Contains(apiErr.Error(), "tracking ID") || !strings.HasSuffix(apiErr.Error(), strings.Repeat("x", 2048)) {
		t.Fatalf("unexpected bounded error: %T %v", err, err)
	}
}

func TestBulkRESTRequiresTokenSource(t *testing.T) {
	b := NewBulkService(&Session{}, WithProtocol(ProtocolREST))
	if _, err := b.GetBulkUploadUrl(); err == nil || !strings.Contains(err.Error(), "token source") {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := b.GetBulkUploadStatus("id"); err == nil || !strings.Contains(err.Error(), "token source") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBulkRESTDebugHidesToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("TrackingId", "tracking-header")
		_, _ = w.Write([]byte(`{"UploadUrl":"https://upload.example","RequestId":"id"}`))
	}))
	defer server.Close()
	oldDebug := debug
	debug = "1"
	defer func() { debug = oldDebug }()
	output := captureStdout(t, func() {
		if _, err := restTestService(server).GetBulkUploadUrl(); err != nil {
			t.Error(err)
		}
	})
	if strings.Contains(output, "secret-token") || strings.Contains(output, "Authorization") || !strings.Contains(output, "POST /Bulk/v13/BulkUploadUrl/Query: HTTP 200 (tracking ID tracking-header)") {
		t.Fatalf("unsafe or missing debug output: %s", output)
	}
}

func TestBulkSOAPDebugHidesToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body><GetBulkUploadUrlResponse><UploadUrl>url</UploadUrl><RequestId>id</RequestId></GetBulkUploadUrlResponse></s:Body></s:Envelope>`))
	}))
	defer server.Close()
	oldDebug := debug
	debug = "1"
	defer func() { debug = oldDebug }()
	b := NewBulkService(&Session{AccountId: "123", DeveloperToken: "developer-secret", TokenSource: oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "secret-token"}), HTTPClient: server.Client()})
	b.Endpoint = server.URL
	output := captureStdout(t, func() {
		if _, err := b.GetBulkUploadUrl(); err != nil {
			t.Error(err)
		}
	})
	if strings.Contains(output, "secret-token") || strings.Contains(output, "developer-secret") || !strings.Contains(output, "GetBulkUploadUrlRequest") {
		t.Fatalf("unsafe or missing debug output: %s", output)
	}
}

func TestBulkSOAPDebugHidesPassword(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		if !strings.Contains(string(body), "password-secret") || !strings.Contains(string(body), "developer-secret") {
			t.Errorf("credentials missing from wire envelope: %s", body)
		}
		_, _ = w.Write([]byte(`<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body><GetBulkUploadUrlResponse><UploadUrl>url</UploadUrl><RequestId>id</RequestId></GetBulkUploadUrlResponse></s:Body></s:Envelope>`))
	}))
	defer server.Close()
	oldDebug := debug
	debug = "1"
	defer func() { debug = oldDebug }()
	b := NewBulkService(&Session{AccountId: "123", Username: "user", Password: "password-secret", DeveloperToken: "developer-secret", HTTPClient: server.Client()})
	b.Endpoint = server.URL
	output := captureStdout(t, func() {
		if _, err := b.GetBulkUploadUrl(); err != nil {
			t.Error(err)
		}
	})
	if strings.Contains(output, "password-secret") || strings.Contains(output, "developer-secret") || !strings.Contains(output, "GetBulkUploadUrlRequest") {
		t.Fatalf("unsafe or missing debug output: %s", output)
	}
}

func captureStdout(t *testing.T, run func()) string {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = writer
	defer func() { os.Stdout = old }()
	run()
	_ = writer.Close()
	out, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}
