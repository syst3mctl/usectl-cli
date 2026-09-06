package api

import (
	"fmt"
	"net/url"
	"strconv"
)

// ========== Storage ==========

type StorageUsage struct {
	BytesUsed     int64  `json:"bytes_used,omitempty"`
	HumanReadable string `json:"human_readable,omitempty"`
}

func (c *Client) GetStorageUsage(projectID string) (map[string]interface{}, error) {
	var resp map[string]interface{}
	err := c.Get(fmt.Sprintf("/api/projects/%s/storage/usage", projectID), &resp)
	return resp, err
}

// ========== Cron history ==========

type CronRun struct {
	Name        string  `json:"name"`
	CronName    string  `json:"cron_name"`
	Status      string  `json:"status"`
	StartedAt   *string `json:"started_at,omitempty"`
	CompletedAt *string `json:"completed_at,omitempty"`
	Logs        string  `json:"logs,omitempty"`
}

type CronHistoryResponse struct {
	Runs  []CronRun `json:"runs"`
	Total int       `json:"total"`
	Page  int       `json:"page"`
	Limit int       `json:"limit"`
}

type CronHistoryFilter struct {
	Page   int
	Limit  int
	Status string
	CronID string
	From   string
	To     string
}

func (c *Client) ListCronHistory(projectID string, f CronHistoryFilter) (*CronHistoryResponse, error) {
	q := url.Values{}
	if f.Page > 0 {
		q.Set("page", strconv.Itoa(f.Page))
	}
	if f.Limit > 0 {
		q.Set("limit", strconv.Itoa(f.Limit))
	}
	if f.Status != "" {
		q.Set("status", f.Status)
	}
	if f.CronID != "" {
		q.Set("cron_id", f.CronID)
	}
	if f.From != "" {
		q.Set("from", f.From)
	}
	if f.To != "" {
		q.Set("to", f.To)
	}
	path := fmt.Sprintf("/api/projects/%s/crons/history", projectID)
	if encoded := q.Encode(); encoded != "" {
		path += "?" + encoded
	}
	var resp CronHistoryResponse
	err := c.Get(path, &resp)
	return &resp, err
}

// ========== Active PRs (delete) ==========

func (c *Client) DeleteActivePR(projectID string, prNumber int) error {
	return c.Delete(fmt.Sprintf("/api/projects/%s/prs/%d", projectID, prNumber), nil)
}

// ========== Stack detection ==========

type StackInfo struct {
	Language       string            `json:"language,omitempty"`
	Framework      string            `json:"framework,omitempty"`
	PackageManager string            `json:"package_manager,omitempty"`
	HasDockerfile  bool              `json:"has_dockerfile,omitempty"`
	BuildCommand   string            `json:"build_command,omitempty"`
	StartCommand   string            `json:"start_command,omitempty"`
	OutputDir      string            `json:"output_dir,omitempty"`
	NodeVersion    string            `json:"node_version,omitempty"`
	Extra          map[string]string `json:"extra,omitempty"`
}

func (c *Client) DetectGitHubStack(installationID int64, owner, repo, ref string) (*StackInfo, error) {
	q := url.Values{}
	q.Set("installation_id", strconv.FormatInt(installationID, 10))
	q.Set("owner", owner)
	q.Set("repo", repo)
	q.Set("ref", ref)
	var stack StackInfo
	err := c.Get("/api/github/detect-stack?"+q.Encode(), &stack)
	return &stack, err
}

// ========== Project billing ==========

type ProjectBilling struct {
	BillingStatus     string  `json:"billing_status"`
	BillingInterval   string  `json:"billing_interval"`
	MonthlyPriceCents int     `json:"monthly_price_cents"`
	ResourceTier      string  `json:"resource_tier"`
	VCPU              float64 `json:"vcpu"`
	RAMGB             float64 `json:"ram_gb"`
	StorageGB         float64 `json:"storage_gb"`
}

func (c *Client) GetProjectBilling(projectID string) (*ProjectBilling, error) {
	var b ProjectBilling
	err := c.Get(fmt.Sprintf("/api/projects/%s/billing", projectID), &b)
	return &b, err
}

type ProjectCheckoutRequest struct {
	AmountCents int64   `json:"amount_cents"`
	Interval    string  `json:"interval"`
	VCPU        float64 `json:"vcpu"`
	RAMGB       float64 `json:"ram_gb"`
	StorageGB   float64 `json:"storage_gb"`
	SuccessURL  string  `json:"success_url"`
	CancelURL   string  `json:"cancel_url"`
}

func (c *Client) CreateProjectCheckout(projectID string, req ProjectCheckoutRequest) (string, error) {
	var resp struct {
		URL string `json:"url"`
	}
	err := c.Post(fmt.Sprintf("/api/projects/%s/billing/checkout", projectID), req, &resp)
	return resp.URL, err
}

func (c *Client) CreateProjectBillingPortal(projectID, returnURL string) (string, error) {
	var resp struct {
		URL string `json:"url"`
	}
	err := c.Post(fmt.Sprintf("/api/projects/%s/billing/portal", projectID),
		map[string]string{"return_url": returnURL}, &resp)
	return resp.URL, err
}

// ========== Pricing calculator ==========

type PriceCalculation struct {
	VCPU           float64 `json:"vcpu"`
	RAMGB          float64 `json:"ram_gb"`
	StorageGB      float64 `json:"storage_gb"`
	Interval       string  `json:"interval"`
	IntervalAmount int64   `json:"interval_amount"`
	MonthlyTotal   int64   `json:"monthly_total"`
}

func (c *Client) CalculatePrice(vcpu, ramGB, storageGB float64, interval string) (*PriceCalculation, error) {
	body := map[string]interface{}{
		"vcpu":       vcpu,
		"ram_gb":     ramGB,
		"storage_gb": storageGB,
		"interval":   interval,
	}
	var calc PriceCalculation
	err := c.Post("/api/billing/calculate", body, &calc)
	return &calc, err
}

// ========== Public pricing config ==========

func (c *Client) GetPublicPricing() (map[string]interface{}, error) {
	var resp map[string]interface{}
	err := c.Get("/api/config/pricing", &resp)
	return resp, err
}

// ========== Project domains attach to app + verify ==========

func (c *Client) AttachDomainToApp(domainID, projectAppID string) error {
	body := map[string]interface{}{"project_app_id": projectAppID}
	return c.Put(fmt.Sprintf("/api/domains/%s/attach-app", domainID), body, nil)
}

func (c *Client) DetachDomainFromApp(domainID string) error {
	body := map[string]interface{}{"project_app_id": nil}
	return c.Put(fmt.Sprintf("/api/domains/%s/attach-app", domainID), body, nil)
}

func (c *Client) VerifyDomain(domainID string) (map[string]interface{}, error) {
	var resp map[string]interface{}
	err := c.Get(fmt.Sprintf("/api/domains/%s/verify", domainID), &resp)
	return resp, err
}

// ========== Project domains list ==========

// GetProjectDomains returns the domain NAMES attached to a machine.
//
// The endpoint responds {"domains": ["a.example.com", ...]} — a wrapped array
// of bare strings. The client previously decoded it into a []Domain, which
// silently produced an empty slice and a nil error, so this call has always
// reported "no domains" for machines that had plenty.
//
// Names only: no ids, and no project_app_id. Use ListProjectDomainRecords when
// you need to know which pod a domain is pinned to.
func (c *Client) GetProjectDomains(projectID string) ([]string, error) {
	var resp struct {
		Domains []string `json:"domains"`
	}
	err := c.Get(fmt.Sprintf("/api/projects/%s/domains", projectID), &resp)
	return resp.Domains, err
}

// ListProjectDomainRecords returns full Domain records for one machine,
// including the project_app_id that pins each domain to a pod.
//
// Filtered client-side from the global /api/domains listing because the
// per-project endpoint above returns names only.
func (c *Client) ListProjectDomainRecords(projectID string) ([]Domain, error) {
	all, err := c.ListDomains()
	if err != nil {
		return nil, err
	}
	out := make([]Domain, 0, 8)
	for _, d := range all {
		if d.ProjectID != nil && *d.ProjectID == projectID {
			out = append(out, d)
		}
	}
	return out, nil
}

// ========== Cancel deployment ==========

func (c *Client) CancelDeployment(projectID, deploymentID string) error {
	return c.Post(fmt.Sprintf("/api/projects/%s/deployments/%s/cancel", projectID, deploymentID), nil, nil)
}

// ========== Auth providers ==========

type AuthProviders struct {
	Google bool `json:"google"`
	GitHub bool `json:"github"`
	Email  bool `json:"email"`
}

func (c *Client) GetAuthProviders() (*AuthProviders, error) {
	var p AuthProviders
	err := c.Get("/api/auth/providers", &p)
	return &p, err
}

// ========== Trial status ==========

type TrialStatus struct {
	IsOnTrial   bool   `json:"is_on_trial"`
	TrialEndsAt string `json:"trial_ends_at,omitempty"`
	DaysLeft    int    `json:"days_left,omitempty"`
}

func (c *Client) GetTrialStatus() (*TrialStatus, error) {
	var s TrialStatus
	err := c.Get("/api/users/me/trial-status", &s)
	return &s, err
}
