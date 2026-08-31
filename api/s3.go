package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

// ========== S3 Storage ==========

type S3Object struct {
	Key          string    `json:"key"`
	Size         int64     `json:"size"`
	LastModified time.Time `json:"last_modified"`
	IsDir        bool      `json:"is_dir"`
}

// S3ListResponse is the live manager envelope (same as the frontend).
// Do not also accept a bare []S3Object root.
type S3ListResponse struct {
	Objects  []S3Object `json:"objects"`
	Prefixes []string   `json:"prefixes"`
	Prefix   string     `json:"prefix"`
	Bucket   string     `json:"bucket"`
}

type S3UploadResponse struct {
	Key  string `json:"key"`
	Size int64  `json:"size"`
}

type S3ToggleRequest struct {
	Enable bool `json:"enable"`
}

func (c *Client) ListS3Objects(projectID, prefix string) (*S3ListResponse, error) {
	var out S3ListResponse
	path := fmt.Sprintf("/api/projects/%s/s3/objects", projectID)
	if prefix != "" {
		path += "?prefix=" + url.QueryEscape(prefix)
	}
	err := c.Get(path, &out)
	if out.Objects == nil {
		out.Objects = []S3Object{}
	}
	return &out, err
}

// DownloadS3Object downloads an S3 object and saves it to destPath.
// If destPath is empty, the filename from the key is used in the current directory.
func (c *Client) DownloadS3Object(projectID, key, destPath string) (string, error) {
	path := fmt.Sprintf("/api/projects/%s/s3/objects/download?key=%s", projectID, url.QueryEscape(key))
	resp, err := c.doRaw("GET", path)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if destPath == "" {
		destPath = filepath.Base(key)
	}

	f, err := os.Create(destPath)
	if err != nil {
		return "", fmt.Errorf("create file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}

	return destPath, nil
}

// UploadS3Object PUTs raw file bytes to the manager S3 objects endpoint.
func (c *Client) UploadS3Object(projectID, key, filePath string) (*S3UploadResponse, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat file: %w", err)
	}

	path := fmt.Sprintf("/api/projects/%s/s3/objects?key=%s", projectID, url.QueryEscape(key))
	resp, err := c.doRawBody(http.MethodPut, path, f, "application/octet-stream", st.Size())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var out S3UploadResponse
	if len(body) > 0 {
		if err := json.Unmarshal(body, &out); err != nil {
			return nil, fmt.Errorf("decode response: %w", err)
		}
	}
	if out.Key == "" {
		out.Key = key
	}
	if out.Size == 0 {
		out.Size = st.Size()
	}
	return &out, nil
}

func (c *Client) ToggleS3(projectID string, enable bool) error {
	return c.Post(fmt.Sprintf("/api/projects/%s/s3/toggle", projectID), S3ToggleRequest{Enable: enable}, nil)
}

type S3CdnToggleResponse struct {
	CdnEnabled bool   `json:"cdn_enabled"`
	CdnURL     string `json:"cdn_url"`
	Message    string `json:"message"`
}

func (c *Client) ToggleS3Cdn(projectID string) (*S3CdnToggleResponse, error) {
	var resp S3CdnToggleResponse
	err := c.Post(fmt.Sprintf("/api/projects/%s/s3/cdn/toggle", projectID), nil, &resp)
	return &resp, err
}
