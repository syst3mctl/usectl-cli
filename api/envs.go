package api

import "fmt"

// ========== Environment Variables ==========

func (c *Client) ListEnvs(projectID string) (map[string]string, error) {
	var envs map[string]string
	err := c.Get(fmt.Sprintf("/api/projects/%s/envs", projectID), &envs)
	return envs, err
}

func (c *Client) SetEnvs(projectID string, vars map[string]string) error {
	body := map[string]interface{}{"vars": vars}
	return c.Put(fmt.Sprintf("/api/projects/%s/envs", projectID), body, nil)
}

func (c *Client) DeleteEnvs(projectID string, keys []string) error {
	body := map[string]interface{}{"keys": keys}
	return c.DeleteWithBody(fmt.Sprintf("/api/projects/%s/envs", projectID), body, nil)
}
