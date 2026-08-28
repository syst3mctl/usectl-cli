package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
)

// Chunked image upload (USCT-184).
//
// The tarball is uploaded in parts THROUGH the API rather than straight to
// object storage: SeaWeedFS sits on a private network the client cannot reach,
// and exposing it is not an option. Parts keep each request under the CDN's
// request-body cap and short enough not to trip the server's read timeout,
// and they give progress and resume for free.

type ImageUploadTicket struct {
	UploadKey string `json:"upload_key"`
	UploadID  string `json:"upload_id"`
	PartSize  int64  `json:"part_size"`
	MaxBytes  int64  `json:"max_bytes"`
	MaxParts  int    `json:"max_parts"`
}

type ImagePart struct {
	PartNumber int    `json:"part_number"`
	ETag       string `json:"etag"`
}

type ImagePushResult struct {
	Message  string `json:"message"`
	Job      string `json:"job"`
	ImageRef string `json:"image_ref"`
	Size     int64  `json:"size"`
}

// StartImageUpload opens a multipart upload and returns the chunking plan.
func (c *Client) StartImageUpload(projectID, appID string) (*ImageUploadTicket, error) {
	var t ImageUploadTicket
	err := c.Post(fmt.Sprintf("/api/projects/%s/apps/%s/image-uploads", projectID, appID), nil, &t)
	return &t, err
}

// UploadImageParts streams `path` to the API one part at a time.
//
// Reads each part into memory before sending because the request must carry an
// exact Content-Length — the storage layer signs each part against its length,
// and a chunked body cannot be signed. Part size is chosen by the server and is
// tens of MB, so this is bounded and predictable rather than proportional to
// the image.
func (c *Client) UploadImageParts(projectID, appID string, t *ImageUploadTicket, path string, progress func(sent, total int64)) ([]ImagePart, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open image tar: %w", err)
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat image tar: %w", err)
	}
	total := st.Size()
	if t.MaxBytes > 0 && total > t.MaxBytes {
		return nil, fmt.Errorf("image is %d bytes; the limit is %d", total, t.MaxBytes)
	}

	partSize := t.PartSize
	if partSize <= 0 {
		partSize = 32 << 20
	}

	var parts []ImagePart
	var sent int64
	buf := make([]byte, partSize)

	for partNumber := 1; ; partNumber++ {
		n, readErr := io.ReadFull(f, buf)
		if n == 0 {
			if readErr == io.EOF {
				break
			}
			if readErr != nil && readErr != io.ErrUnexpectedEOF {
				return nil, fmt.Errorf("read image tar: %w", readErr)
			}
			break
		}
		if t.MaxParts > 0 && partNumber > t.MaxParts {
			return nil, fmt.Errorf("image needs more than %d parts", t.MaxParts)
		}

		part, err := c.uploadOnePart(projectID, appID, t, partNumber, buf[:n])
		if err != nil {
			return nil, err
		}
		parts = append(parts, part)

		sent += int64(n)
		if progress != nil {
			progress(sent, total)
		}

		if readErr == io.EOF || readErr == io.ErrUnexpectedEOF {
			break
		}
	}

	if len(parts) == 0 {
		return nil, fmt.Errorf("the image file is empty")
	}
	return parts, nil
}

func (c *Client) uploadOnePart(projectID, appID string, t *ImageUploadTicket, partNumber int, chunk []byte) (ImagePart, error) {
	endpoint := fmt.Sprintf("%s/api/projects/%s/apps/%s/image-uploads/parts?upload_key=%s&upload_id=%s&part=%d",
		c.BaseURL, projectID, appID, url.QueryEscape(t.UploadKey), url.QueryEscape(t.UploadID), partNumber)

	req, err := http.NewRequest(http.MethodPut, endpoint, bytes.NewReader(chunk))
	if err != nil {
		return ImagePart{}, err
	}
	req.ContentLength = int64(len(chunk))
	req.Header.Set("Content-Type", "application/octet-stream")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ImagePart{}, fmt.Errorf("upload part %d: %w", partNumber, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var e struct {
			Error   string `json:"error"`
			Message string `json:"message"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&e)
		msg := e.Message
		if msg == "" {
			msg = e.Error
		}
		if msg == "" {
			msg = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}
		return ImagePart{}, fmt.Errorf("upload part %d: %s", partNumber, msg)
	}

	var out ImagePart
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return ImagePart{}, fmt.Errorf("upload part %d: bad response: %w", partNumber, err)
	}
	return out, nil
}

// CompleteImageUpload assembles the parts and starts the registry push.
func (c *Client) CompleteImageUpload(projectID, appID, uploadKey, uploadID, tag string, parts []ImagePart) (*ImagePushResult, error) {
	var out ImagePushResult
	body := map[string]any{
		"upload_key": uploadKey,
		"upload_id":  uploadID,
		"parts":      parts,
		"tag":        tag,
	}
	err := c.Post(fmt.Sprintf("/api/projects/%s/apps/%s/image-uploads/complete", projectID, appID), body, &out)
	return &out, err
}

// AbortImageUpload discards a partial upload. An abandoned multipart upload
// holds storage that an object listing does not show, so the orphan sweeper
// cannot reclaim it — only an explicit abort does.
func (c *Client) AbortImageUpload(projectID, appID, uploadKey, uploadID string) error {
	path := fmt.Sprintf("/api/projects/%s/apps/%s/image-uploads?upload_key=%s&upload_id=%s",
		projectID, appID, url.QueryEscape(uploadKey), url.QueryEscape(uploadID))
	return c.Delete(path, nil)
}
