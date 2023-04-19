package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"os"
	"strings"

	"github.com/joho/godotenv"
	"github.com/rudderlabs/bing-ads-go-sdk/bingads"
	"golang.org/x/oauth2"
)

func generate_refresh_token(config *oauth2.Config) (*oauth2.Token, error) {
	token, err := os.ReadFile("token.json")
	if err == nil && len(token) > 0 {
		var tokenResponse oauth2.Token
		if err := json.Unmarshal(token, &tokenResponse); err != nil {
			return nil, err
		}
		return &tokenResponse, nil
	}

	tokenResponse, err := config.Exchange(context.TODO(), os.Getenv("CODE"))
	if err != nil {
		return nil, err
	}
	body, _ := json.MarshalIndent(tokenResponse, "", " ")
	os.WriteFile("token.json", body, 0o644)
	return tokenResponse, nil
}

func generate_access_token(config *oauth2.Config) (*oauth2.Token, error) {
	refreshToken, err := generate_refresh_token(config)
	if err != nil {
		return nil, err
	}

	return config.TokenSource(context.TODO(), refreshToken).Token()
}

func get_session_config(config *oauth2.Config, token *oauth2.Token) bingads.SessionConfig {
	return bingads.SessionConfig{
		OAuth2Config:   config,
		OAuth2Token:    token,
		AccountId:      os.Getenv("CUSTOMER_ACCOUNT_ID"),
		CustomerId:     os.Getenv("CUSTOMER_ID"),
		DeveloperToken: os.Getenv("DEVELOPER_TOKEN"),
		HTTPClient:     http.DefaultClient,
	}
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	oauth2Config := &oauth2.Config{
		ClientID:     os.Getenv("CLIENT_ID"),
		ClientSecret: os.Getenv("CLIENT_SECRET"),
		Endpoint: oauth2.Endpoint{
			AuthURL:  bingads.AuthEndpoint,
			TokenURL: bingads.TokenEndpoint,
		},
		Scopes:      strings.Split(os.Getenv("SCOPE"), " "),
		RedirectURL: os.Getenv("REDIRECT_URI"),
	}
	fmt.Println(oauth2Config.AuthCodeURL("state"))

	token, err := generate_access_token(oauth2Config)
	if err != nil {
		log.Fatal(err)
	}
	session := bingads.NewSession(get_session_config(oauth2Config, token))
	service := bingads.NewBulkService(session)
	urlResp, err := service.GetBulkUploadUrl()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("urlResp %#v\n", urlResp)

	uploadBulkFileResp, err := service.UploadBulkFile(urlResp.UploadUrl, "testfile.zip")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("uploadBulkFileResp %#v\n", uploadBulkFileResp)
	var uploadStatus *bingads.GetBulkUploadStatusResponse
	for {
		time.Sleep(5 * time.Second)
		uploadStatus, err = service.GetBulkUploadStatus(uploadBulkFileResp.RequestId)
		if err != nil {
			log.Fatal(err)
			break
		}
		fmt.Printf("%#v", uploadStatus)
		if uploadStatus.RequestStatus == "Completed" {
			break
		}
	}
}
