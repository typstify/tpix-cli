package api

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"time"

	"github.com/typstify/tpix-cli/version"
)

const (
	TpixServer = "https://tpix.typstify.com"
)

var (
	TpixClientUserAgent = fmt.Sprintf("tpix-client/%s", version.Version)
)

type ApiKeyProvider interface {
	Get() string
}

type HttpClient struct {
	apiKeyProvider ApiKeyProvider
	httpClient     *http.Client
	// baseURL is the TPIX server base URL. Defaults to TpixServer.
	baseURL string
	// maxRetry is the number of attempts for retryable failures.
	maxRetry int
}

func NewHttpClient(provider ApiKeyProvider) *HttpClient {
	defaultClient := &http.Client{
		Timeout: 30 * time.Second,
	}

	return &HttpClient{
		apiKeyProvider: provider,
		httpClient:     defaultClient,
		baseURL:        TpixServer,
		// max retry after request failed for some reason
		maxRetry: 5,
	}
}

func (c *HttpClient) SetMaxRetry(maxRetry int) {
	c.maxRetry = maxRetry
}

// SetHttpClient overrides the underlying HTTP client used to send requests.
func (c *HttpClient) SetHttpClient(client *http.Client) {
	c.httpClient = client
}

// SetBaseURL overrides the server base URL. This is primarily useful in tests
// to point the client at an httptest.Server.
func (c *HttpClient) SetBaseURL(baseURL string) {
	c.baseURL = baseURL
}

// MakeRequest creates an HTTP request with Bearer token. The request is retried when
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

	resp, err := c.doRequestWithRetry(method, url, bodyBytes, contentType)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

// doRequest executes a single HTTP request without retry logic.
func (c *HttpClient) doRequest(method, url string, bodyBytes []byte, contentType string) (*http.Response, error) {
	apiUrl := fmt.Sprintf("%s%s", c.baseURL, url)

	var bodyReader io.Reader
	if bodyBytes != nil {
		bodyReader = bytes.NewReader(bodyBytes)
	}

	req, err := http.NewRequest(method, apiUrl, bodyReader)
	if err != nil {
		return nil, err
	}

	if c.apiKeyProvider != nil {
		req.Header.Set("Authorization", "Bearer "+c.apiKeyProvider.Get())
	}

	req.Header.Set("User-Agent", TpixClientUserAgent)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	client := c.httpClient
	if client == nil {
		client = http.DefaultClient
	}

	return client.Do(req)
}

func (c *HttpClient) doRequestWithRetry(method, url string, bodyBytes []byte, contentType string) (*http.Response, error) {
	retries := 0

	maxRetry := c.maxRetry
	if maxRetry <= 0 {
		c.maxRetry = 1
	}

	for retries < maxRetry {
		waitTimeInMillis := int((math.Pow(2, float64(retries)) - 1) * 100)
		time.Sleep(time.Millisecond * time.Duration(waitTimeInMillis))

		resp, err := c.doRequest(method, url, bodyBytes, contentType)
		if err != nil {
			return nil, err
		}

		switch resp.StatusCode {
		case http.StatusOK, http.StatusAccepted:
			return resp, nil
		case http.StatusTooManyRequests, http.StatusGatewayTimeout, http.StatusServiceUnavailable, http.StatusRequestTimeout:
			resp.Body.Close()
			retries++
		default:
			return resp, nil
		}
	}

	return nil, errors.New("too many failure retries")
}
