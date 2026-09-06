package api

import (
	"fmt"
	"net/url"
	"time"
)

// Endpoints the CLI previously had no client for. Each corresponds to an API
// route the dashboard already used but that no `usectl` command could reach —
// which meant scripted or agent-driven workflows had to fall back to curl.

// ── Deployments ───────────────────────────────────────────────────────

// DeploymentPage is one page of deployment history.
type DeploymentPage struct {
	Deployments []Deployment `json:"deployments"`
	Total       int          `json:"total"`
	Page        int          `json:"page"`
	PerPage     int          `json:"per_page"`
	TotalPages  int          `json:"total_pages"`
}

// ListDeployments returns a machine's deployment history, newest first.
//
// status and appID are optional filters; page/perPage are optional (the server
// defaults them). perPage is capped at 100 server-side.
func (c *Client) ListDeployments(projectID, status, appID string, page, perPage int) (*DeploymentPage, error) {
	q := url.Values{}
	if status != "" {
		q.Set("status", status)
	}
	if appID != "" {
		q.Set("app_id", appID)
	}
	if page > 0 {
		q.Set("page", fmt.Sprint(page))
	}
	if perPage > 0 {
		q.Set("per_page", fmt.Sprint(perPage))
	}
	path := fmt.Sprintf("/api/projects/%s/deployments", projectID)
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var out DeploymentPage
	if err := c.Get(path, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// RollbackToDeployment redeploys the image a previous deployment ran.
//
// Targets an app rather than the machine: each app has its own image, so a
// machine-wide rollback is not a meaningful operation.
func (c *Client) RollbackToDeployment(projectID, appID, deploymentID, reason string) error {
	body := map[string]string{}
	if reason != "" {
		body["reason"] = reason
	}
	return c.Post(fmt.Sprintf("/api/projects/%s/apps/%s/deployments/%s/rollback",
		projectID, appID, deploymentID), body, nil)
}

// ── Namespace pods ────────────────────────────────────────────────────

// NamespacePod is one Kubernetes pod inside a machine.
type NamespacePod struct {
	Name        string            `json:"name"`
	Phase       string            `json:"phase"`
	Terminating bool              `json:"terminating"`
	Reason      string            `json:"reason,omitempty"`
	Message     string            `json:"message,omitempty"`
	Ready       int               `json:"ready"`
	Total       int               `json:"total"`
	Restarts    int32             `json:"restarts"`
	CreatedAt   time.Time         `json:"created_at"`
	NodeName    string            `json:"node_name,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	OwnerKind   string            `json:"owner_kind,omitempty"`
	OwnerName   string            `json:"owner_name,omitempty"`
	Namespace   string            `json:"namespace,omitempty"`
	GroupName   string            `json:"group_name"`
}

// ListNamespacePods returns every pod across all namespaces the machine owns,
// including addon and group namespaces.
//
// Distinct from `machines pods list`, which reports the app-level stats view:
// this is the raw Kubernetes picture, including addon pods and pods with no
// app of their own.
func (c *Client) ListNamespacePods(projectID string) ([]NamespacePod, error) {
	var out struct {
		Pods []NamespacePod `json:"pods"`
	}
	if err := c.Get(fmt.Sprintf("/api/projects/%s/pods", projectID), &out); err != nil {
		return nil, err
	}
	return out.Pods, nil
}

// DeleteNamespacePod deletes one pod by name.
//
// The controller recreates it, so this is the "restart just this one" tool —
// narrower than `machines pods restart`, which rolls every app in the machine.
func (c *Client) DeleteNamespacePod(projectID, podName string) error {
	return c.Delete(fmt.Sprintf("/api/projects/%s/pods/%s", projectID, url.PathEscape(podName)), nil)
}

// ── Registry usage (USCT-186) ─────────────────────────────────────────

// UploadedImage is one image counted against the registry allowance.
type UploadedImage struct {
	ImageRef  string `json:"image_ref"`
	SizeBytes int64  `json:"size_bytes"`
}

// RegistryUsage is a machine's registry consumption.
type RegistryUsage struct {
	UsedBytes      int64           `json:"used_bytes"`
	AllowanceBytes int64           `json:"allowance_bytes"`
	FreeBytes      int64           `json:"free_bytes"`
	Images         []UploadedImage `json:"images"`
}

// GetRegistryUsage reports how much of the machine's image allowance is used.
func (c *Client) GetRegistryUsage(projectID string) (*RegistryUsage, error) {
	var out RegistryUsage
	if err := c.Get(fmt.Sprintf("/api/projects/%s/registry/usage", projectID), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ── Machine groups (mig 055) ──────────────────────────────────────────

// ProjectGroup partitions a machine's apps and addons into their own namespace.
type ProjectGroup struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	Name      string    `json:"name"`
	Color     *string   `json:"color,omitempty"`
	SortOrder int       `json:"sort_order"`
	CreatedAt time.Time `json:"created_at"`
}

// ListProjectGroups returns a machine's groups.
func (c *Client) ListProjectGroups(projectID string) ([]ProjectGroup, error) {
	// The handler wraps the list: {"groups": [...]}. Decoding straight into a
	// []ProjectGroup failed outright with "cannot unmarshal object into Go
	// value of type []api.ProjectGroup", so `machines groups list` had never
	// worked. Same shape mismatch as GetProjectDomains.
	var out struct {
		Groups []ProjectGroup `json:"groups"`
	}
	if err := c.Get(fmt.Sprintf("/api/projects/%s/groups", projectID), &out); err != nil {
		return nil, err
	}
	return out.Groups, nil
}

// CreateProjectGroup adds a group. Names are lowercased server-side and must
// be DNS-safe, since each group becomes its own namespace.
func (c *Client) CreateProjectGroup(projectID, name, color string, sortOrder *int) (*ProjectGroup, error) {
	body := map[string]any{"name": name}
	if color != "" {
		body["color"] = color
	}
	if sortOrder != nil {
		body["sort_order"] = *sortOrder
	}
	var out ProjectGroup
	if err := c.Post(fmt.Sprintf("/api/projects/%s/groups", projectID), body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteProjectGroup removes a group.
//
// Renaming is not supported: a group is a Kubernetes namespace and namespaces
// cannot be renamed. Delete, recreate, and reassign members.
func (c *Client) DeleteProjectGroup(projectID, groupID string) error {
	return c.Delete(fmt.Sprintf("/api/projects/%s/groups/%s", projectID, groupID), nil)
}
