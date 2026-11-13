package bingads

import "encoding/xml"

type soapBulkTransport struct {
	session  *Session
	endpoint string
}

func (t *soapBulkTransport) getBulkUploadURL(accountID int64, responseMode string) (*GetBulkUploadUrlResponse, error) {
	req := GetBulkUploadUrlRequest{
		NS:           BingNamespace,
		AccountId:    accountID,
		ResponseMode: responseMode,
	}

	resp, err := t.session.SendRequest(req, t.endpoint, "GetBulkUploadUrl")
	if err != nil {
		return nil, err
	}

	var getBulkUploadUrlResponse GetBulkUploadUrlResponse
	if err = xml.Unmarshal(resp, &getBulkUploadUrlResponse); err != nil {
		return nil, err
	}
	return &getBulkUploadUrlResponse, nil
}

func (t *soapBulkTransport) getBulkUploadStatus(requestID string) (*GetBulkUploadStatusResponse, error) {
	req := GetBulkUploadStatusRequest{
		NS:        BingNamespace,
		RequestId: requestID,
	}

	resp, err := t.session.SendRequest(req, t.endpoint, "GetBulkUploadStatus")
	if err != nil {
		return nil, err
	}

	var getBulkUploadStatusResponse GetBulkUploadStatusResponse
	if err = xml.Unmarshal(resp, &getBulkUploadStatusResponse); err != nil {
		return nil, err
	}
	return &getBulkUploadStatusResponse, nil
}
