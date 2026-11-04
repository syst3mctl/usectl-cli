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
	// WorkloadName is the StatefulSet/pod base name for a dedicated addon
	// (e.g. "runbyagents-postgres"), supplied by the API so a client can map
	// the addon to its pods without guessing at the naming scheme. Empty for
	// managed addons, which have no pods of their own.
	WorkloadName string `json:"workload_name,omitempty"`
	// Instances is how many copies actually run (a replicated Postgres runs
	// 2, each at the full size). Older servers omit it.
	Instances int     `json:"instances,omitempty"`
	GroupID   *string `json:"group_id,omitempty"`
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`
}

type AddonCatalogEntry struct {
	Type        string   `json:"type"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Icon        string   `json:"icon"`
	EnvVars     []string `json:"env_vars"`
	HasUI       bool     `json:"has_ui"`
	UITool      string   `json:"ui_tool,omitempty"`
	// SupportsDedicated is false for managed-only addons (S3, cron, oauth2 …).
	// Offering "dedicated" for those asks a question the server will reject.
	SupportsDedicated bool `json:"supports_dedicated"`
	// SizePresets is the real dedicated-mode ladder, published by the
	// provisioner. Empty on an older API, in which case the size is asked for
	// as free text and validated server-side.
	SizePresets []AddonSizePreset `json:"size_presets,omitempty"`
}

// AddonSizePreset is one rung of an addon's dedicated-mode size ladder.
type AddonSizePreset struct {
	Name       string `json:"name"`
	CPUMillis  int    `json:"cpu_millis"`
	MemoryMiB  int    `json:"memory_mib"`
	StorageGiB int    `json:"storage_gib"`
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
//
// A dedicated Postgres takes a final backup before it is deleted (when it has
// a backup schedule) and the removal is refused if that backup fails.
// skipFinalBackup opts out — the data is then gone for good.
func (c *Client) RemoveProjectAddon(projectID, addonType string, skipFinalBackup bool) error {
	return c.Delete(fmt.Sprintf("/api/projects/%s/addons/%s%s", projectID, addonType, removeAddonQuery(skipFinalBackup)), nil)
}

// RemoveProjectAddonByID removes a specific addon row by its UUID.
func (c *Client) RemoveProjectAddonByID(projectID, addonID string, skipFinalBackup bool) error {
	return c.Delete(fmt.Sprintf("/api/projects/%s/addons/by-id/%s%s", projectID, addonID, removeAddonQuery(skipFinalBackup)), nil)
}

func removeAddonQuery(skipFinalBackup bool) string {
	if skipFinalBackup {
		return "?skip_final_backup=true"
	}
	return ""
}

func (c *Client) ToggleAddonUI(projectID, addonType string, enable bool) error {
	body := map[string]bool{"enabled": enable}
	return c.Put(fmt.Sprintf("/api/projects/%s/addons/%s/ui", projectID, addonType), body, nil)
}

func (c *Client) ToggleAddonUIByID(projectID, addonID string, enable bool) error {
	body := map[string]bool{"enabled": enable}
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

// MoveAddonGroup moves an addon between machine groups.
//
// groupID is the all-zero UUID to move the addon out of any group. pvcAction is
// "leave" (default — the volume stays in the old namespace for manual recovery)
// or "destroy". Only dedicated addons own a PVC; for a managed addon the move
// just rewrites the connection Secret on the next deploy.
func (c *Client) MoveAddonGroup(projectID, addonID, groupID, pvcAction string) error {
	body := map[string]interface{}{"group_id": groupID}
	if pvcAction != "" {
		body["pvc_action"] = pvcAction
	}
	return c.Post(fmt.Sprintf("/api/projects/%s/addons/%s/move-group", projectID, addonID), body, nil)
}

// NATSAuthRules mirrors the backend's deployer.NATSAuthRules.
type NATSAuthRules struct {
	Subscribe         []string `json:"subscribe"`
	Publish           []string `json:"publish"`
	Inbox             bool     `json:"inbox"`
	MaxSessionSeconds int64    `json:"max_session_seconds"`
}

type NATSAuthUpdate struct {
	Mode           string         `json:"mode"`
	ValidatorAppID string         `json:"validator_app_id,omitempty"`
	Path           string         `json:"path,omitempty"`
	Port           int            `json:"port,omitempty"`
	Rules          *NATSAuthRules `json:"rules,omitempty"`
}

type NATSAuthResult struct {
	Mode              string `json:"mode"`
	Message           string `json:"message"`
	ValidatorAttached *bool  `json:"validator_attached,omitempty"`
	ValidatorURL      string `json:"validator_url,omitempty"`
}

// UpdateNATSAuth sets how WebSocket clients of a dedicated NATS authenticate:
// "password", "readonly" or "callout" (per-user, validated by one of the
// machine's pods).
// GetAddonCredentials returns the real values of an addon's secret config
// keys. Every other addon response masks them. Developer role or above;
// the server audits each call.
func (c *Client) GetAddonCredentials(projectID, addonID string) (map[string]string, error) {
	var out struct {
		Credentials map[string]string `json:"credentials"`
	}
	if err := c.Get(fmt.Sprintf("/api/projects/%s/addons/by-id/%s/credentials", projectID, addonID), &out); err != nil {
		return nil, err
	}
	return out.Credentials, nil
}

func (c *Client) UpdateNATSAuth(projectID, addonID string, body NATSAuthUpdate) (*NATSAuthResult, error) {
	var out NATSAuthResult
	if err := c.Put(fmt.Sprintf("/api/projects/%s/addons/by-id/%s/nats-auth", projectID, addonID), body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
