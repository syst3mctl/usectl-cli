package api

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// ========== Devo (POST /api/copilot/chat, SSE) ==========

// CopilotEvent is one SSE frame from Devo.
type CopilotEvent struct {
	Content  string          `json:"content,omitempty"`
	Tool     string          `json:"tool,omitempty"`
	Proposal *CopilotAction  `json:"proposal,omitempty"`
	Error    string          `json:"error,omitempty"`
	Done     bool            `json:"done,omitempty"`
	Raw      json.RawMessage `json:"-"`
}

// CopilotAction is a change Devo proposed (mig 086).
type CopilotAction struct {
	ID      string         `json:"id"`
	Tool    string         `json:"tool"`
	Args    map[string]any `json:"args"`
	Summary string         `json:"summary"`
	Why     string         `json:"why,omitempty"`
	Risk    string         `json:"risk"`
	Status  string         `json:"status"`
	Result  *string        `json:"result,omitempty"`
}

type CopilotMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Ask streams one Devo turn, calling fn for every event until done.
func (c *Client) Ask(projectID, message string, history []CopilotMessage, conversationID string, fn func(CopilotEvent)) error {
	body, _ := json.Marshal(map[string]any{"message": message, "project_id": projectID, "history": history, "conversation_id": conversationID})
	req, err := http.NewRequest(http.MethodPost, c.BaseURL+"/api/copilot/chat", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("User-Agent", "usectl-cli")
	hc := &http.Client{Timeout: 5 * time.Minute}
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("devo: HTTP %d", resp.StatusCode)
	}
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		var ev CopilotEvent
		if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &ev); err != nil {
			continue
		}
		fn(ev)
		if ev.Done {
			return nil
		}
	}
	return sc.Err()
}

func (c *Client) ApproveCopilotAction(projectID, actionID string) (*CopilotAction, error) {
	var a CopilotAction
	err := c.Post(fmt.Sprintf("/api/projects/%s/copilot/actions/%s/approve", projectID, actionID), nil, &a)
	return &a, err
}

func (c *Client) RejectCopilotAction(projectID, actionID string) (*CopilotAction, error) {
	var a CopilotAction
	err := c.Post(fmt.Sprintf("/api/projects/%s/copilot/actions/%s/reject", projectID, actionID), nil, &a)
	return &a, err
}
