package bingads

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/oauth2"
)

func TestNewBulkService(t *testing.T) {
	session := &Session{}
	bulkService := NewBulkService(session)
	if bulkService.Session != session || bulkService.Endpoint != "https://bulk.api.bingads.microsoft.com/Api/Advertiser/CampaignManagement/v13/BulkService.svc" {
		t.Fatalf("unexpected default service: %+v", bulkService)
	}
	if _, ok := bulkService.transport().(*soapBulkTransport); !ok {
		t.Fatalf("default transport must be SOAP: %T", bulkService.transport())
	}
	restService := NewBulkService(session, WithProtocol(ProtocolREST), WithRESTBaseURL("https://example.test"))
	if rest, ok := restService.transport().(*restBulkTransport); !ok || rest.baseURL != "https://example.test" {
		t.Fatalf("unexpected REST transport: %T", restService.transport())
	}
}

func TestBulkSOAPGolden(t *testing.T) {
	for _, tc := range []struct {
		name     string
		wantBody string
		response string
		call     func(*BulkService) error
	}{
		{
			name: "GetBulkUploadUrl",
			wantBody: `<s:Envelope xmlns:i="http://www.w3.org/2001/XMLSchema-instance" xmlns:s="http://schemas.xmlsoap.org/soap/envelope/">
  <s:Header xmlns="https://bingads.microsoft.com/CampaignManagement/v13">
    <Action>GetBulkUploadUrl</Action>
    <AuthenticationToken>secret-token</AuthenticationToken>
    <CustomerAccountId>123</CustomerAccountId>
    <CustomerId>customer</CustomerId>
    <DeveloperToken>developer</DeveloperToken>
  </s:Header>
  <s:Body>
    <GetBulkUploadUrlRequest xmlns="https://bingads.microsoft.com/CampaignManagement/v13">
      <AccountId>123</AccountId>
      <ResponseMode>ErrorsOnly</ResponseMode>
    </GetBulkUploadUrlRequest>
  </s:Body>
</s:Envelope>`,
			response: `<GetBulkUploadUrlResponse><UploadUrl>https://upload.example</UploadUrl><RequestId>id</RequestId></GetBulkUploadUrlResponse>`,
			call: func(b *BulkService) error {
				_, err := b.GetBulkUploadUrl()
				return err
			},
		},
		{
			name: "GetBulkUploadStatus",
			wantBody: `<s:Envelope xmlns:i="http://www.w3.org/2001/XMLSchema-instance" xmlns:s="http://schemas.xmlsoap.org/soap/envelope/">
  <s:Header xmlns="https://bingads.microsoft.com/CampaignManagement/v13">
    <Action>GetBulkUploadStatus</Action>
    <AuthenticationToken>secret-token</AuthenticationToken>
    <CustomerAccountId>123</CustomerAccountId>
    <CustomerId>customer</CustomerId>
    <DeveloperToken>developer</DeveloperToken>
  </s:Header>
  <s:Body>
    <GetBulkUploadStatusRequest xmlns="https://bingads.microsoft.com/CampaignManagement/v13">
      <RequestId>request-123</RequestId>
    </GetBulkUploadStatusRequest>
  </s:Body>
</s:Envelope>`,
			response: `<GetBulkUploadStatusResponse><RequestStatus>Completed</RequestStatus></GetBulkUploadStatusResponse>`,
			call: func(b *BulkService) error {
				_, err := b.GetBulkUploadStatus("request-123")
				return err
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Fatal(err)
				}
				if string(body) != tc.wantBody {
					t.Errorf("SOAP envelope mismatch:\n got: %q\nwant: %q", body, tc.wantBody)
				}
				if _, err := w.Write([]byte(`<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body>` + tc.response + `</s:Body></s:Envelope>`)); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			b := NewBulkService(&Session{AccountId: "123", CustomerId: "customer", DeveloperToken: "developer", TokenSource: oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "secret-token"}), HTTPClient: server.Client()})
			b.Endpoint = server.URL
			if err := tc.call(b); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestBulkSOAPFault(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		if _, err := w.Write([]byte(`<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body><Fault><detail><AdApiFaultDetail><Errors><AdApiError><ErrorCode>AuthenticationTokenExpired</ErrorCode><Message>expired</Message></AdApiError></Errors></AdApiFaultDetail></detail></Fault></s:Body></s:Envelope>`)); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	b := NewBulkService(&Session{AccountId: "123", TokenSource: oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "secret-token"}), HTTPClient: server.Client()})
	b.Endpoint = server.URL
	_, err := b.GetBulkUploadUrl()
	if e, ok := err.(*baseError); !ok || e.Code() != "AuthenticationTokenExpired" {
		t.Fatalf("expected AuthenticationTokenExpired baseError, got %T: %v", err, err)
	}
}
