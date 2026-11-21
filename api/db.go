package api

import (
	"fmt"
	"net/url"
)

// Database panel endpoints (/api/projects/{id}/addons/{addonId}/db/*). The
// server runs every statement with the addon's own credentials and applies
// the same roles as the dashboard: viewers read, developers and owners may
// write. Read-only agent keys are read-only regardless of role.

type DBAccess struct {
	Access struct {
		Role           string `json:"role"`
		CanWrite       bool   `json:"can_write"`
		ReadOnlyReason string `json:"read_only_reason"`
	} `json:"access"`
	Addon struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Mode string `json:"mode"`
	} `json:"addon"`
	Limits struct {
		MaxRows        int `json:"max_rows"`
		TimeoutSeconds int `json:"timeout_seconds"`
	} `json:"limits"`
}

type DBTable struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	EstRows   *int64 `json:"est_rows"`
	SizeBytes int64  `json:"size_bytes"`
	Comment   string `json:"comment"`
}

type DBColumnInfo struct {
	Name      string  `json:"name"`
	Type      string  `json:"type"`
	Nullable  bool    `json:"nullable"`
	Default   *string `json:"default"`
	Identity  bool    `json:"identity"`
	Generated bool    `json:"generated"`
	IsPK      bool    `json:"is_pk"`
	Comment   string  `json:"comment"`
}

type DBTableInfo struct {
	Schema     string         `json:"schema"`
	Name       string         `json:"name"`
	Kind       string         `json:"kind"`
	EstRows    *int64         `json:"est_rows"`
	Columns    []DBColumnInfo `json:"columns"`
	PrimaryKey []string       `json:"primary_key"`
	Indexes    []struct {
		Name    string `json:"name"`
		Def     string `json:"definition"`
		Unique  bool   `json:"unique"`
		Primary bool   `json:"primary"`
	} `json:"indexes"`
	Constraints []struct {
		Name string `json:"name"`
		Type string `json:"type"`
		Def  string `json:"definition"`
	} `json:"constraints"`
	Editable bool `json:"editable"`
}

type DBResult struct {
	Columns []struct {
		Name string `json:"name"`
		Type string `json:"type"`
	} `json:"columns"`
	Rows          [][]*string `json:"rows"`
	RowCount      int64       `json:"row_count"`
	Command       string      `json:"command"`
	Truncated     bool        `json:"truncated"`
	DurationMs    int64       `json:"duration_ms"`
	ExactCount    *int64      `json:"exact_count,omitempty"`
	CountCapped   bool        `json:"count_capped,omitempty"`
	CountError    string      `json:"count_error,omitempty"`
	EstimatedRows *int64      `json:"estimated_rows,omitempty"`
	CutCells      [][2]int    `json:"cut_cells,omitempty"`
}

type DBFilter struct {
	Column string  `json:"column"`
	Op     string  `json:"op"`
	Value  *string `json:"value"`
}

type DBSort struct {
	Column string `json:"column"`
	Desc   bool   `json:"desc"`
}

type DBBrowseRequest struct {
	Schema  string     `json:"schema"`
	Table   string     `json:"table"`
	Filters []DBFilter `json:"filters"`
	Sort    []DBSort   `json:"sort"`
	Limit   int        `json:"limit"`
	Offset  int        `json:"offset"`
	Count   bool       `json:"count"`
	// After pages by primary key: rows after this key (default order only).
	After map[string]*string `json:"after,omitempty"`
}

type DBHistoryEntry struct {
	UserName   string  `json:"user_name"`
	UserEmail  string  `json:"user_email"`
	Source     string  `json:"source"`
	Mode       string  `json:"mode"`
	SQL        string  `json:"sql"`
	Status     string  `json:"status"`
	Error      *string `json:"error"`
	RowCount   *int64  `json:"row_count"`
	DurationMs int     `json:"duration_ms"`
	CreatedAt  string  `json:"created_at"`
}

func dbPath(machineID, addonID, rest string) string {
	return fmt.Sprintf("/api/projects/%s/addons/%s/db/%s", machineID, addonID, rest)
}

func (c *Client) DBAccess(machineID, addonID string) (*DBAccess, error) {
	var out DBAccess
	return &out, c.do("GET", dbPath(machineID, addonID, "access"), nil, &out)
}

func (c *Client) DBTables(machineID, addonID, schema string) ([]DBTable, error) {
	var out struct {
		Tables []DBTable `json:"tables"`
	}
	err := c.do("GET", dbPath(machineID, addonID, "tables?schema="+url.QueryEscape(schema)), nil, &out)
	return out.Tables, err
}

func (c *Client) DBSchemas(machineID, addonID string) ([]string, error) {
	var out struct {
		Schemas []string `json:"schemas"`
	}
	err := c.do("GET", dbPath(machineID, addonID, "schemas"), nil, &out)
	return out.Schemas, err
}

func (c *Client) DBDescribe(machineID, addonID, schema, table string) (*DBTableInfo, error) {
	var out DBTableInfo
	q := "structure?schema=" + url.QueryEscape(schema) + "&table=" + url.QueryEscape(table)
	return &out, c.do("GET", dbPath(machineID, addonID, q), nil, &out)
}

func (c *Client) DBBrowse(machineID, addonID string, req DBBrowseRequest) (*DBResult, error) {
	var out DBResult
	return &out, c.do("POST", dbPath(machineID, addonID, "rows"), req, &out)
}

// DBQuery runs one statement. A write without WHERE comes back as a 409
// until confirmUnbounded is set.
func (c *Client) DBQuery(machineID, addonID, sql string, write, confirmUnbounded bool) (*DBResult, error) {
	var out DBResult
	body := map[string]any{"sql": sql, "write": write, "confirm_unbounded": confirmUnbounded}
	// The client only retries after a 401, i.e. when the server refused the
	// request before running it — so a write is never applied twice.
	return &out, c.do("POST", dbPath(machineID, addonID, "query"), body, &out)
}

func (c *Client) DBHistory(machineID, addonID string, mine bool, limit int) ([]DBHistoryEntry, error) {
	q := url.Values{}
	if mine {
		q.Set("mine", "1")
	}
	if limit > 0 {
		q.Set("limit", fmt.Sprint(limit))
	}
	var out struct {
		History []DBHistoryEntry `json:"history"`
	}
	err := c.do("GET", dbPath(machineID, addonID, "history?"+q.Encode()), nil, &out)
	return out.History, err
}
