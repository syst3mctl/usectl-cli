package api

import "fmt"

// ========== Admin ==========

type SetEnabledRequest struct {
	Enabled bool `json:"enabled"`
}

type SetRoleRequest struct {
	Role string `json:"role"`
}

func (c *Client) ListUsers() ([]User, error) {
	var users []User
	err := c.Get("/api/admin/users", &users)
	return users, err
}

func (c *Client) SetUserEnabled(id string, enabled bool) error {
	return c.Put(fmt.Sprintf("/api/admin/users/%s/enabled", id), SetEnabledRequest{Enabled: enabled}, nil)
}

func (c *Client) SetUserRole(id string, role string) error {
	return c.Put(fmt.Sprintf("/api/admin/users/%s/role", id), SetRoleRequest{Role: role}, nil)
}

func (c *Client) DeleteUser(id string) error {
	return c.Delete(fmt.Sprintf("/api/admin/users/%s", id), nil)
}
