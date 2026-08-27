package api

import "fmt"

// ========== Environment Variables ==========

// ListEnvs returns the project vault. Values are pointers because a protected
// variable comes back as JSON null: the endpoint's body is a bare map with
// nowhere to carry a flag, so null IS the signal. Decoding into
// map[string]string would silently turn that into "" and hide the difference.
func (c *Client) ListEnvs(projectID string) (map[string]*string, error) {
	var envs map[string]*string
	err := c.Get(fmt.Sprintf("/api/projects/%s/envs", projectID), &envs)
	return envs, err
}

// SetVarProtection marks one project-scoped variable protected or open.
func (c *Client) SetVarProtection(projectID, key string, protected bool) error {
	body := map[string]interface{}{"key": key, "protected": protected}
	return c.Put(fmt.Sprintf("/api/projects/%s/vars/protection", projectID), body, nil)
}

// SetAppVarProtection marks one app-scoped variable protected or open.
func (c *Client) SetAppVarProtection(projectID, appID, key string, protected bool) error {
	body := map[string]interface{}{"key": key, "protected": protected}
	return c.Put(fmt.Sprintf("/api/projects/%s/apps/%s/vars/protection", projectID, appID), body, nil)
}

func (c *Client) SetEnvs(projectID string, vars map[string]string) error {
	body := map[string]interface{}{"vars": vars}
	return c.Put(fmt.Sprintf("/api/projects/%s/envs", projectID), body, nil)
}

func (c *Client) DeleteEnvs(projectID string, keys []string) error {
	body := map[string]interface{}{"keys": keys}
	return c.DeleteWithBody(fmt.Sprintf("/api/projects/%s/envs", projectID), body, nil)
}
