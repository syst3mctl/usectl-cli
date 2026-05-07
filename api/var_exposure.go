package api

import (
	"fmt"
	"net/url"
)

// ========== Build-time variable exposure (PROMPT_explicit_build_arg_checkbox.md) ==========

type ProjectVarDefaultsRequest struct {
	DefaultBuildTarget string  `json:"default_build_target"`
	BuildDotenvAuto    bool    `json:"build_dotenv_auto"`
	BuildDotenvPath    *string `json:"build_dotenv_path,omitempty"`
}

// AppVarDefaultsRequest — every field is nullable. null clears the override
// (i.e. inherits from the project default).
type AppVarDefaultsRequest struct {
	DefaultBuildTarget *string `json:"default_build_target"`
	BuildDotenvAuto    *bool   `json:"build_dotenv_auto"`
	BuildDotenvPath    *string `json:"build_dotenv_path"`
}

type VarExposure struct {
	Key         string `json:"key"`
	BuildTarget string `json:"build_target"`
	Runtime     bool   `json:"runtime"`
}

type ExposureResolved struct {
	DefaultBuildTarget string  `json:"default_build_target"`
	BuildDotenvAuto    bool    `json:"build_dotenv_auto"`
	BuildDotenvPath    *string `json:"build_dotenv_path,omitempty"`
}

type AppExposureResponse struct {
	Project   ExposureResolved      `json:"project"`
	App       AppVarDefaultsRequest `json:"app"`
	Resolved  ExposureResolved      `json:"resolved"`
	Overrides []VarExposure         `json:"overrides"`
}

func (c *Client) UpdateProjectVarDefaults(projectID string, req ProjectVarDefaultsRequest) error {
	return c.Put(fmt.Sprintf("/api/projects/%s/vars/defaults", projectID), req, nil)
}

func (c *Client) UpdateAppVarDefaults(projectID, appID string, req AppVarDefaultsRequest) error {
	return c.Put(fmt.Sprintf("/api/projects/%s/apps/%s/vars/defaults", projectID, appID), req, nil)
}

func (c *Client) GetAppVarExposure(projectID, appID string) (*AppExposureResponse, error) {
	var resp AppExposureResponse
	err := c.Get(fmt.Sprintf("/api/projects/%s/apps/%s/vars/exposure", projectID, appID), &resp)
	return &resp, err
}

func (c *Client) UpsertVarExposure(projectID, appID string, exp VarExposure) error {
	return c.Put(fmt.Sprintf("/api/projects/%s/apps/%s/vars/exposure", projectID, appID), exp, nil)
}

// DeleteVarExposure removes the per-key override (key passed as ?key= query param).
func (c *Client) DeleteVarExposure(projectID, appID, key string) error {
	path := fmt.Sprintf("/api/projects/%s/apps/%s/vars/exposure?key=%s",
		projectID, appID, url.QueryEscape(key))
	return c.Delete(path, nil)
}
