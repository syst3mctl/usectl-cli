package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/giorgi/usectl/config"
)

// Client wraps HTTP calls to the k-deploy API.
type Client struct {
	BaseURL    string
	Token      string
	httpClient *http.Client

	// RefreshToken is the rotating credential used to transparently mint a
	// new access token when the current one has expired. Empty for clients
	// built before the user last logged in, which simply behave as before.
	RefreshToken string

	// refreshing guards against recursion: the refresh call itself must
	// never trigger another refresh on its own 401.
	refreshing bool
}

// NewClient creates a client from the saved config, with optional overrides.
func NewClient(apiURLOverride string) (*Client, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}

	baseURL := cfg.APIURL
	if apiURLOverride != "" {
		baseURL = apiURLOverride
	}

	return &Client{
		BaseURL:      baseURL,
		Token:        cfg.Token,
		RefreshToken: cfg.RefreshToken,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

// NewClientUnauth creates an unauthenticated client (for login/register).
func NewClientUnauth(apiURLOverride string) *Client {
	baseURL := config.DefaultAPIURL
	if apiURLOverride != "" {
		baseURL = apiURLOverride
	} else {
		cfg, err := config.Load()
		if err == nil && cfg.APIURL != "" {
			baseURL = cfg.APIURL
		}
	}

	return &Client{
		BaseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// APIError represents a non-2xx API response.
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("API error %d: %s", e.StatusCode, e.Message)
}

// do performs an HTTP request and decodes the JSON response.
func (c *Client) do(method, path string, body interface{}, result interface{}) error {
	return c.doWithHeaders(method, path, body, result, nil)
}

// doWithHeaders performs an HTTP request with extra headers and decodes the
// JSON response, transparently refreshing an expired access token once.
//
// The retry is what makes a long-lived CLI session work: the access token
// lasts 24h, so any command run the next day would otherwise fail with 401
// and force a manual `usectl login`.
func (c *Client) doWithHeaders(method, path string, body interface{}, result interface{}, headers map[string]string) error {
	err := c.doOnce(method, path, body, result, headers)
	if !c.shouldRetryWithRefresh(err) {
		return err
	}
	if rErr := c.refreshAccessToken(); rErr != nil {
		// Surface the original 401, not the refresh failure — "your session
		// expired, log in again" is the actionable message.
		return err
	}
	return c.doOnce(method, path, body, result, headers)
}

// shouldRetryWithRefresh reports whether err is an expired-session 401 that a
// token refresh could plausibly fix.
func (c *Client) shouldRetryWithRefresh(err error) bool {
	if err == nil || c.refreshing || c.RefreshToken == "" {
		return false
	}
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusUnauthorized
}

// refreshAccessToken exchanges the stored refresh token for a new access
// token and persists both. The server rotates the refresh token on every
// call, so the new value MUST be written to disk or the next refresh will
// look like a replay and revoke the whole session family.
func (c *Client) refreshAccessToken() error {
	c.refreshing = true
	defer func() { c.refreshing = false }()

	resp, err := c.Refresh(c.RefreshToken)
	if err != nil {
		return err
	}

	c.Token = resp.Token
	c.RefreshToken = resp.RefreshToken

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	cfg.Token = resp.Token
	cfg.RefreshToken = resp.RefreshToken
	return config.Save(cfg)
}

// postNoRetry issues a POST that never attempts a token refresh. Used by the
// auth endpoints themselves.
func (c *Client) postNoRetry(path string, body interface{}, result interface{}) error {
	c.refreshing = true
	defer func() { c.refreshing = false }()
	return c.doOnce(http.MethodPost, path, body, result, nil)
}

// doOnce performs a single HTTP request with no refresh handling.
func (c *Client) doOnce(method, path string, body interface{}, result interface{}, headers map[string]string) error {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	url := c.BaseURL + path
	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request to %s: %w", url, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Try to parse error message from JSON
		var errResp struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error != "" {
			return &APIError{StatusCode: resp.StatusCode, Message: errResp.Error}
		}
		return &APIError{StatusCode: resp.StatusCode, Message: string(respBody)}
	}

	if result != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

// doRaw performs an HTTP request and returns the raw response (for binary downloads).
// doRaw performs a request returning the raw response, refreshing an expired
// access token once. Binary downloads go through here.
func (c *Client) doRaw(method, path string) (*http.Response, error) {
	resp, err := c.doRawOnce(method, path)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusUnauthorized && !c.refreshing && c.RefreshToken != "" {
		resp.Body.Close()
		if rErr := c.refreshAccessToken(); rErr != nil {
			return nil, &APIError{StatusCode: http.StatusUnauthorized, Message: "session expired — run 'usectl login'"}
		}
		return c.doRawOnce(method, path)
	}
	return resp, nil
}

func (c *Client) doRawOnce(method, path string) (*http.Response, error) {
	url := c.BaseURL + path
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request to %s: %w", url, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		respBody, _ := io.ReadAll(resp.Body)
		var errResp struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error != "" {
			return nil, &APIError{StatusCode: resp.StatusCode, Message: errResp.Error}
		}
		return nil, &APIError{StatusCode: resp.StatusCode, Message: string(respBody)}
	}

	return resp, nil
}

// Get performs a GET request.
func (c *Client) Get(path string, result interface{}) error {
	return c.do(http.MethodGet, path, nil, result)
}

// Post performs a POST request.
func (c *Client) Post(path string, body interface{}, result interface{}) error {
	return c.do(http.MethodPost, path, body, result)
}

// Put performs a PUT request.
func (c *Client) Put(path string, body interface{}, result interface{}) error {
	return c.do(http.MethodPut, path, body, result)
}

// Delete performs a DELETE request.
func (c *Client) Delete(path string, result interface{}) error {
	return c.do(http.MethodDelete, path, nil, result)
}

// Patch performs a PATCH request.
func (c *Client) Patch(path string, body interface{}, result interface{}) error {
	return c.do(http.MethodPatch, path, body, result)
}

// DeleteWithBody performs a DELETE request with a JSON body.
func (c *Client) DeleteWithBody(path string, body interface{}, result interface{}) error {
	return c.do(http.MethodDelete, path, body, result)
}
