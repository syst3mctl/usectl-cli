package api

import "fmt"

// ========== Weekly machine digest (mig 085) ==========

type MachineDigest struct {
	ProjectID  string  `json:"project_id"`
	WeekStart  string  `json:"week_start"`
	Status     string  `json:"status"`
	Severity   string  `json:"severity"`
	Headline   string  `json:"headline"`
	Narrative  string  `json:"narrative"`
	Rendered   string  `json:"rendered"`
	Model      string  `json:"model,omitempty"`
	DurationMs int     `json:"duration_ms"`
	Error      *string `json:"error,omitempty"`
	CreatedAt  string  `json:"created_at"`
}

type MachineDigestSettings struct {
	Enabled     bool     `json:"enabled"`
	SkipQuiet   bool     `json:"skip_quiet"`
	ExtraEmails []string `json:"extra_emails"`
}

func (c *Client) ListMachineDigests(projectID string, limit int) ([]MachineDigest, error) {
	var r struct {
		Digests []MachineDigest `json:"digests"`
	}
	err := c.Get(fmt.Sprintf("/api/projects/%s/digests?limit=%d", projectID, limit), &r)
	return r.Digests, err
}

// GetMachineDigest fetches one week (Monday YYYY-MM-DD) or "latest".
func (c *Client) GetMachineDigest(projectID, week string) (*MachineDigest, error) {
	var d MachineDigest
	err := c.Get(fmt.Sprintf("/api/projects/%s/digests/%s", projectID, week), &d)
	return &d, err
}

func (c *Client) RunMachineDigest(projectID string, deliver bool) (*MachineDigest, error) {
	var d MachineDigest
	path := fmt.Sprintf("/api/projects/%s/digests/run", projectID)
	if deliver {
		path += "?deliver=1"
	}
	err := c.Post(path, nil, &d)
	return &d, err
}

func (c *Client) GetMachineDigestSettings(projectID string) (*MachineDigestSettings, error) {
	var s MachineDigestSettings
	err := c.Get(fmt.Sprintf("/api/projects/%s/digest-settings", projectID), &s)
	return &s, err
}

func (c *Client) PutMachineDigestSettings(projectID string, body map[string]any) (*MachineDigestSettings, error) {
	var s MachineDigestSettings
	err := c.Put(fmt.Sprintf("/api/projects/%s/digest-settings", projectID), body, &s)
	return &s, err
}
