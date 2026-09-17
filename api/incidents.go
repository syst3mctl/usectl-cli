package api

import (
	"fmt"
	"net/url"
	"time"
)

// ========== Incidents: alert → agent (mig 078) ==========

type IncidentAction struct {
	ID         string                 `json:"id"`
	Seq        int                    `json:"seq"`
	Tool       string                 `json:"tool"`
	Args       map[string]interface{} `json:"args"`
	Why        string                 `json:"why"`
	Risk       string                 `json:"risk"`
	Status     string                 `json:"status"`
	ApprovedBy *string                `json:"approved_by,omitempty"`
	ExecutedAt *time.Time             `json:"executed_at,omitempty"`
	Result     *string                `json:"result,omitempty"`
}

type Incident struct {
	ID                     string            `json:"id"`
	ProjectID              *string           `json:"project_id,omitempty"`
	AppID                  *string           `json:"app_id,omitempty"`
	Source                 string            `json:"source"`
	AlertName              string            `json:"alert_name"`
	Labels                 map[string]string `json:"labels"`
	Values                 string            `json:"values"`
	StartedAt              time.Time         `json:"started_at"`
	ResolvedAt             *time.Time        `json:"resolved_at,omitempty"`
	RefiredCount           int               `json:"refired_count"`
	Status                 string            `json:"status"`
	Severity               string            `json:"severity"`
	Headline               string            `json:"headline"`
	Summary                string            `json:"summary"`
	ProbableCause          string            `json:"probable_cause"`
	CorrelatedDeploymentID *string           `json:"correlated_deployment_id,omitempty"`
	Evidence               []string          `json:"evidence"`
	Confidence             float64           `json:"confidence"`
	Model                  string            `json:"model"`
	PolicyMode             string            `json:"policy_mode"`
	Error                  *string           `json:"error,omitempty"`
	Actions                []IncidentAction  `json:"actions"`
	CreatedAt              time.Time         `json:"created_at"`
}

func (c *Client) ListIncidents(projectID, status string, limit int) ([]Incident, error) {
	q := url.Values{}
	if status != "" {
		q.Set("status", status)
	}
	if limit > 0 {
		q.Set("limit", fmt.Sprint(limit))
	}
	path := fmt.Sprintf("/api/projects/%s/incidents", projectID)
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var out struct {
		Incidents []Incident `json:"incidents"`
	}
	if err := c.Get(path, &out); err != nil {
		return nil, err
	}
	return out.Incidents, nil
}

func (c *Client) GetIncident(projectID, incidentID string) (*Incident, error) {
	var in Incident
	if err := c.Get(fmt.Sprintf("/api/projects/%s/incidents/%s", projectID, incidentID), &in); err != nil {
		return nil, err
	}
	return &in, nil
}

// ApproveIncidentAction executes a proposed action as the caller.
func (c *Client) ApproveIncidentAction(projectID, incidentID, actionID string) (*IncidentAction, error) {
	var a IncidentAction
	if err := c.Post(fmt.Sprintf("/api/projects/%s/incidents/%s/actions/%s/approve", projectID, incidentID, actionID), nil, &a); err != nil {
		return nil, err
	}
	return &a, nil
}

func (c *Client) RejectIncidentAction(projectID, incidentID, actionID string) (*IncidentAction, error) {
	var a IncidentAction
	if err := c.Post(fmt.Sprintf("/api/projects/%s/incidents/%s/actions/%s/reject", projectID, incidentID, actionID), nil, &a); err != nil {
		return nil, err
	}
	return &a, nil
}

type IncidentPolicy struct {
	Mode            string   `json:"mode"`
	AllowedTools    []string `json:"allowed_tools"`
	MinConfidence   float64  `json:"min_confidence"`
	MaxAutoPerDay   int      `json:"max_auto_per_day"`
	CooldownMinutes int      `json:"cooldown_minutes"`
}

func (c *Client) GetIncidentPolicy(projectID string) (*IncidentPolicy, error) {
	var p IncidentPolicy
	err := c.Get(fmt.Sprintf("/api/projects/%s/incident-policy", projectID), &p)
	return &p, err
}

func (c *Client) PutIncidentPolicy(projectID string, patch map[string]interface{}) (*IncidentPolicy, error) {
	var p IncidentPolicy
	err := c.Put(fmt.Sprintf("/api/projects/%s/incident-policy", projectID), patch, &p)
	return &p, err
}
