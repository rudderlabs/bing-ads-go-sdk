package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/joho/godotenv"
	"github.com/rudderlabs/bing-ads-go-sdk/bingads"
	"golang.org/x/oauth2"
)

func generate_token(params url.Values) (*oauth2.Token, error) {
	endpoint := "https://login.microsoftonline.com/common/oauth2/v2.0/token"
	method := "POST"
	payload := strings.NewReader(params.Encode())
	client := &http.Client{}
	req, err := http.NewRequest(method, endpoint, payload)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	var tokenResponse oauth2.Token
	if err := json.Unmarshal(body, &tokenResponse); err != nil {
		return nil, err
	}
	if tokenResponse.RefreshToken == "" {
		return nil, fmt.Errorf("invalid token")
	}
	return &tokenResponse, nil
}

func generate_refresh_token() (*oauth2.Token, error) {
	token, err := os.ReadFile("token.json")
	if err == nil && len(token) > 0 {
		var tokenResponse oauth2.Token
		if err := json.Unmarshal(token, &tokenResponse); err != nil {
			return nil, err
		}
		return &tokenResponse, nil
	}

	params := url.Values{}
	params.Add("client_id", os.Getenv("CLIENT_ID"))
	params.Add("scope", os.Getenv("SCOPE"))
	params.Add("code", os.Getenv("CODE"))
	params.Add("redirect_uri", os.Getenv("REDIRECT_URI"))
	params.Add("grant_type", "authorization_code")
	params.Add("client_secret", os.Getenv("CLIENT_SECRET"))
	tokenResponse, err := generate_token(params)
	if err != nil {
		return nil, err
	}
	body, _ := json.MarshalIndent(tokenResponse, "", " ")
	os.WriteFile("token.json", body, 0o644)
	return tokenResponse, nil
}

func generate_access_token() (*oauth2.Token, error) {
	refreshToken, err := generate_refresh_token()
	if err != nil {
		return nil, err
	}

	params := url.Values{}
	params.Add("client_id", os.Getenv("CLIENT_ID"))
	params.Add("scope", os.Getenv("SCOPE"))
	params.Add("refresh_token", refreshToken.RefreshToken)
	params.Add("redirect_uri", os.Getenv("REDIRECT_URI"))
	params.Add("client_secret", os.Getenv("CLIENT_SECRET"))
	params.Add("grant_type", "refresh_token")
	return generate_token(params)
}

func get_session_config(token *oauth2.Token) bingads.SessionConfig {
	return bingads.SessionConfig{
		OAuth2Config: &oauth2.Config{
			ClientID:     os.Getenv("CLIENT_ID"),
			ClientSecret: os.Getenv("CLIENT_SECRET"),
			Endpoint: oauth2.Endpoint{
				AuthURL:  bingads.AuthEndpoint,
				TokenURL: bingads.TokenEndpoint,
			},
			Scopes:      strings.Split(os.Getenv("SCOPE"), " "),
			RedirectURL: "https://login.microsoftonline.com/common/oauth2/nativeclient",
		},
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

	token, err := generate_access_token()
	if err != nil {
		log.Fatal(err)
	}
	session := bingads.NewSession(get_session_config(token))
	service := bingads.NewBulkService(session)
	resp, err := service.GetBulkUploadUrl()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%#v\n", resp)
}
