package api

import "fmt"

// ========== Projects ==========

type Project struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	RepoURL     string  `json:"repo_url"`
	Branch      string  `json:"branch"`
	Domain      string  `json:"domain"`
	ProjectType string  `json:"project_type"`
	Port        int     `json:"port"`
	NeedsDB     bool    `json:"needs_db"`
	NeedsS3     bool    `json:"needs_s3"`
	OwnerID     *string `json:"owner_id,omitempty"`
	DBName      *string `json:"db_name,omitempty"`
	S3Bucket    *string `json:"s3_bucket,omitempty"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
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
}

type UpdateProjectRequest struct {
	Name           *string `json:"name,omitempty"`
	Domain         *string `json:"domain,omitempty"`
	Branch         *string `json:"branch,omitempty"`
	Port           *int    `json:"port,omitempty"`
	GithubToken    *string `json:"github_token,omitempty"`
	InstallationID *int64  `json:"installation_id,omitempty"`
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
	CreatedAt    string  `json:"created_at"`
	UpdatedAt    string  `json:"updated_at"`
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
	BuildLog  string `json:"build_log"`
	DeployLog string `json:"deploy_log"`
}

func (c *Client) ListProjects() ([]ProjectWithDeployment, error) {
	var projects []ProjectWithDeployment
	err := c.Get("/api/projects", &projects)
	return projects, err
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

func (c *Client) DeployProject(id string) (*DeployResponse, error) {
	var resp DeployResponse
	err := c.Post(fmt.Sprintf("/api/projects/%s/deploy", id), nil, &resp)
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

func (c *Client) GetRuntimeLogs(id string, lines int) (*LogsResponse, error) {
	var logs LogsResponse
	path := fmt.Sprintf("/api/projects/%s/runtime-logs", id)
	if lines > 0 {
		path = fmt.Sprintf("%s?lines=%d", path, lines)
	}
	err := c.Get(path, &logs)
	return &logs, err
}

func (c *Client) GetDeploymentLogs(projectID, deploymentID string) (*DeploymentLogsResponse, error) {
	var logs DeploymentLogsResponse
	err := c.Get(fmt.Sprintf("/api/projects/%s/deployments/%s/logs", projectID, deploymentID), &logs)
	return &logs, err
}
