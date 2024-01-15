package bingads

import (
	"reflect"
	"testing"
)

func TestNewBulkService(t *testing.T) {
	session := &Session{}
	want := &BulkService{
		Endpoint: "https://bulk.api.bingads.microsoft.com/Api/Advertiser/CampaignManagement/v13/BulkService.svc",
		Session:  session,
	}
	bulkService := NewBulkService(session)
	if !reflect.DeepEqual(want, bulkService) {
		t.Fatalf(`Test failed`)
	}
}
