package api

import "fmt"

// CLI browser-login exchange (backend mig 070).

type CLIExchangeRequest struct {
	Code     string `json:"code"`
	Verifier string `json:"verifier"`
}

// ExchangeCLICode trades a one-time code plus the PKCE verifier for a token
// pair. Unauthenticated by design — this is the call that obtains credentials.
func (c *Client) ExchangeCLICode(code, verifier string) (*AuthResponse, error) {
	var resp AuthResponse
	if err := c.Post("/api/auth/cli/exchange", CLIExchangeRequest{Code: code, Verifier: verifier}, &resp); err != nil {
		return nil, fmt.Errorf("exchange authorization code: %w", err)
	}
	return &resp, nil
}
