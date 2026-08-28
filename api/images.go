package api

import (
	"fmt"
	"io"
	"net/http"
	"os"
)

// Direct image upload (USCT-172 follow-up).
//
// Three steps, because the tarball must not travel through the API server:
// StartImageUpload returns a presigned URL, the client PUTs straight to object
// storage, then CompleteImageUpload asks the platform to push it into the
// registry. Images run 200MB–1GB+ and the API pod also serves log streams and
// web terminals.

type ImageUploadTicket struct {
	UploadURL string `json:"upload_url"`
	UploadKey string `json:"upload_key"`
	ExpiresAt string `json:"expires_at"`
	MaxBytes  int64  `json:"max_bytes"`
}

type ImagePushResult struct {
	Message  string `json:"message"`
	Job      string `json:"job"`
	ImageRef string `json:"image_ref"`
}

// StartImageUpload asks for a presigned upload URL.
func (c *Client) StartImageUpload(projectID, appID string) (*ImageUploadTicket, error) {
	var t ImageUploadTicket
	err := c.Post(fmt.Sprintf("/api/projects/%s/apps/%s/image-uploads", projectID, appID), nil, &t)
	return &t, err
}

// UploadImageTar PUTs the tarball straight to object storage.
//
// Deliberately uses a bare http.Client rather than the API client: the
// presigned URL carries its own authorisation, and attaching our bearer token
// to a third-party storage endpoint would leak it. `progress` is called with
// bytes sent so a TTY can render a bar; pass nil for silence.
func UploadImageTar(uploadURL, path string, progress func(sent, total int64)) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open image tar: %w", err)
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat image tar: %w", err)
	}

	var body io.Reader = f
	if progress != nil {
		body = &progressReader{r: f, total: st.Size(), fn: progress}
	}

	req, err := http.NewRequest(http.MethodPut, uploadURL, body)
	if err != nil {
		return err
	}
	// Required: a presigned PUT is signed for an exact length, and a chunked
	// upload would not match the signature.
	req.ContentLength = st.Size()
	req.Header.Set("Content-Type", "application/x-tar")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("upload failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// The storage error body is not ours and may be verbose XML; report
		// the status and keep the body out of the user's terminal.
		return fmt.Errorf("upload rejected by storage (HTTP %d)", resp.StatusCode)
	}
	return nil
}

// CompleteImageUpload asks the platform to push the staged tarball.
func (c *Client) CompleteImageUpload(projectID, appID, uploadKey, tag string) (*ImagePushResult, error) {
	var out ImagePushResult
	body := map[string]string{"upload_key": uploadKey, "tag": tag}
	err := c.Post(fmt.Sprintf("/api/projects/%s/apps/%s/image-uploads/complete", projectID, appID), body, &out)
	return &out, err
}

// progressReader reports upload progress without buffering the stream.
type progressReader struct {
	r     io.Reader
	total int64
	sent  int64
	fn    func(sent, total int64)
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	if n > 0 {
		p.sent += int64(n)
		p.fn(p.sent, p.total)
	}
	return n, err
}
