package api

import (
	"fmt"
	"net/url"
)

// Alerts (mig 088): built-in health checks, custom rules and destinations
// on the platform alerting engine. All calls take an optional group id.

type HealthCheck struct {
	Key         string  `json:"key"`
	Label       string  `json:"label"`
	Description string  `json:"description"`
	Threshold   float64 `json:"threshold"`
	For         string  `json:"for"`
	Enabled     bool    `json:"enabled"`
}

type AlertDestination struct {
	UID      string `json:"uid"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Settings struct {
		URL string `json:"url"`
	} `json:"settings"`
}

type AlertRule struct {
	UID                  string            `json:"uid"`
	Title                string            `json:"title"`
	For                  string            `json:"for"`
	Annotations          map[string]string `json:"annotations"`
	NotificationSettings struct {
		Receiver string `json:"receiver"`
	} `json:"notification_settings"`
}

type AlertMetric struct {
	Value       string `json:"value"`
	Label       string `json:"label"`
	Unit        string `json:"unit"`
	Description string `json:"description"`
	Targets     string `json:"targets"`
}

type AlertTarget struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	K8sName string `json:"k8s_name"`
	Kind    string `json:"kind"`
}

func alertsPath(projectID, tail, groupID string) string {
	p := fmt.Sprintf("/api/projects/%s/alerts/%s", projectID, tail)
	if groupID != "" {
		p += "?group_id=" + url.QueryEscape(groupID)
	}
	return p
}

func (c *Client) ListHealthChecks(projectID, groupID string) ([]HealthCheck, bool, error) {
	var r struct {
		Checks    []HealthCheck `json:"checks"`
		Available bool          `json:"available"`
	}
	err := c.Get(alertsPath(projectID, "health", groupID), &r)
	return r.Checks, r.Available, err
}

func (c *Client) SetHealthCheck(projectID, groupID, key string, enabled bool) ([]HealthCheck, error) {
	var r struct {
		Checks []HealthCheck `json:"checks"`
	}
	err := c.Put(alertsPath(projectID, "health", groupID), map[string]any{"key": key, "enabled": enabled}, &r)
	return r.Checks, err
}

func (c *Client) ListAlertDestinations(projectID, groupID string) ([]AlertDestination, error) {
	var out []AlertDestination
	err := c.Get(alertsPath(projectID, "contact-points", groupID), &out)
	return out, err
}

func (c *Client) CreateAlertDestination(projectID, groupID, name, typ, u string) (*AlertDestination, error) {
	var d AlertDestination
	err := c.Post(alertsPath(projectID, "contact-points", groupID), map[string]string{"name": name, "type": typ, "url": u}, &d)
	return &d, err
}

func (c *Client) DeleteAlertDestination(projectID, groupID, uid string) error {
	return c.Delete(alertsPath(projectID, "contact-points/"+uid, groupID), nil)
}

func (c *Client) ListAlertRules(projectID, groupID string) ([]AlertRule, error) {
	var r struct {
		Rules []AlertRule `json:"rules"`
	}
	err := c.Get(alertsPath(projectID, "rules", groupID), &r)
	return r.Rules, err
}

type CreateAlertRuleRequest struct {
	TargetType   string  `json:"target_type"`
	TargetName   string  `json:"target_name"`
	TargetK8s    string  `json:"target_k8s_name"`
	Metric       string  `json:"metric"`
	Expr         string  `json:"expr,omitempty"`
	Label        string  `json:"label,omitempty"`
	Comparator   string  `json:"comparator"`
	Threshold    float64 `json:"threshold"`
	ForDuration  string  `json:"for_duration"`
	ContactPoint string  `json:"contact_point"`
}

func (c *Client) CreateAlertRule(projectID, groupID string, req CreateAlertRuleRequest) (*AlertRule, error) {
	var r AlertRule
	err := c.Post(alertsPath(projectID, "rules", groupID), req, &r)
	return &r, err
}

func (c *Client) DeleteAlertRule(projectID, groupID, uid string) error {
	return c.Delete(alertsPath(projectID, "rules/"+uid, groupID), nil)
}

func (c *Client) ListAlertMetrics(projectID string) ([]AlertMetric, error) {
	var r struct {
		Metrics []AlertMetric `json:"metrics"`
	}
	err := c.Get(alertsPath(projectID, "metrics", ""), &r)
	return r.Metrics, err
}

func (c *Client) ListAlertTargets(projectID, groupID string) ([]AlertTarget, error) {
	var r struct {
		Targets []AlertTarget `json:"targets"`
	}
	err := c.Get(alertsPath(projectID, "targets", groupID), &r)
	return r.Targets, err
}
