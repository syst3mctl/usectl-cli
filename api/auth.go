package api

// ========== Auth ==========

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type RegisterRequest struct {
	Email    string `json:"email"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type AuthResponse struct {
	Token        string `json:"token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	User         User   `json:"user"`
}

// RefreshRequest carries the refresh token to /auth/refresh and /auth/logout.
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type User struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	Username string `json:"username"`
	Role     string `json:"role"`
	Enabled  bool   `json:"enabled"`
}

type UpdateProfileRequest struct {
	Username string `json:"username,omitempty"`
	Email    string `json:"email,omitempty"`
	Password string `json:"password,omitempty"`
}

func (c *Client) Login(req LoginRequest) (*AuthResponse, error) {
	var resp AuthResponse
	err := c.Post("/api/auth/login", req, &resp)
	return &resp, err
}

func (c *Client) Register(req RegisterRequest) (*AuthResponse, error) {
	var resp AuthResponse
	err := c.Post("/api/auth/register", req, &resp)
	return &resp, err
}

func (c *Client) GetProfile() (*User, error) {
	var user User
	err := c.Get("/api/auth/profile", &user)
	return &user, err
}

func (c *Client) UpdateProfile(req UpdateProfileRequest) (*User, error) {
	var user User
	err := c.Put("/api/auth/profile", req, &user)
	return &user, err
}

// Refresh exchanges a refresh token for a new access token and a new refresh
// token. The server rotates on every call, so the returned RefreshToken
// replaces the one that was sent — persist it or the next refresh fails.
//
// Deliberately does not go through the 401-retry path in doWithHeaders: a
// failed refresh must surface as "log in again", not recurse.
func (c *Client) Refresh(refreshToken string) (*AuthResponse, error) {
	var resp AuthResponse
	if err := c.postNoRetry("/api/auth/refresh", RefreshRequest{RefreshToken: refreshToken}, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Logout revokes this device's refresh token server-side. Other devices stay
// signed in. Safe to call with an expired access token — the endpoint is
// public and the refresh token is the credential.
func (c *Client) Logout(refreshToken string) error {
	return c.postNoRetry("/api/auth/logout", RefreshRequest{RefreshToken: refreshToken}, nil)
}

// LogoutAll revokes every refresh token for the account ("sign out
// everywhere"). Requires a valid access token.
func (c *Client) LogoutAll() (int64, error) {
	var resp struct {
		SessionsRevoked int64 `json:"sessions_revoked"`
	}
	err := c.Post("/api/auth/logout-all", nil, &resp)
	return resp.SessionsRevoked, err
}
