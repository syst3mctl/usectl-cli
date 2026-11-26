package api

import (
	"encoding/json"
	"fmt"
)

// ========== developers.usectl.com (addon publishers) ==========

type Publisher struct {
	ID     string `json:"id"`
	Slug   string `json:"slug"`
	Name   string `json:"name"`
	Status string `json:"status"`
	Role   string `json:"role"`
}

type DeveloperMe struct {
	Publishers []Publisher `json:"publishers"`
	IsAdmin    bool        `json:"is_admin"`
}

type ManifestCheck struct {
	OK       bool           `json:"ok"`
	Errors   []string       `json:"errors"`
	Warnings []string       `json:"warnings"`
	Summary  map[string]any `json:"summary,omitempty"`
}

type ReleaseGate struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

type DeveloperRelease struct {
	ID         string          `json:"id"`
	Version    string          `json:"version"`
	Status     string          `json:"status"`
	Notes      string          `json:"notes"`
	Critical   bool            `json:"critical"`
	ReleasedAt string          `json:"released_at"`
	Gates      []ReleaseGate   `json:"gates"`
	ReviewNote string          `json:"review_note"`
	Manifest   json.RawMessage `json:"manifest"`
}

type DeveloperAddon struct {
	Listing struct {
		AddonType string `json:"addon_type"`
		Name      string `json:"name"`
		Slug      string `json:"slug"`
		Channel   string `json:"channel"`
	} `json:"listing"`
	Releases     []DeveloperRelease        `json:"releases"`
	TestMachines []string                  `json:"test_machines"`
	Installs     map[string]map[string]int `json:"installs"`
}

type SubmitReleaseRequest struct {
	Manifest        string `json:"manifest"`
	Notes           string `json:"notes,omitempty"`
	Critical        bool   `json:"critical,omitempty"`
	DowngradeUnsafe bool   `json:"downgrade_unsafe,omitempty"`
}

func (c *Client) DeveloperMe() (*DeveloperMe, error) {
	var out DeveloperMe
	err := c.Get("/api/developer/me", &out)
	return &out, err
}

func (c *Client) ValidateManifest(manifest, publisher string) (*ManifestCheck, error) {
	var out ManifestCheck
	err := c.Post("/api/developer/validate", map[string]string{"manifest": manifest, "publisher": publisher}, &out)
	return &out, err
}

func (c *Client) CreatePublisherAddon(publisher, manifest string) error {
	return c.Post(fmt.Sprintf("/api/developer/publishers/%s/addons", publisher), map[string]string{"manifest": manifest}, nil)
}

func (c *Client) GetDeveloperAddon(addonType string) (*DeveloperAddon, error) {
	var out DeveloperAddon
	err := c.Get(fmt.Sprintf("/api/developer/addons/%s", addonType), &out)
	return &out, err
}

func (c *Client) SubmitAddonRelease(addonType string, req SubmitReleaseRequest) (*DeveloperRelease, error) {
	var out DeveloperRelease
	err := c.Post(fmt.Sprintf("/api/developer/addons/%s/releases", addonType), req, &out)
	return &out, err
}
