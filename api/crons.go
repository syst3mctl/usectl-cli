package api

import "fmt"

// ========== Cron Jobs ==========

type ProjectCron struct {
	ID         string  `json:"id"`
	ProjectID  string  `json:"project_id"`
	Name       string  `json:"name"`
	Schedule   string  `json:"schedule"`
	Command    string  `json:"command"`
	Enabled    bool    `json:"enabled"`
	LastRunAt  *string `json:"last_run_at,omitempty"`
	LastStatus string  `json:"last_status"`
	CreatedAt  string  `json:"created_at"`
	UpdatedAt  string  `json:"updated_at"`
}

type CreateCronRequest struct {
	Name     string `json:"name"`
	Schedule string `json:"schedule"`
	Command  string `json:"command"`
}

type UpdateCronRequest struct {
	Schedule *string `json:"schedule,omitempty"`
	Command  *string `json:"command,omitempty"`
	Enabled  *bool   `json:"enabled,omitempty"`
}

func (c *Client) ListProjectCrons(projectID string) ([]ProjectCron, error) {
	var crons []ProjectCron
	err := c.Get(fmt.Sprintf("/api/projects/%s/crons", projectID), &crons)
	return crons, err
}

func (c *Client) CreateProjectCron(projectID string, req CreateCronRequest) (*ProjectCron, error) {
	var cron ProjectCron
	err := c.Post(fmt.Sprintf("/api/projects/%s/crons", projectID), req, &cron)
	return &cron, err
}

func (c *Client) UpdateProjectCron(projectID, cronID string, req UpdateCronRequest) (*ProjectCron, error) {
	var cron ProjectCron
	err := c.Put(fmt.Sprintf("/api/projects/%s/crons/%s", projectID, cronID), req, &cron)
	return &cron, err
}

func (c *Client) DeleteProjectCron(projectID, cronID string) error {
	return c.Delete(fmt.Sprintf("/api/projects/%s/crons/%s", projectID, cronID), nil)
}
