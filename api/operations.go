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
	// Triage holds the failed-deploy diagnosis summary per deployment id
	// (mig 077); absent for rows that were never diagnosed.
	Triage     map[string]TriageSummary `json:"triage,omitempty"`
	Total      int                      `json:"total"`
	Page       int                      `json:"page"`
	PerPage    int                      `json:"per_page"`
	TotalPages int                      `json:"total_pages"`
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
	Name        string `json:"name"`
	Phase       string `json:"phase"`
	Terminating bool   `json:"terminating"`
	Reason      string `json:"reason,omitempty"`
	Message     string `json:"message,omitempty"`
	Ready       int    `json:"ready"`
	Total       int    `json:"total"`
	Restarts    int32  `json:"restarts"`
	// Why the container died last time. Distinct from Reason, which is the
	// CURRENT state: a container that was OOMKilled and restarted reports
	// Running with an empty Reason, so this is the only field that shows an
	// OOM kill after the fact. Empty against an API server older than the
	// change that added it — treat absence as "unknown", not "healthy".
	LastTerminationReason string            `json:"last_termination_reason,omitempty"`
	LastTerminationCode   int32             `json:"last_termination_code,omitempty"`
	LastTerminatedAt      *time.Time        `json:"last_terminated_at,omitempty"`
	CreatedAt             time.Time         `json:"created_at"`
	NodeName              string            `json:"node_name,omitempty"`
	Labels                map[string]string `json:"labels,omitempty"`
	OwnerKind             string            `json:"owner_kind,omitempty"`
	OwnerName             string            `json:"owner_name,omitempty"`
	Namespace             string            `json:"namespace,omitempty"`
	GroupName             string            `json:"group_name"`
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

// ── Failed-deploy triage (mig 077) ─────────────────────────────────────

// TriageSummary is the per-row summary the deployments list carries.
type TriageSummary struct {
	Status     string  `json:"status"`
	Category   string  `json:"category"`
	Title      string  `json:"title"`
	Confidence float64 `json:"confidence"`
}

// TriageAction is a suggested follow-up expressed as an agent tool call.
type TriageAction struct {
	Tool string                 `json:"tool"`
	Args map[string]interface{} `json:"args,omitempty"`
	Why  string                 `json:"why,omitempty"`
}

// DeploymentTriage is the full diagnosis of one failed deployment.
type DeploymentTriage struct {
	DeploymentID     string         `json:"deployment_id"`
	Status           string         `json:"status"`
	Category         string         `json:"category"`
	Title            string         `json:"title"`
	RootCause        string         `json:"root_cause"`
	Evidence         []string       `json:"evidence"`
	Fix              []string       `json:"fix"`
	Actions          []TriageAction `json:"usectl_actions"`
	Confidence       float64        `json:"confidence"`
	Model            string         `json:"model"`
	DurationMs       int            `json:"duration_ms"`
	GitHubCommentURL *string        `json:"github_comment_url,omitempty"`
	Error            *string        `json:"error,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
}

// GetDeploymentTriage returns the diagnosis, or an API 404 when none exists.
func (c *Client) GetDeploymentTriage(projectID, deploymentID string) (*DeploymentTriage, error) {
	var t DeploymentTriage
	if err := c.Get(fmt.Sprintf("/api/projects/%s/deployments/%s/triage", projectID, deploymentID), &t); err != nil {
		return nil, err
	}
	return &t, nil
}

// TriageSettings are the per-machine switches.
type TriageSettings struct {
	Enabled        bool `json:"enabled"`
	GitHubComments bool `json:"github_comments"`
}

func (c *Client) GetTriageSettings(projectID string) (*TriageSettings, error) {
	var s TriageSettings
	err := c.Get(fmt.Sprintf("/api/projects/%s/triage-settings", projectID), &s)
	return &s, err
}

func (c *Client) PutTriageSettings(projectID string, enabled, githubComments *bool) (*TriageSettings, error) {
	body := map[string]interface{}{}
	if enabled != nil {
		body["enabled"] = *enabled
	}
	if githubComments != nil {
		body["github_comments"] = *githubComments
	}
	var s TriageSettings
	err := c.Put(fmt.Sprintf("/api/projects/%s/triage-settings", projectID), body, &s)
	return &s, err
}
