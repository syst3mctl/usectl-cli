package api

import (
	"encoding/json"
	"fmt"
	"net/url"
	"time"
)

// ========== Ops digest (admin) ==========

type OpsDigest struct {
	Day        string          `json:"day"`
	Status     string          `json:"status"`
	Severity   string          `json:"severity"`
	Headline   string          `json:"headline"`
	Narrative  string          `json:"narrative"`
	Items      json.RawMessage `json:"items"`
	Rendered   string          `json:"rendered"`
	Model      string          `json:"model"`
	DurationMs int             `json:"duration_ms"`
	Error      *string         `json:"error,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
}

func (c *Client) ListOpsDigests(limit int) ([]OpsDigest, error) {
	var out struct {
		Digests []OpsDigest `json:"digests"`
	}
	path := "/api/admin/digest"
	if limit > 0 {
		path += "?limit=" + fmt.Sprint(limit)
	}
	err := c.Get(path, &out)
	return out.Digests, err
}

func (c *Client) GetOpsDigest(day string) (*OpsDigest, error) {
	if day == "" {
		day = "latest"
	}
	var d OpsDigest
	if err := c.Get("/api/admin/digest/"+url.PathEscape(day), &d); err != nil {
		return nil, err
	}
	return &d, nil
}

func (c *Client) RunOpsDigest() (*OpsDigest, error) {
	var d OpsDigest
	if err := c.Post("/api/admin/digest/run", nil, &d); err != nil {
		return nil, err
	}
	return &d, nil
}
