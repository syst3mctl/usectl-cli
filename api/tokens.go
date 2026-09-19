package api

import "fmt"

// ========== Scoped agent API keys (mig 084) ==========

// AgentTokenPrefix marks a credential that has no refresh token: the client
// must never try to refresh it, and USECTL_TOKEN may carry one.
const AgentTokenPrefix = "usectl_agt_"

type AgentToken struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Prefix      string   `json:"prefix"`
	ProjectIDs  []string `json:"project_ids"` // nil = all machines
	Access      string   `json:"access"`      // read | write
	AdminScopes []string `json:"admin_scopes"`
	ExpiresAt   *string  `json:"expires_at"`
	RevokedAt   *string  `json:"revoked_at,omitempty"`
	LastUsedAt  *string  `json:"last_used_at"`
	CreatedAt   string   `json:"created_at"`
	OwnerEmail  string   `json:"owner_email,omitempty"`
}

type CreateAgentTokenRequest struct {
	Name        string   `json:"name"`
	ProjectIDs  []string `json:"project_ids,omitempty"`
	Access      string   `json:"access"`
	AdminScopes []string `json:"admin_scopes,omitempty"`
	TTLDays     *int     `json:"ttl_days,omitempty"` // nil = 90, 0 = never (admins)
}

type CreateAgentTokenResponse struct {
	Token string     `json:"token"` // plaintext — shown once
	Key   AgentToken `json:"key"`
}

func (c *Client) CreateAgentToken(req CreateAgentTokenRequest) (*CreateAgentTokenResponse, error) {
	var r CreateAgentTokenResponse
	err := c.Post("/api/tokens", req, &r)
	return &r, err
}

func (c *Client) ListAgentTokens() ([]AgentToken, []string, error) {
	var r struct {
		Keys       []AgentToken `json:"keys"`
		AdminAreas []string     `json:"admin_areas"`
	}
	err := c.Get("/api/tokens", &r)
	return r.Keys, r.AdminAreas, err
}

func (c *Client) RevokeAgentToken(id string) error {
	return c.Delete(fmt.Sprintf("/api/tokens/%s", id), nil)
}

func (c *Client) AdminListAgentTokens() ([]AgentToken, error) {
	var r struct {
		Keys []AgentToken `json:"keys"`
	}
	err := c.Get("/api/admin/tokens", &r)
	return r.Keys, err
}

func (c *Client) AdminRevokeAgentToken(id string) error {
	return c.Delete(fmt.Sprintf("/api/admin/tokens/%s", id), nil)
}
