package api

import (
	"encoding/json"
	"fmt"
	"time"
)

// ========== Project Apps (multi-app pods) ==========

type ProjectApp struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	Name      string `json:"name"`
	// SourceType (mig 065) is "git" or "image". An image-sourced pod has no
	// repo, no branch and no GitHub App involvement, and skips the builder
	// entirely. Empty means git, for pods predating the migration.
	SourceType         string          `json:"source_type,omitempty"`
	ImageRef           string          `json:"image_ref,omitempty"`
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
	// Per-app pod sizing (mig 051 + mig 052). nil = app has not opted in
	// (legacy default applies: 256 MiB / 250m / 2 GiB ephemeral).
	MemoryMiB  *int `json:"memory_mib,omitempty"`
	CPUMillis  *int `json:"cpu_millis,omitempty"`
	StorageMiB *int `json:"storage_mib,omitempty"`
	// mig 054: "rolling" or "recreate". nil = inherit default (rolling).
	RolloutStrategy *string `json:"rollout_strategy,omitempty"`
	// mig 059: extra cluster-internal-only ports.
	ExtraPorts []AppPort `json:"extra_ports,omitempty"`
	// mig 060: metrics scraping. MetricsPort nil = the app's primary port.
	MetricsEnabled bool    `json:"metrics_enabled"`
	MetricsPort    *int    `json:"metrics_port,omitempty"`
	MetricsPath    string  `json:"metrics_path,omitempty"`
	LastDeployAt   *string `json:"last_deploy_at,omitempty"`
	CreatedAt      string  `json:"created_at"`
	UpdatedAt      string  `json:"updated_at"`
}

// AppPort is one additional cluster-internal-only port on an app pod (mig 059).
type AppPort struct {
	Name     string `json:"name"`
	Port     int    `json:"port"`
	Protocol string `json:"protocol"`
}

type CreateProjectAppRequest struct {
	Name string `json:"name"`
	// SourceType (mig 065): "git" builds from RepoURL, "image" deploys
	// ImageRef with no build. Empty means git, so existing callers are
	// unaffected.
	SourceType string `json:"source_type,omitempty"`
	ImageRef   string `json:"image_ref,omitempty"`
	// Private-registry credentials. Sent once, stored server-side in the
	// vault, never returned by any read.
	RegistryUsername  string   `json:"registry_username,omitempty"`
	RegistryPassword  string   `json:"registry_password,omitempty"`
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
	// mig 059: extra cluster-internal-only ports (web pods only).
	ExtraPorts []AppPort `json:"extra_ports,omitempty"`
	// mig 060: scrape this app's own /metrics endpoint (web pods only).
	MetricsEnabled bool   `json:"metrics_enabled,omitempty"`
	MetricsPort    *int   `json:"metrics_port,omitempty"`
	MetricsPath    string `json:"metrics_path,omitempty"`
}

type UpdateProjectAppRequest struct {
	// SourceType (mig 065) switches an app between a repo and a prebuilt
	// image. Switching to "image" clears the repo link and disables
	// auto-deploy + preview envs server-side, which is why the response
	// carries SourceSwitchWarning — GitHub pushes silently stop deploying
	// the pod otherwise.
	SourceType        *string  `json:"source_type,omitempty"`
	ImageRef          *string  `json:"image_ref,omitempty"`
	RepoURL           *string  `json:"repo_url,omitempty"`
	RegistryUsername  *string  `json:"registry_username,omitempty"`
	RegistryPassword  *string  `json:"registry_password,omitempty"`
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
	// mig 059: nil = leave unchanged; non-nil (incl. empty) replaces the
	// extra-port list wholesale.
	ExtraPorts *[]AppPort `json:"extra_ports,omitempty"`
	// mig 060: nil = leave unchanged. MetricsPort 0 resets to the app's
	// primary port; MetricsPath "" resets to /metrics.
	MetricsEnabled *bool   `json:"metrics_enabled,omitempty"`
	MetricsPort    *int    `json:"metrics_port,omitempty"`
	MetricsPath    *string `json:"metrics_path,omitempty"`
}

// AppAddressPort is one port entry in an app's internal address (mig 059).
type AppAddressPort struct {
	Name     string `json:"name"`
	Port     int    `json:"port"`
	Protocol string `json:"protocol"`
	URLShort string `json:"url_short"`
	URLFQDN  string `json:"url_fqdn"`
}

type AppInternalAddress struct {
	ServiceName string           `json:"service_name"`
	Namespace   string           `json:"namespace"`
	Port        int              `json:"port"`
	ShortDNS    string           `json:"short_dns"`
	FQDN        string           `json:"fqdn"`
	URLShort    string           `json:"url_short"`
	URLFQDN     string           `json:"url_fqdn"`
	Ports       []AppAddressPort `json:"ports,omitempty"`
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
	Key string `json:"key"`
	// Value is nil when the variable is Protected — the API deliberately
	// sends JSON null rather than "" so a caller piping `--json` into a
	// .env file can tell "not allowed to read this" apart from "set to
	// empty" and fail loudly instead of writing a blank credential.
	Value      *string `json:"value"`
	Source     string  `json:"source"`
	Overridden bool    `json:"overridden,omitempty"`
	Protected  bool    `json:"protected,omitempty"`

	// Provenance (USCT-192). Nil for keys unchanged since the change log
	// shipped. Carried on protected entries too — the log records key names
	// and actors, never values, so there is nothing here to withhold.
	UpdatedAt *time.Time   `json:"updated_at,omitempty"`
	UpdatedBy *EnvVarActor `json:"updated_by,omitempty"`
}

// EnvVarActor is who last changed a variable.
type EnvVarActor struct {
	UserID string `json:"user_id,omitempty"`
	Name   string `json:"name,omitempty"`
	Email  string `json:"email,omitempty"`
	Source string `json:"source"`
}

// DisplayActor renders the actor for human-readable output, falling back
// through name → email → the source alone, so a deleted account still shows
// where the change came from rather than an empty column.
func (e AppEnvVarEntry) DisplayActor() string {
	if e.UpdatedBy == nil {
		return ""
	}
	switch {
	case e.UpdatedBy.Name != "":
		return e.UpdatedBy.Name
	case e.UpdatedBy.Email != "":
		return e.UpdatedBy.Email
	default:
		return e.UpdatedBy.Source
	}
}

// DisplayValue renders one entry for human-readable output. Protected values
// are never printed — not even partially — because this text lands in
// terminals, CI logs and scrollback.
func (e AppEnvVarEntry) DisplayValue() string {
	if e.Protected || e.Value == nil {
		return "(protected)"
	}
	return *e.Value
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
		App                 ProjectApp `json:"app"`
		Warning             string     `json:"warning,omitempty"`
		SourceSwitchWarning string     `json:"source_switch_warning,omitempty"`
		DetachedDomains     int        `json:"detached_domains,omitempty"`
	}
	err := c.Patch(fmt.Sprintf("/api/projects/%s/apps/%s", projectID, appID), req, &resp)
	// Prefer the source-switch warning: it reports that GitHub pushes have
	// stopped deploying this pod, which the user cannot infer from anything
	// else in the response.
	warning := resp.Warning
	if resp.SourceSwitchWarning != "" {
		warning = resp.SourceSwitchWarning
	}
	return &resp.App, warning, err
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

// ResizeAppRequest is the body of PATCH /api/projects/{id}/apps/{appId}/resources.
// All pointers are optional; pass only the dimension you want to change.
type ResizeAppRequest struct {
	MemoryMiB       *int    `json:"memory_mib,omitempty"`
	CPUMillis       *int    `json:"cpu_millis,omitempty"`
	StorageMiB      *int    `json:"storage_mib,omitempty"`
	RolloutStrategy *string `json:"rollout_strategy,omitempty"` // mig 054
}

// ResizeAppResponse is the success envelope. Strategy is "in_place" when
// live pods got the new size with no restart, or "rolling_restart" when
// the kubelet rejected the in-place patch and the Deployment template was
// updated (causing a rolling restart). "noop" and "deferred" are edge cases.
type ResizeAppResponse struct {
	App      ProjectApp `json:"app"`
	Strategy string     `json:"strategy"`
	Message  string     `json:"message"`
}

// ResizeApp issues PATCH /api/projects/{id}/apps/{appId}/resources. On
// quota failure the backend returns a 409 with {"error":"plan_too_small",
// "message": ..., "detail": ...}; the client surfaces the message through
// the returned error (full detail is in the response body, not parsed here
// — the CLI prints the message and exits non-zero).
func (c *Client) ResizeApp(projectID, appID string, req ResizeAppRequest) (*ResizeAppResponse, error) {
	var resp ResizeAppResponse
	err := c.Patch(fmt.Sprintf("/api/projects/%s/apps/%s/resources", projectID, appID), req, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
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
	RequestsTotal          int64            `json:"requests_total"`
	RequestsByCode         map[string]int64 `json:"requests_by_code"`
	RequestsByCodeDetailed map[string]int64 `json:"requests_by_code_detailed"`
	RequestsByMethod       map[string]int64 `json:"requests_by_method"`
	RequestsByProtocol     map[string]int64 `json:"requests_by_protocol"`
	Requests5m             int64            `json:"requests_5m"`
	RequestRate            float64          `json:"request_rate"`
	AvgDurationMs          float64          `json:"avg_duration_ms"`
	P50Ms                  float64          `json:"p50_ms"`
	P95Ms                  float64          `json:"p95_ms"`
	P99Ms                  float64          `json:"p99_ms"`
	BytesInRate            float64          `json:"bytes_in_rate"`
	BytesOutRate           float64          `json:"bytes_out_rate"`
	BytesInTotal           int64            `json:"bytes_in_total"`
	BytesOutTotal          int64            `json:"bytes_out_total"`
	OpenConnections        int64            `json:"open_connections"`
	WindowSeconds          float64          `json:"window_seconds"`
	NoRouters              bool             `json:"no_routers,omitempty"`
	RouterRegex            string           `json:"router_regex"`
	GrafanaURL             string           `json:"grafana_url,omitempty"`
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
