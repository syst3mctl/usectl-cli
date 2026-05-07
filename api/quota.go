package api

import "fmt"

// ========== Quota / Resources ==========

type QuotaAppView struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	MinReplicas     int    `json:"min_replicas"`
	MaxReplicas     int32  `json:"max_replicas"`
	CurrentReplicas int32  `json:"current_replicas"`
}

type LegacyOversizedPod struct {
	Name              string `json:"name"`
	DeclaredCPUMillis int64  `json:"declared_cpu_millis"`
	DeclaredMemoryMiB int64  `json:"declared_memory_mib"`
}

type SuggestedPlan struct {
	VCPU              float64 `json:"vcpu"`
	RAMGB             float64 `json:"ram_gb"`
	StorageGB         float64 `json:"storage_gb"`
	MonthlyPriceCents int64   `json:"monthly_price_cents"`
}

type QuotaRecommendation struct {
	Action        string         `json:"action"`
	Message       string         `json:"message"`
	SuggestedPlan *SuggestedPlan `json:"suggested_plan,omitempty"`
}

type ProjectQuota struct {
	VCPUTotal               float64              `json:"vcpu_total"`
	RAMGBTotal              float64              `json:"ram_gb_total"`
	StorageGBTotal          float64              `json:"storage_gb_total"`
	VCPUUsedMillis          int64                `json:"vcpu_used_millis"`
	RAMUsedMiB              int64                `json:"ram_used_mib"`
	StorageUsedGiB          int64                `json:"storage_used_gib"`
	PerPodCPUMillis         int                  `json:"per_pod_cpu_millis"`
	PerPodRAMMiB            int                  `json:"per_pod_ram_mib"`
	Apps                    []QuotaAppView       `json:"apps"`
	Applied                 bool                 `json:"applied"`
	Status                  string               `json:"status"`
	AdmissionFailuresRecent int                  `json:"admission_failures_recent"`
	LegacyOversizedPods     []LegacyOversizedPod `json:"legacy_oversized_pods"`
	Recommendation          *QuotaRecommendation `json:"recommendation,omitempty"`
}

func (c *Client) GetProjectQuota(projectID string) (*ProjectQuota, error) {
	var q ProjectQuota
	err := c.Get(fmt.Sprintf("/api/projects/%s/quota", projectID), &q)
	return &q, err
}

// PreviewQuotaChange runs a dry-run of a plan resize.
type QuotaPreviewRequest struct {
	VCPU      float64 `json:"vcpu"`
	RAMGB     float64 `json:"ram_gb"`
	StorageGB float64 `json:"storage_gb"`
}

func (c *Client) PreviewQuotaChange(projectID string, req QuotaPreviewRequest) (map[string]interface{}, error) {
	var resp map[string]interface{}
	err := c.Post(fmt.Sprintf("/api/projects/%s/quota/preview", projectID), req, &resp)
	return resp, err
}

// RolloverLegacyPods restarts oversized legacy pods so they come back under the
// LimitRange's per-pod default.
func (c *Client) RolloverLegacyPods(projectID string) error {
	return c.Post(fmt.Sprintf("/api/projects/%s/quota/rollover-legacy", projectID), nil, nil)
}
