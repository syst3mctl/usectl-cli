package api

import (
	"fmt"
	"io"
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

type S3ToggleRequest struct {
	Enable bool `json:"enable"`
}

func (c *Client) ListS3Objects(projectID, prefix string) ([]S3Object, error) {
	var objects []S3Object
	path := fmt.Sprintf("/api/projects/%s/s3/objects", projectID)
	if prefix != "" {
		path += "?prefix=" + url.QueryEscape(prefix)
	}
	err := c.Get(path, &objects)
	return objects, err
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

func (c *Client) ToggleS3(projectID string, enable bool) error {
	return c.Post(fmt.Sprintf("/api/projects/%s/s3/toggle", projectID), S3ToggleRequest{Enable: enable}, nil)
}
