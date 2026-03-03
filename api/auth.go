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
	Token string `json:"token"`
	User  User   `json:"user"`
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
