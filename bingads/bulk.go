package bingads

import (
	"encoding/xml"
	"strconv"
)

type BulkService struct {
	Endpoint string
	Session  *Session
}

func NewBulkService(session *Session) *BulkService {
	return &BulkService{
		Endpoint: "https://bulk.api.bingads.microsoft.com/Api/Advertiser/CampaignManagement/v13/BulkService.svc",
		Session:  session,
	}
}

type GetBulkUploadUrlRequest struct {
	XMLName      xml.Name `xml:"GetBulkUploadUrlRequest"`
	NS           string   `xml:"xmlns,attr"`
	AccountId    int64    `xml:"AccountId"`
	ResponseMode string   `xml:"ResponseMode"`
}

type GetBulkUploadUrlResponse struct {
	UploadUrl string `xml:"UploadUrl"`
	RequestId string `xml:"RequestId"`
}

// GetBulkCampaignsByAccountId
func (c *BulkService) GetBulkUploadUrl() (*GetBulkUploadUrlResponse, error) {
	accountId, _ := strconv.ParseInt(c.Session.AccountId, 10, 64)
	req := GetBulkUploadUrlRequest{
		NS:           BingNamespace,
		AccountId:    accountId,
		ResponseMode: "ErrorsOnly",
	}

	resp, err := c.Session.SendRequest(req, c.Endpoint, "GetBulkUploadUrl")
	if err != nil {
		return nil, err
	}

	var getBulkUploadUrlResponse GetBulkUploadUrlResponse
	if err = xml.Unmarshal(resp, &getBulkUploadUrlResponse); err != nil {
		return nil, err
	}
	return &getBulkUploadUrlResponse, nil
}
