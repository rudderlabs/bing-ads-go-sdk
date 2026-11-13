package bingads

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
)

type Protocol string

const (
	// Deprecated: Microsoft removes SOAP on 2027-01-31.
	ProtocolSOAP Protocol = "soap"
	ProtocolREST Protocol = "rest"
)

const defaultRESTBaseURL = "https://bulk.api.bingads.microsoft.com"

type bulkTransport interface {
	getBulkUploadURL(accountID int64, responseMode string) (*GetBulkUploadUrlResponse, error)
	getBulkUploadStatus(requestID string) (*GetBulkUploadStatusResponse, error)
}

type BulkService struct {
	Endpoint    string
	Session     *Session
	protocol    Protocol
	restBaseURL string
}

type BulkServiceOption func(*BulkService)

func WithProtocol(p Protocol) BulkServiceOption {
	return func(b *BulkService) { b.protocol = p }
}

func WithRESTBaseURL(u string) BulkServiceOption {
	return func(b *BulkService) { b.restBaseURL = u }
}

func NewBulkService(session *Session, opts ...BulkServiceOption) *BulkService {
	b := &BulkService{
		Endpoint:    "https://bulk.api.bingads.microsoft.com/Api/Advertiser/CampaignManagement/v13/BulkService.svc",
		Session:     session,
		protocol:    ProtocolSOAP,
		restBaseURL: defaultRESTBaseURL,
	}
	for _, opt := range opts {
		opt(b)
	}
	return b
}

func (c *BulkService) transport() bulkTransport {
	if c.protocol == ProtocolREST {
		return &restBulkTransport{session: c.Session, baseURL: c.restBaseURL}
	}
	return &soapBulkTransport{session: c.Session, endpoint: c.Endpoint}
}

type GetBulkUploadUrlRequest struct {
	XMLName      xml.Name `xml:"GetBulkUploadUrlRequest" json:"-"`
	NS           string   `xml:"xmlns,attr" json:"-"`
	AccountId    int64    `xml:"AccountId" json:"AccountId"`
	ResponseMode string   `xml:"ResponseMode" json:"ResponseMode"`
}

type GetBulkUploadUrlResponse struct {
	UploadUrl string `xml:"UploadUrl" json:"UploadUrl"`
	RequestId string `xml:"RequestId" json:"RequestId"`
}

type GetBulkUploadStatusRequest struct {
	XMLName   xml.Name `xml:"GetBulkUploadStatusRequest" json:"-"`
	NS        string   `xml:"xmlns,attr" json:"-"`
	RequestId string   `xml:"RequestId" json:"RequestId"`
}

type GetBulkUploadStatusResponse struct {
	XMLName         xml.Name `xml:"GetBulkUploadStatusResponse" json:"-"`
	NS              string   `xml:"xmlns,attr" json:"-"`
	PercentComplete int64    `xml:"PercentComplete" json:"PercentComplete"`
	RequestStatus   string   `xml:"RequestStatus" json:"RequestStatus"`
	ResultFileUrl   string   `xml:"ResultFileUrl" json:"ResultFileUrl"`
	// Errors is the SOAP fault detail; REST never sets it.
	Errors ErrorsType `xml:"ErrorsType" json:"-"`
	// ProcessingErrors is the REST response's Errors: problems found while processing the uploaded file.
	ProcessingErrors []OperationError `xml:"-" json:"Errors"`
}

// GetBulkUploadUrl gets the URL and request ID for a bulk upload.
func (c *BulkService) GetBulkUploadUrl() (*GetBulkUploadUrlResponse, error) {
	accountID, _ := strconv.ParseInt(c.Session.AccountId, 10, 64)
	response, err := c.transport().getBulkUploadURL(accountID, "ErrorsOnly")
	if err != nil {
		return nil, err
	}
	if response.RequestId == "" || response.UploadUrl == "" {
		return nil, fmt.Errorf("unable to get bulk upload url, check your credentials")
	}
	return response, nil
}

// GetBulkUploadStatus gets the current status of a bulk upload.
func (c *BulkService) GetBulkUploadStatus(requestId string) (*GetBulkUploadStatusResponse, error) {
	return c.transport().getBulkUploadStatus(requestId)
}

type UploadBulkFileResponse struct {
	TrackingId string `json:"TrackingId"`
	RequestId  string `json:"RequestId"`
}

func (c *BulkService) UploadBulkFile(url, filename string) (*UploadBulkFileResponse, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	payload := &bytes.Buffer{}
	writer := multipart.NewWriter(payload)
	part1, err := writer.CreateFormFile("", filepath.Base(filename))
	if err != nil {
		return nil, err
	}
	_, err = io.Copy(part1, file)
	if err != nil {
		return nil, err
	}

	if err = writer.Close(); err != nil {
		return nil, err
	}
	req, err := http.NewRequest("POST", url, payload)
	if err != nil {
		return nil, err
	}

	token, err := c.Session.TokenSource.Token()
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("AuthenticationToken", token.AccessToken)
	req.Header.Add("DeveloperToken", c.Session.DeveloperToken)
	req.Header.Add("CustomerId", c.Session.CustomerId)
	req.Header.Add("AccountId", c.Session.AccountId)

	resp, err := c.Session.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var uploadBulkFileResponse UploadBulkFileResponse

	if err = json.Unmarshal(body, &uploadBulkFileResponse); err != nil {
		return nil, err
	}

	if uploadBulkFileResponse.RequestId == "" || uploadBulkFileResponse.TrackingId == "" {
		return nil, fmt.Errorf("unable to upload bulk file, check your credentials")
	}
	return &uploadBulkFileResponse, nil
}
