package api

import "fmt"

// ========== usectl AI: usage + budget (mig 057 / 083) ==========

type AIUsageDay struct {
	Period           string `json:"period"`
	Model            string `json:"model"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
	TotalTokens      int64  `json:"total_tokens"`
	Requests         int64  `json:"requests"`
}

type AIUsageResponse struct {
	Usage       []AIUsageDay `json:"usage"`
	TotalTokens int64        `json:"total_tokens"`
}

func (c *Client) GetAIUsage(projectID string) (*AIUsageResponse, error) {
	var r AIUsageResponse
	err := c.Get(fmt.Sprintf("/api/projects/%s/ai/usage", projectID), &r)
	return &r, err
}

// AIBudget mirrors the API row. MonthlyTokens nil = no cap.
type AIBudget struct {
	MonthlyTokens        *int64 `json:"monthly_tokens"`
	OnExceed             string `json:"on_exceed"` // block | fallback
	WarnPct              int    `json:"warn_pct"`
	AlertsEnabled        bool   `json:"alerts_enabled"`
	PricePerMillionCents *int   `json:"price_per_million_cents"`
}

type AIBudgetStatus struct {
	Budget            *AIBudget `json:"budget"`
	UsedTokens        int64     `json:"used_tokens"`
	Pct               float64   `json:"pct"`
	MonthStart        string    `json:"month_start"`
	ResetsAt          string    `json:"resets_at"`
	ProjectedTokens   int64     `json:"projected_tokens"`
	EstimatedCents    *int64    `json:"estimated_cents,omitempty"`
	PlatformCapTokens int64     `json:"platform_cap_tokens,omitempty"`
}

func (c *Client) GetAIBudget(projectID string) (*AIBudgetStatus, error) {
	var r AIBudgetStatus
	err := c.Get(fmt.Sprintf("/api/projects/%s/ai/budget", projectID), &r)
	return &r, err
}

func (c *Client) PutAIBudget(projectID string, b AIBudget) (*AIBudgetStatus, error) {
	var r AIBudgetStatus
	err := c.Put(fmt.Sprintf("/api/projects/%s/ai/budget", projectID), b, &r)
	return &r, err
}

func (c *Client) DeleteAIBudget(projectID string) error {
	return c.Delete(fmt.Sprintf("/api/projects/%s/ai/budget", projectID), nil)
}
