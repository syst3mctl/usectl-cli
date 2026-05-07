package api

import (
	"encoding/json"
	"fmt"
)

// ========== Addons ==========

type ProjectAddon struct {
	ID              string            `json:"id"`
	ProjectID       string            `json:"project_id"`
	AddonType       string            `json:"addon_type"`
	Name            string            `json:"name"`
	EnvPrefix       string            `json:"env_prefix"`
	Status          string            `json:"status"`
	Config          map[string]string `json:"config"`
	SharedFrom      *string           `json:"shared_from,omitempty"`
	UIEnabled       bool              `json:"ui_enabled"`
	Mode            string            `json:"mode"`
	Replicas        int               `json:"replicas"`
	DedicatedConfig json.RawMessage   `json:"dedicated_config,omitempty"`
	IsStopped       bool              `json:"is_stopped"`
	CreatedAt       string            `json:"created_at"`
	UpdatedAt       string            `json:"updated_at"`
}

type AddonCatalogEntry struct {
	Type        string   `json:"type"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Icon        string   `json:"icon"`
	EnvVars     []string `json:"env_vars"`
	HasUI       bool     `json:"has_ui"`
	UITool      string   `json:"ui_tool,omitempty"`
}

type AddonCatalogWithStatus struct {
	AddonCatalogEntry
	InUse   bool    `json:"in_use"`
	AddonID *string `json:"addon_id,omitempty"`
}

type AddProjectAddonRequest struct {
	AddonType       string          `json:"addon_type"`
	Mode            string          `json:"mode,omitempty"`
	Name            string          `json:"name,omitempty"`
	SharedFrom      *string         `json:"shared_from,omitempty"`
	Replicas        int             `json:"replicas,omitempty"`
	DedicatedConfig json.RawMessage `json:"dedicated_config,omitempty"`
}

func (c *Client) AddonCatalog() ([]AddonCatalogEntry, error) {
	var entries []AddonCatalogEntry
	err := c.Get("/api/addons/catalog", &entries)
	return entries, err
}

func (c *Client) ListProjectAddons(projectID string) ([]ProjectAddon, error) {
	var addons []ProjectAddon
	err := c.Get(fmt.Sprintf("/api/projects/%s/addons", projectID), &addons)
	return addons, err
}

// ListProjectAddonsCatalog combines the catalog with usage status (?view=catalog).
func (c *Client) ListProjectAddonsCatalog(projectID string) ([]AddonCatalogWithStatus, error) {
	var entries []AddonCatalogWithStatus
	err := c.Get(fmt.Sprintf("/api/projects/%s/addons?view=catalog", projectID), &entries)
	return entries, err
}

func (c *Client) ListShareableAddons(projectID string) ([]ProjectAddon, error) {
	var addons []ProjectAddon
	err := c.Get(fmt.Sprintf("/api/projects/%s/addons/shareable", projectID), &addons)
	return addons, err
}

func (c *Client) AddProjectAddon(projectID string, req AddProjectAddonRequest) (*ProjectAddon, error) {
	var addon ProjectAddon
	err := c.Post(fmt.Sprintf("/api/projects/%s/addons", projectID), req, &addon)
	return &addon, err
}

// RemoveProjectAddon removes by addon type (targets the primary instance).
func (c *Client) RemoveProjectAddon(projectID, addonType string) error {
	return c.Delete(fmt.Sprintf("/api/projects/%s/addons/%s", projectID, addonType), nil)
}

// RemoveProjectAddonByID removes a specific addon row by its UUID.
func (c *Client) RemoveProjectAddonByID(projectID, addonID string) error {
	return c.Delete(fmt.Sprintf("/api/projects/%s/addons/by-id/%s", projectID, addonID), nil)
}

func (c *Client) ToggleAddonUI(projectID, addonType string, enable bool) error {
	body := map[string]bool{"ui_enabled": enable}
	return c.Put(fmt.Sprintf("/api/projects/%s/addons/%s/ui", projectID, addonType), body, nil)
}

func (c *Client) ToggleAddonUIByID(projectID, addonID string, enable bool) error {
	body := map[string]bool{"ui_enabled": enable}
	return c.Put(fmt.Sprintf("/api/projects/%s/addons/by-id/%s/ui", projectID, addonID), body, nil)
}

// UpdateAddonConfig merges the provided config into the addon row.
func (c *Client) UpdateAddonConfig(projectID, addonID string, config map[string]interface{}) error {
	return c.Put(fmt.Sprintf("/api/projects/%s/addons/by-id/%s/config", projectID, addonID), config, nil)
}

func (c *Client) StopProjectAddon(projectID, addonID string) error {
	return c.Post(fmt.Sprintf("/api/projects/%s/addons/%s/stop", projectID, addonID), nil, nil)
}

func (c *Client) StartProjectAddon(projectID, addonID string) error {
	return c.Post(fmt.Sprintf("/api/projects/%s/addons/%s/start", projectID, addonID), nil, nil)
}
