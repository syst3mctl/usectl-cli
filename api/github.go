package api

import "fmt"

// ========== GitHub App ==========

type GitHubAppInfo struct {
	ClientID string `json:"client_id"`
}

type GitHubInstallation struct {
	ID      int64 `json:"id"`
	Account struct {
		Login     string `json:"login"`
		AvatarURL string `json:"avatar_url"`
	} `json:"account"`
	AppID      int64  `json:"app_id"`
	TargetType string `json:"target_type"`
}

type GitHubRepo struct {
	ID       int64  `json:"id"`
	FullName string `json:"full_name"`
	Name     string `json:"name"`
	Private  bool   `json:"private"`
	HTMLURL  string `json:"html_url"`
	CloneURL string `json:"clone_url"`
}

type GitHubBranch struct {
	Name      string `json:"name"`
	Protected bool   `json:"protected"`
}

func (c *Client) GetGitHubAppInfo() (*GitHubAppInfo, error) {
	var info GitHubAppInfo
	err := c.Get("/api/github/app-info", &info)
	return &info, err
}

func (c *Client) ExchangeGitHubCode(code string) (string, error) {
	var resp struct {
		GitHubToken string `json:"github_token"`
	}
	err := c.Post("/api/github/callback", map[string]string{"code": code}, &resp)
	return resp.GitHubToken, err
}

func (c *Client) ListGitHubInstallations(githubToken string) ([]GitHubInstallation, error) {
	var installations []GitHubInstallation
	err := c.doWithHeaders("GET", "/api/github/installations", nil, &installations, map[string]string{
		"X-GitHub-Token": githubToken,
	})
	return installations, err
}

func (c *Client) ListGitHubRepos(githubToken string, installationID int64) ([]GitHubRepo, error) {
	var repos []GitHubRepo
	path := fmt.Sprintf("/api/github/repos?installation_id=%d", installationID)
	err := c.doWithHeaders("GET", path, nil, &repos, map[string]string{
		"X-GitHub-Token": githubToken,
	})
	return repos, err
}

func (c *Client) ListGitHubBranches(githubToken string, installationID int64, repo string) ([]GitHubBranch, error) {
	var branches []GitHubBranch
	path := fmt.Sprintf("/api/github/branches?installation_id=%d&repo=%s", installationID, repo)
	err := c.doWithHeaders("GET", path, nil, &branches, map[string]string{
		"X-GitHub-Token": githubToken,
	})
	return branches, err
}
