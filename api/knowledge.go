package api

import (
	"fmt"
	"net/url"
	"time"
)

// ========== Knowledge base (admin) ==========

// KnowledgeDoc is one file pushed by `usectl admin knowledge sync`.
type KnowledgeDoc struct {
	Path    string `json:"path"`
	Title   string `json:"title,omitempty"`
	Content string `json:"content"`
}

type KnowledgeSyncStatus struct {
	Corpus     string     `json:"corpus"`
	Status     string     `json:"status"`
	Docs       int        `json:"docs"`
	Chunks     int        `json:"chunks"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	Error      *string    `json:"error,omitempty"`
}

type KnowledgeChunk struct {
	Path    string  `json:"path"`
	Title   string  `json:"title"`
	Heading string  `json:"heading"`
	Content string  `json:"content"`
	Score   float64 `json:"score"`
}

// KnowledgeSync pushes a whole corpus; the API diffs by hash and embeds in
// the background.
func (c *Client) KnowledgeSync(corpus string, docs []KnowledgeDoc) error {
	return c.Post("/api/admin/knowledge/sync", map[string]interface{}{"corpus": corpus, "docs": docs}, nil)
}

func (c *Client) KnowledgeStatus() ([]KnowledgeSyncStatus, error) {
	var out struct {
		Corpora []KnowledgeSyncStatus `json:"corpora"`
	}
	err := c.Get("/api/admin/knowledge/status", &out)
	return out.Corpora, err
}

func (c *Client) KnowledgeSearch(corpus, query string, k int) ([]KnowledgeChunk, error) {
	q := url.Values{"corpus": {corpus}, "q": {query}}
	if k > 0 {
		q.Set("k", fmt.Sprint(k))
	}
	var out struct {
		Results []KnowledgeChunk `json:"results"`
	}
	err := c.Get("/api/admin/knowledge/search?"+q.Encode(), &out)
	return out.Results, err
}
