package api

// ========== Google OAuth ==========

// GoogleClientInfo mirrors GET /api/google/client-id.
type GoogleClientInfo struct {
	ClientID string `json:"client_id"`
}

// GetGoogleClientInfo fetches the public Google OAuth client_id from the
// platform. Returns 404 when Google Sign In isn't configured server-side.
func (c *Client) GetGoogleClientInfo() (*GoogleClientInfo, error) {
	var info GoogleClientInfo
	err := c.Get("/api/google/client-id", &info)
	return &info, err
}

// LoginWithGoogle exchanges an OAuth code (and the matching redirect_uri) for
// a usectl JWT. The backend handles the Google token-exchange + id_token
// verification + find-or-create-user dance; we just hand it the code.
//
// Returns the same {token, user} shape as Login / Register.
func (c *Client) LoginWithGoogle(code, redirectURI string) (*AuthResponse, error) {
	body := map[string]string{
		"code":         code,
		"redirect_uri": redirectURI,
	}
	var resp AuthResponse
	err := c.Post("/api/auth/google", body, &resp)
	return &resp, err
}
