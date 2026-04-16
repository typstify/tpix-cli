package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/typstify/tpix-cli/config"
	"github.com/typstify/tpix-cli/version"
)

const (
	TpixServer = "https://tpix.typstify.com"
)

var (
	TpixClientUserAgent = fmt.Sprintf("tpix-client/%s", version.Version)
)

type CredentialsProvider interface {
	Load() (config.Credentials, error)
	Save(cred config.Credentials) error
}

type HttpClient struct {
	cred CredentialsProvider

	// refreshMu prevents concurrent refresh attempts
	refreshMu sync.Mutex
}

func NewHttpClient(provider CredentialsProvider) *HttpClient {
	return &HttpClient{cred: provider}
}

// MakeRequest creates an HTTP request with Bearer token.
// On 401 responses, it transparently attempts to refresh the access token
// and retries the request once.
func (c *HttpClient) MakeRequest(method, url string, body io.Reader, contentType string) (*http.Response, error) {
	// Buffer the body so we can replay it on retry
	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = io.ReadAll(body)
		if err != nil {
			return nil, err
		}
	}

	cred, err := c.cred.Load()
	if err != nil {
		return nil, err
	}

	resp, err := c.doRequest(method, url, bodyBytes, contentType, cred.AccessToken)
	if err != nil {
		return nil, err
	}

	// If 401 and we have a refresh token, try to refresh and retry
	if resp.StatusCode == http.StatusUnauthorized && cred.RefreshToken != "" {
		resp.Body.Close()
		refreshErr := c.refreshAccessToken(cred)
		if refreshErr != nil {
			return nil, refreshErr
		}

		// reload config
		cfg, err := c.cred.Load()
		if err != nil {
			return nil, err
		}

		return c.doRequest(method, url, bodyBytes, contentType, cfg.AccessToken)
	}

	return resp, nil
}

// doRequest executes a single HTTP request without retry logic.
func (c *HttpClient) doRequest(method, url string, bodyBytes []byte, contentType string, accessToken string) (*http.Response, error) {
	apiUrl := fmt.Sprintf("%s%s", TpixServer, url)

	var bodyReader io.Reader
	if bodyBytes != nil {
		bodyReader = bytes.NewReader(bodyBytes)
	}

	req, err := http.NewRequest(method, apiUrl, bodyReader)
	if err != nil {
		return nil, err
	}

	if accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}

	req.Header.Set("User-Agent", TpixClientUserAgent)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	return http.DefaultClient.Do(req)
}

// refreshAccessToken uses the stored refresh token to obtain a new access token.
// On success, it updates the config with both new tokens and persists them.
func (c *HttpClient) refreshAccessToken(cred config.Credentials) error {
	c.refreshMu.Lock()
	defer c.refreshMu.Unlock()

	reqBody, _ := json.Marshal(map[string]string{
		"refresh_token": cred.RefreshToken,
	})

	resp, err := c.doRequest("POST", "/auth/token/refresh", reqBody, "application/json", "")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Refresh failed — clear refresh token so we don't keep retrying
		cred.RefreshToken = ""
		c.cred.Save(cred)
		return fmt.Errorf("token refresh failed with status %d", resp.StatusCode)
	}

	var tokenResp TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return err
	}

	cred.AccessToken = tokenResp.AccessToken
	if tokenResp.RefreshToken != "" {
		cred.RefreshToken = tokenResp.RefreshToken
	}

	return c.cred.Save(cred)
}
