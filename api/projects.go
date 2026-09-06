package api

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/gorilla/websocket"
	"golang.org/x/term"
)

// ========== Projects ==========

type Project struct {
	ID                string  `json:"id"`
	Name              string  `json:"name"`
	RepoURL           string  `json:"repo_url"`
	Branch            string  `json:"branch"`
	Domain            string  `json:"domain"`
	ProjectType       string  `json:"project_type"`
	Port              int     `json:"port"`
	NeedsDB           bool    `json:"needs_db"`
	NeedsS3           bool    `json:"needs_s3"`
	EnablePreviewEnvs bool    `json:"enable_preview_envs"`
	OwnerID           *string `json:"owner_id,omitempty"`
	DBName            *string `json:"db_name,omitempty"`
	S3Bucket          *string `json:"s3_bucket,omitempty"`
	CreatedAt         string  `json:"created_at"`
	UpdatedAt         string  `json:"updated_at"`
	// Machine-level sizing and billing. RepoURL/Branch/Domain/Port above are
	// legacy single-app fields kept for older machines; new machines carry
	// those per pod instead.
	VCPU              float64 `json:"vcpu"`
	RAMGB             float64 `json:"ram_gb"`
	StorageGB         float64 `json:"storage_gb"`
	BillingStatus     string  `json:"billing_status"`
	BillingInterval   string  `json:"billing_interval"`
	MonthlyPriceCents int     `json:"monthly_price_cents"`
	TrialEndsAt       *string `json:"trial_ends_at,omitempty"`
	BackupSchedule    *string `json:"backup_schedule,omitempty"`
	InstallationID    *int64  `json:"installation_id,omitempty"`
}

type CreateProjectRequest struct {
	Name           string   `json:"name"`
	RepoURL        string   `json:"repo_url"`
	Branch         string   `json:"branch"`
	Domain         string   `json:"domain"`
	ProjectType    string   `json:"project_type"`
	Port           int      `json:"port"`
	NeedsDB        bool     `json:"needs_db"`
	NeedsS3        bool     `json:"needs_s3"`
	GithubToken    string   `json:"github_token,omitempty"`
	InstallationID *int64   `json:"installation_id,omitempty"`
	Addons         []string `json:"addons,omitempty"`
	// Resource sizing. A machine is a quota wallet: vCPU, RAM and storage are
	// the machine's, while repo/branch/domain/port belong to the pods inside
	// it. The API defaults these to 1/1/1 monthly when omitted, which is why
	// the CLI could create machines without them before these fields existed —
	// it just silently got the smallest possible machine.
	VCPU            float64 `json:"vcpu,omitempty"`
	RAMGB           float64 `json:"ram_gb,omitempty"`
	StorageGB       float64 `json:"storage_gb,omitempty"`
	BillingInterval string  `json:"billing_interval,omitempty"` // "month" | "year"
}

type UpdateProjectRequest struct {
	Name              *string `json:"name,omitempty"`
	Domain            *string `json:"domain,omitempty"`
	Branch            *string `json:"branch,omitempty"`
	Port              *int    `json:"port,omitempty"`
	GithubToken       *string `json:"github_token,omitempty"`
	InstallationID    *int64  `json:"installation_id,omitempty"`
	EnablePreviewEnvs *bool   `json:"enable_preview_envs,omitempty"`
}

type Deployment struct {
	ID           string  `json:"id"`
	ProjectID    string  `json:"project_id"`
	CommitHash   string  `json:"commit_hash"`
	ImageTag     string  `json:"image_tag"`
	Status       string  `json:"status"`
	K8sNamespace string  `json:"k8s_namespace,omitempty"`
	BuildLog     *string `json:"build_log,omitempty"`
	DeployLog    *string `json:"deploy_log,omitempty"`
	PRNumber     *int    `json:"pr_number,omitempty"`
	PRBranch     *string `json:"pr_branch,omitempty"`
	PRDomain     *string `json:"pr_domain,omitempty"`
	CreatedAt    string  `json:"created_at"`
	UpdatedAt    string  `json:"updated_at"`

	// Fields the API has grown since this type was written.
	//
	// ProjectAppID scopes a deployment to one app (mig 053); it is null on
	// older project-level rows.
	ProjectAppID *string `json:"project_app_id,omitempty"`
	// CommitMessage is best-effort — absent for image-sourced deployments.
	CommitMessage *string `json:"commit_message,omitempty"`
	// SourceType is "git" or "image" (mig 065). An image-sourced deployment
	// has no commit at all, which is why CommitHash can come back empty.
	SourceType string `json:"source_type,omitempty"`
	// ImageDigest pins the exact image a deployment ran, so a rollback
	// redeploys that build rather than whatever a mutable tag points at now.
	ImageDigest *string `json:"image_digest,omitempty"`
	// RollbackState is the deploy-safety watcher's verdict (mig 048/061):
	// none | watching | passed | rolled_back | manual_rollback | cancelled.
	RollbackState  string  `json:"rollback_state,omitempty"`
	RollbackReason *string `json:"rollback_reason,omitempty"`
	// UpstreamService/Code name the third party that caused a failure
	// (USCT-178/189) — e.g. service "registry", code "registry_misconfigured".
	UpstreamService *string `json:"upstream_service,omitempty"`
	UpstreamCode    *string `json:"upstream_code,omitempty"`
	// ImagePrunedAt is set when retention reclaimed this deployment's image
	// (mig 067). Non-nil means rollback here is no longer possible.
	ImagePrunedAt *string `json:"image_pruned_at,omitempty"`
}

type ProjectWithDeployment struct {
	Project          Project     `json:"project"`
	LatestDeployment *Deployment `json:"latest_deployment,omitempty"`
}

type DeployResponse struct {
	Message    string     `json:"message"`
	Deployment Deployment `json:"deployment"`
}

type ProjectStatus struct {
	Status   string `json:"status"`
	Replicas int    `json:"replicas"`
}

type ProjectStats struct {
	Pods        []PodStats `json:"pods"`
	DBSize      string     `json:"db_size,omitempty"`
	StorageUsed string     `json:"storage_used,omitempty"`
}

type PodStats struct {
	Name     string `json:"name"`
	CPU      string `json:"cpu"`
	Memory   string `json:"memory"`
	NetRx    string `json:"net_rx"`
	NetTx    string `json:"net_tx"`
	Status   string `json:"status"`
	Restarts int32  `json:"restarts"`
}

type LogsResponse struct {
	Logs string `json:"logs"`
}

type DeploymentLogsResponse struct {
	Log string `json:"log"`
}

type DiagnosticsEvent struct {
	Type    string `json:"type"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
	Time    string `json:"time"`
}

type ContainerStatus struct {
	State        string `json:"state"`
	WaitReason   string `json:"reason,omitempty"`
	Message      string `json:"message,omitempty"`
	RestartCount int32  `json:"restart_count"`
}

type DiagnosticResponse struct {
	PodName         string             `json:"pod_name"`
	Phase           string             `json:"phase"`
	ContainerStatus *ContainerStatus   `json:"container_status,omitempty"`
	Events          []DiagnosticsEvent `json:"events"`
	PreviousLogs    string             `json:"previous_logs,omitempty"`
}

func (c *Client) GetDiagnostics(id string) (*DiagnosticResponse, error) {
	var resp DiagnosticResponse
	err := c.Get(fmt.Sprintf("/api/projects/%s/diagnostics", id), &resp)
	return &resp, err
}

func (c *Client) ListProjects() ([]ProjectWithDeployment, error) {
	var resp struct {
		Projects   []ProjectWithDeployment `json:"projects"`
		Total      int                     `json:"total"`
		Page       int                     `json:"page"`
		PerPage    int                     `json:"per_page"`
		TotalPages int                     `json:"total_pages"`
	}
	err := c.Get("/api/projects?per_page=100", &resp)
	return resp.Projects, err
}

func (c *Client) GetProject(id string) (*Project, error) {
	var project Project
	err := c.Get(fmt.Sprintf("/api/projects/%s", id), &project)
	return &project, err
}

func (c *Client) CreateProject(req CreateProjectRequest) (*Project, error) {
	var project Project
	err := c.Post("/api/projects", req, &project)
	return &project, err
}

func (c *Client) UpdateProject(id string, req UpdateProjectRequest) (*Project, error) {
	var project Project
	err := c.Put(fmt.Sprintf("/api/projects/%s", id), req, &project)
	return &project, err
}

func (c *Client) DeleteProject(id string) error {
	return c.Delete(fmt.Sprintf("/api/projects/%s", id), nil)
}

// DeployProject triggers a build+deploy. appID targets one pod; empty asks the
// server to deploy the machine as a whole.
//
// A whole-machine deploy only works when the machine itself carries a repo —
// the legacy single-app shape. On a machine whose repos live per pod (every
// machine created since project_apps), the server has nothing to resolve HEAD
// from and fails with "commit_hash is required (could not auto-resolve HEAD)",
// which is why the CLI deploys pod by pod instead.
func (c *Client) DeployProject(id, appID string) (*DeployResponse, error) {
	var resp DeployResponse
	var body interface{}
	if appID != "" {
		body = map[string]string{"app_id": appID}
	}
	err := c.Post(fmt.Sprintf("/api/projects/%s/deploy", id), body, &resp)
	return &resp, err
}

func (c *Client) StartProject(id string) error {
	return c.Post(fmt.Sprintf("/api/projects/%s/start", id), nil, nil)
}

func (c *Client) StopProject(id string) error {
	return c.Post(fmt.Sprintf("/api/projects/%s/stop", id), nil, nil)
}

func (c *Client) GetProjectStatus(id string) (*ProjectStatus, error) {
	var status ProjectStatus
	err := c.Get(fmt.Sprintf("/api/projects/%s/status", id), &status)
	return &status, err
}

func (c *Client) GetProjectStats(id string) (*ProjectStats, error) {
	var stats ProjectStats
	err := c.Get(fmt.Sprintf("/api/projects/%s/stats", id), &stats)
	return &stats, err
}

// GetRuntimeLogs fetches recent logs. appID narrows to one pod; empty means
// every pod in the machine, which on a multi-pod machine interleaves output
// from unrelated workloads.
func (c *Client) GetRuntimeLogs(id string, lines int, appID string) (*LogsResponse, error) {
	var logs LogsResponse
	q := url.Values{}
	if lines > 0 {
		q.Set("lines", strconv.Itoa(lines))
	}
	if appID != "" {
		q.Set("app_id", appID)
	}
	path := fmt.Sprintf("/api/projects/%s/runtime-logs", id)
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	err := c.Get(path, &logs)
	return &logs, err
}

// StreamRuntimeLogs follows logs in real-time (like docker logs -f).
func (c *Client) StreamRuntimeLogs(id string, lines int, appID string, writer io.Writer) error {
	q := url.Values{"follow": {"true"}}
	if lines > 0 {
		q.Set("lines", strconv.Itoa(lines))
	}
	if appID != "" {
		q.Set("app_id", appID)
	}
	path := fmt.Sprintf("/api/projects/%s/runtime-logs?%s", id, q.Encode())

	// Refresh before opening the stream rather than after a 401: a follow
	// stream can stay open for hours, and reconnecting mid-tail would drop
	// output. http.DefaultClient (not c.httpClient) on purpose — the 30s
	// client timeout would cut the stream off.
	doStream := func() (*http.Response, error) {
		req, err := http.NewRequest("GET", c.BaseURL+path, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+c.Token)
		return http.DefaultClient.Do(req)
	}

	resp, err := doStream()
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusUnauthorized && !c.refreshing && c.RefreshToken != "" {
		resp.Body.Close()
		if rErr := c.refreshAccessToken(); rErr != nil {
			return fmt.Errorf("session expired — run 'usectl login'")
		}
		resp, err = doStream()
		if err != nil {
			return err
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}

	_, err = io.Copy(writer, resp.Body)
	return err
}

func (c *Client) GetDeploymentLogs(projectID, deploymentID string) (*DeploymentLogsResponse, error) {
	var logs DeploymentLogsResponse
	err := c.Get(fmt.Sprintf("/api/projects/%s/deployments/%s/logs", projectID, deploymentID), &logs)
	return &logs, err
}

// ProjectFullResponse is the full response from GET /api/projects/{id}
// which includes the project, latest deployment, and all deployments.
type ProjectFullResponse struct {
	Project          Project      `json:"project"`
	LatestDeployment *Deployment  `json:"latest_deployment,omitempty"`
	Deployments      []Deployment `json:"deployments"`
}

func (c *Client) GetProjectFull(id string) (*ProjectFullResponse, error) {
	var resp ProjectFullResponse
	err := c.Get(fmt.Sprintf("/api/projects/%s", id), &resp)
	return &resp, err
}

type RollbackRequest struct {
	CommitHash string `json:"commit_hash"`
	SkipBuild  bool   `json:"skip_build"`
}

func (c *Client) RollbackProject(projectID, commitHash string) error {
	req := RollbackRequest{
		CommitHash: commitHash,
		SkipBuild:  true,
	}
	return c.Post(fmt.Sprintf("/api/projects/%s/deploy", projectID), req, nil)
}

// ListActivePRs returns active PR preview deployments for a project.
func (c *Client) ListActivePRs(projectID string) ([]Deployment, error) {
	var deployments []Deployment
	err := c.Get(fmt.Sprintf("/api/projects/%s/prs", projectID), &deployments)
	return deployments, err
}

func (c *Client) StreamTerminal(projectID string, podName string) error {
	path := fmt.Sprintf("/api/projects/%s/terminal?token=%s", projectID, url.QueryEscape(c.Token))
	if podName != "" {
		path += fmt.Sprintf("&pod=%s", url.QueryEscape(podName))
	}

	wsURL := c.BaseURL
	if strings.HasPrefix(wsURL, "https://") {
		wsURL = strings.Replace(wsURL, "https://", "wss://", 1)
	} else if strings.HasPrefix(wsURL, "http://") {
		wsURL = strings.Replace(wsURL, "http://", "ws://", 1)
	}
	wsURL += path

	// Put terminal into raw mode
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return fmt.Errorf("failed to set raw terminal mode: %w", err)
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	dialer := websocket.DefaultDialer
	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		return fmt.Errorf("websocket connection failed: %w", err)
	}
	defer conn.Close()

	done := make(chan struct{})

	// Handle STDOUT
	go func() {
		defer close(done)
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				return
			}
			os.Stdout.Write(message)
		}
	}()

	// Handle STDIN
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := os.Stdin.Read(buf)
			if err != nil {
				return
			}
			if n > 0 {
				err = conn.WriteMessage(websocket.TextMessage, buf[:n])
				if err != nil {
					return
				}
			}
		}
	}()

	<-done
	return nil
}
