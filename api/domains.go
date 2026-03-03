package api

import "fmt"

// ========== Domains ==========

type Domain struct {
	ID        string  `json:"id"`
	Domain    string  `json:"domain"`
	ProjectID *string `json:"project_id,omitempty"`
	OwnerID   string  `json:"owner_id"`
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`
}

type CreateDomainRequest struct {
	Domain string `json:"domain"`
}

type AttachDomainRequest struct {
	ProjectID string `json:"project_id"`
}

func (c *Client) ListDomains() ([]Domain, error) {
	var domains []Domain
	err := c.Get("/api/domains", &domains)
	return domains, err
}

func (c *Client) GetDomain(id string) (*Domain, error) {
	var domain Domain
	err := c.Get(fmt.Sprintf("/api/domains/%s", id), &domain)
	return &domain, err
}

func (c *Client) CreateDomain(req CreateDomainRequest) (*Domain, error) {
	var domain Domain
	err := c.Post("/api/domains", req, &domain)
	return &domain, err
}

func (c *Client) AttachDomain(id string, req AttachDomainRequest) (*Domain, error) {
	var domain Domain
	err := c.Put(fmt.Sprintf("/api/domains/%s/attach", id), req, &domain)
	return &domain, err
}

func (c *Client) DeleteDomain(id string) error {
	return c.Delete(fmt.Sprintf("/api/domains/%s", id), nil)
}
