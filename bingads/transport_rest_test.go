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
				if err == nil && (result.RequestStatus != "Completed" || result.PercentComplete != 95 || result.ResultFileUrl != "https://result.example" || len(result.ProcessingErrors) != 1 || result.ProcessingErrors[0] != (OperationError{Code: 11, Details: "detail", ErrorCode: "SomeError", Message: "details"})) {
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
		name, body, code, message, tracking  string
		status, adAPIErrors, operationErrors int
	}{
		{"ad-api-fault", `{"TrackingId":"json-id","Type":"AdApiFaultDetail","Errors":[{"Code":42,"Detail":"why","ErrorCode":"Other","Message":"top failure"}]}`, "", "top failure", "json-id", 400, 1, 0},
		{"api-fault", `{"TrackingId":"json-id","Type":"ApiFaultDetail","OperationErrors":[{"Code":1,"ErrorCode":"Other","Message":"op failure"}],"BatchErrors":[{"Code":2,"ErrorCode":"Other","Index":0,"Message":"batch failure"}]}`, "", "op failure\nbatch failure", "json-id", 400, 0, 2},
		{"missing-messages", `{"Type":"ApiFaultDetail","OperationErrors":[{"ErrorCode":"Other","Details":"first detail"},{"ErrorCode":"Other","Details":"second detail"}]}`, "", `"second detail"`, "header-id", 400, 0, 2},
		{"expired", `{"Type":"AdApiFaultDetail","Errors":[{"ErrorCode":"AuthenticationTokenExpired","Message":"expired"}]}`, "AuthenticationTokenExpired", "expired", "header-id", 401, 1, 0},
		{"credentials", `{"Type":"ApiFaultDetail","OperationErrors":[{"ErrorCode":"InvalidCredentials","Message":"credentials"}]}`, "InvalidCredentials", "credentials", "header-id", 403, 0, 1},
		{"internal", `{"Type":"ApiFaultDetail","BatchErrors":[{"ErrorCode":"InternalError","Message":"internal"}]}`, "InternalError", "internal", "header-id", 500, 0, 1},
		{"rate", `{"Type":"AdApiFaultDetail","Errors":[{"ErrorCode":"CallRateExceeded","Message":"rate"}]}`, "CallRateExceeded", "rate", "header-id", 429, 1, 0},
		{"untyped", `{"Errors":[{"ErrorCode":"CallRateExceeded","Message":"rate"}]}`, "", `"CallRateExceeded"`, "header-id", 429, 0, 0},
		{"html", `<html>unavailable</html>`, "", "<html>unavailable</html>", "header-id", 503, 0, 0},
		{"empty", "", "", "empty response body", "header-id", 503, 0, 0},
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
			if apiErr.TrackingID != tc.tracking || apiErr.Errors.TrackingId != tc.tracking || !strings.Contains(apiErr.Error(), "tracking ID "+tc.tracking) {
				t.Errorf("tracking ID: %+v", apiErr)
			}
			if len(apiErr.Errors.AdApiErrors) != tc.adAPIErrors || len(apiErr.Errors.OperationErrors) != tc.operationErrors {
				t.Errorf("fault errors: %+v", apiErr.Errors)
			}
			if tc.name == "ad-api-fault" && (apiErr.Errors.AdApiErrors[0].Code != 42 || apiErr.Errors.AdApiErrors[0].Details != "why") {
				t.Errorf("AdApiError fields: %+v", apiErr.Errors.AdApiErrors[0])
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

func TestBulkRESTReadFailureKeepsStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("short"))
	}))
	defer server.Close()
	_, err := restTestService(server).GetBulkUploadStatus("id")
	if err == nil || !strings.Contains(err.Error(), "GetBulkUploadStatus: HTTP 502: read response body") {
		t.Fatalf("unexpected error: %v", err)
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
	if strings.Contains(output, "secret-token") || strings.Contains(output, "Authorization") || strings.Contains(output, "upload.example") || !strings.Contains(output, "POST /Bulk/v13/BulkUploadUrl/Query: HTTP 200 (tracking ID tracking-header)") {
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
	if strings.Contains(output, "secret-token") || strings.Contains(output, "developer-secret") || strings.Contains(output, "<s:Envelope") || !strings.Contains(output, "bingads debug request: action=GetBulkUploadUrl") {
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
	if strings.Contains(output, "password-secret") || strings.Contains(output, "developer-secret") || strings.Contains(output, "<s:Envelope") || !strings.Contains(output, "bingads debug request: action=GetBulkUploadUrl") {
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
