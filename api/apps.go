package api

import (
	"encoding/json"
	"fmt"
)

// ========== Project Apps (multi-app pods) ==========

type ProjectApp struct {
	ID                 string          `json:"id"`
	ProjectID          string          `json:"project_id"`
	Name               string          `json:"name"`
	RepoURL            string          `json:"repo_url"`
	Branch             string          `json:"branch"`
	Domain             string          `json:"domain"`
	Port               int             `json:"port"`
	Replicas           int             `json:"replicas"`
	InstallationID     *int64          `json:"installation_id,omitempty"`
	RepoFullName       *string         `json:"repo_full_name,omitempty"`
	AutoDeploy         bool            `json:"auto_deploy"`
	EnablePreviewEnvs  bool            `json:"enable_preview_envs"`
	IsPublic           bool            `json:"is_public"`
	IsStopped          bool            `json:"is_stopped"`
	SecretID           *string         `json:"secret_id,omitempty"`
	VarsUpdatedAt      *string         `json:"vars_updated_at,omitempty"`
	Stack              json.RawMessage `json:"stack,omitempty"`
	StackDetectedAt    *string         `json:"stack_detected_at,omitempty"`
	DotenvPath         *string         `json:"dotenv_path,omitempty"`
	DotenvAuto         bool            `json:"dotenv_auto"`
	DefaultBuildTarget *string         `json:"default_build_target,omitempty"`
	BuildDotenvAuto    *bool           `json:"build_dotenv_auto,omitempty"`
	BuildDotenvPath    *string         `json:"build_dotenv_path,omitempty"`
	Kind               string          `json:"kind"`
	Command            *string         `json:"command,omitempty"`
	Args               []string        `json:"args,omitempty"`
	LastDeployAt       *string         `json:"last_deploy_at,omitempty"`
	CreatedAt          string          `json:"created_at"`
	UpdatedAt          string          `json:"updated_at"`
}

type CreateProjectAppRequest struct {
	Name              string   `json:"name"`
	RepoURL           string   `json:"repo_url"`
	Branch            string   `json:"branch,omitempty"`
	Domain            string   `json:"domain,omitempty"`
	Port              int      `json:"port,omitempty"`
	Replicas          int      `json:"replicas,omitempty"`
	InstallationID    *int64   `json:"installation_id,omitempty"`
	RepoFullName      *string  `json:"repo_full_name,omitempty"`
	GithubToken       string   `json:"github_token,omitempty"`
	AutoDeploy        bool     `json:"auto_deploy,omitempty"`
	EnablePreviewEnvs bool     `json:"enable_preview_envs,omitempty"`
	IsPublic          *bool    `json:"is_public,omitempty"`
	Kind              string   `json:"kind,omitempty"`
	Command           string   `json:"command,omitempty"`
	Args              []string `json:"args,omitempty"`
}

type UpdateProjectAppRequest struct {
	Branch            *string  `json:"branch,omitempty"`
	Domain            *string  `json:"domain,omitempty"`
	Port              *int     `json:"port,omitempty"`
	Replicas          *int     `json:"replicas,omitempty"`
	InstallationID    *int64   `json:"installation_id,omitempty"`
	RepoFullName      *string  `json:"repo_full_name,omitempty"`
	AutoDeploy        *bool    `json:"auto_deploy,omitempty"`
	EnablePreviewEnvs *bool    `json:"enable_preview_envs,omitempty"`
	IsPublic          *bool    `json:"is_public,omitempty"`
	DotenvPath        *string  `json:"dotenv_path,omitempty"`
	DotenvAuto        *bool    `json:"dotenv_auto,omitempty"`
	Kind              *string  `json:"kind,omitempty"`
	Command           *string  `json:"command,omitempty"`
	Args              []string `json:"args,omitempty"`
}

type AppInternalAddress struct {
	ServiceName string `json:"service_name"`
	Namespace   string `json:"namespace"`
	Port        int    `json:"port"`
	ShortDNS    string `json:"short_dns"`
	FQDN        string `json:"fqdn"`
	URLShort    string `json:"url_short"`
	URLFQDN     string `json:"url_fqdn"`
}

type VariableEntry struct {
	Key       string `json:"key"`
	Value     string `json:"value"`
	Source    string `json:"source"`
	AddonType string `json:"addon_type,omitempty"`
	Masked    bool   `json:"masked,omitempty"`
}

type AppVariablesResponse struct {
	User   []VariableEntry `json:"user"`
	Addons []VariableEntry `json:"addons"`
}

type AppEnvVarEntry struct {
	Key        string `json:"key"`
	Value      string `json:"value"`
	Source     string `json:"source"`
	Overridden bool   `json:"overridden,omitempty"`
}

type AppEnvsResponse struct {
	Vars []AppEnvVarEntry `json:"vars"`
}

func (c *Client) ListProjectApps(projectID string) ([]ProjectApp, error) {
	var apps []ProjectApp
	err := c.Get(fmt.Sprintf("/api/projects/%s/apps", projectID), &apps)
	return apps, err
}

func (c *Client) CreateProjectApp(projectID string, req CreateProjectAppRequest) (*ProjectApp, error) {
	var app ProjectApp
	err := c.Post(fmt.Sprintf("/api/projects/%s/apps", projectID), req, &app)
	return &app, err
}

// UpdateProjectApp returns the response envelope from PATCH which is
// {"app": ProjectApp, "warning"?: string, "detached_domains"?: int}.
func (c *Client) UpdateProjectApp(projectID, appID string, req UpdateProjectAppRequest) (*ProjectApp, string, error) {
	var resp struct {
		App              ProjectApp `json:"app"`
		Warning          string     `json:"warning,omitempty"`
		DetachedDomains  int        `json:"detached_domains,omitempty"`
	}
	err := c.Patch(fmt.Sprintf("/api/projects/%s/apps/%s", projectID, appID), req, &resp)
	return &resp.App, resp.Warning, err
}

func (c *Client) DeleteProjectApp(projectID, appID string) error {
	return c.Delete(fmt.Sprintf("/api/projects/%s/apps/%s", projectID, appID), nil)
}

func (c *Client) GetAppInternalAddress(projectID, appID string) (*AppInternalAddress, error) {
	var addr AppInternalAddress
	err := c.Get(fmt.Sprintf("/api/projects/%s/apps/%s/internal-address", projectID, appID), &addr)
	return &addr, err
}

func (c *Client) StartApp(projectID, appID string) error {
	return c.Post(fmt.Sprintf("/api/projects/%s/apps/%s/start", projectID, appID), nil, nil)
}

func (c *Client) StopApp(projectID, appID string) error {
	return c.Post(fmt.Sprintf("/api/projects/%s/apps/%s/stop", projectID, appID), nil, nil)
}

func (c *Client) RestartApp(projectID, appID string) error {
	return c.Post(fmt.Sprintf("/api/projects/%s/apps/%s/restart", projectID, appID), nil, nil)
}

func (c *Client) GetAppVariables(projectID, appID string) (*AppVariablesResponse, error) {
	var resp AppVariablesResponse
	err := c.Get(fmt.Sprintf("/api/projects/%s/apps/%s/variables", projectID, appID), &resp)
	return &resp, err
}

// RevealAppVariable returns the unmasked value of an env var for an app.
func (c *Client) RevealAppVariable(projectID, appID, key string) (string, error) {
	var resp struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	err := c.Get(fmt.Sprintf("/api/projects/%s/apps/%s/variables/%s/reveal", projectID, appID, key), &resp)
	return resp.Value, err
}

func (c *Client) ListAppEnvs(projectID, appID string) (*AppEnvsResponse, error) {
	var resp AppEnvsResponse
	err := c.Get(fmt.Sprintf("/api/projects/%s/apps/%s/envs", projectID, appID), &resp)
	return &resp, err
}

func (c *Client) UpdateAppEnvs(projectID, appID string, vars map[string]string) error {
	body := map[string]interface{}{"vars": vars}
	return c.Put(fmt.Sprintf("/api/projects/%s/apps/%s/envs", projectID, appID), body, nil)
}

func (c *Client) DeleteAppEnvs(projectID, appID string, keys []string) error {
	body := map[string]interface{}{"keys": keys}
	return c.DeleteWithBody(fmt.Sprintf("/api/projects/%s/apps/%s/envs", projectID, appID), body, nil)
}

// ListAppAddonAttachments returns addons currently attached to an app.
func (c *Client) ListAppAddonAttachments(projectID, appID string) ([]ProjectAddon, error) {
	var resp struct {
		Addons []ProjectAddon `json:"addons"`
	}
	err := c.Get(fmt.Sprintf("/api/projects/%s/apps/%s/addons", projectID, appID), &resp)
	return resp.Addons, err
}

func (c *Client) AttachAppAddon(projectID, appID, addonID string) error {
	return c.Post(fmt.Sprintf("/api/projects/%s/apps/%s/addons/%s", projectID, appID, addonID), nil, nil)
}

func (c *Client) DetachAppAddon(projectID, appID, addonID string) error {
	return c.Delete(fmt.Sprintf("/api/projects/%s/apps/%s/addons/%s", projectID, appID, addonID), nil)
}

// AppTraffic mirrors the trafficResponse JSON from /traffic.
type AppTraffic struct {
	RequestsTotal          int64              `json:"requests_total"`
	RequestsByCode         map[string]int64   `json:"requests_by_code"`
	RequestsByCodeDetailed map[string]int64   `json:"requests_by_code_detailed"`
	RequestsByMethod       map[string]int64   `json:"requests_by_method"`
	RequestsByProtocol     map[string]int64   `json:"requests_by_protocol"`
	Requests5m             int64              `json:"requests_5m"`
	RequestRate            float64            `json:"request_rate"`
	AvgDurationMs          float64            `json:"avg_duration_ms"`
	P50Ms                  float64            `json:"p50_ms"`
	P95Ms                  float64            `json:"p95_ms"`
	P99Ms                  float64            `json:"p99_ms"`
	BytesInRate            float64            `json:"bytes_in_rate"`
	BytesOutRate           float64            `json:"bytes_out_rate"`
	BytesInTotal           int64              `json:"bytes_in_total"`
	BytesOutTotal          int64              `json:"bytes_out_total"`
	OpenConnections        int64              `json:"open_connections"`
	WindowSeconds          float64            `json:"window_seconds"`
	NoRouters              bool               `json:"no_routers,omitempty"`
	RouterRegex            string             `json:"router_regex"`
	GrafanaURL             string             `json:"grafana_url,omitempty"`
}

func (c *Client) GetAppTraffic(projectID, appID string) (*AppTraffic, error) {
	var t AppTraffic
	err := c.Get(fmt.Sprintf("/api/projects/%s/apps/%s/traffic", projectID, appID), &t)
	return &t, err
}

// AppInsights — slower-changing per-pod metrics + recent error logs.
type SeriesPoint struct {
	T int64   `json:"t"`
	V float64 `json:"v"`
}

type PodResourceHistory struct {
	Pod    string        `json:"pod"`
	CPU    []SeriesPoint `json:"cpu"`
	Memory []SeriesPoint `json:"memory"`
}

type RecentLogEntry struct {
	Timestamp string `json:"timestamp"`
	Pod       string `json:"pod,omitempty"`
	Container string `json:"container,omitempty"`
	Line      string `json:"line"`
}

type AppInsights struct {
	WindowSeconds   int                  `json:"window_seconds"`
	StepSeconds     int                  `json:"step_seconds"`
	ResourceHistory []PodResourceHistory `json:"resource_history"`
	RecentErrors    []RecentLogEntry     `json:"recent_errors"`
	ErrorsAvailable bool                 `json:"errors_available"`
}

func (c *Client) GetAppInsights(projectID, appID string) (*AppInsights, error) {
	var i AppInsights
	err := c.Get(fmt.Sprintf("/api/projects/%s/apps/%s/insights", projectID, appID), &i)
	return &i, err
}
