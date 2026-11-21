package api

import (
	"encoding/json"
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
		ID        string `json:"id"`
		Name      string `json:"name"`
		Mode      string `json:"mode"`
		Kind      string `json:"kind"` // postgres | mssql | mongodb | redis
		Database  string `json:"database"`
		KeyPrefix string `json:"key_prefix"`
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
	Database string     `json:"database,omitempty"` // SQL Server
	Schema   string     `json:"schema"`
	Table    string     `json:"table"`
	Filters  []DBFilter `json:"filters"`
	Sort     []DBSort   `json:"sort"`
	Limit    int        `json:"limit"`
	Offset   int        `json:"offset"`
	Count    bool       `json:"count"`
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

func dbParam(database string) string {
	if database == "" {
		return ""
	}
	return "&database=" + url.QueryEscape(database)
}

func (c *Client) DBTables(machineID, addonID, schema, database string) ([]DBTable, error) {
	var out struct {
		Tables []DBTable `json:"tables"`
	}
	err := c.do("GET", dbPath(machineID, addonID, "tables?schema="+url.QueryEscape(schema)+dbParam(database)), nil, &out)
	return out.Tables, err
}

func (c *Client) DBSchemas(machineID, addonID, database string) ([]string, error) {
	var out struct {
		Schemas []string `json:"schemas"`
	}
	err := c.do("GET", dbPath(machineID, addonID, "schemas?"+dbParam(database)), nil, &out)
	return out.Schemas, err
}

func (c *Client) DBDatabases(machineID, addonID string) ([]string, string, error) {
	var out struct {
		Databases []string `json:"databases"`
		Default   string   `json:"default"`
	}
	err := c.do("GET", dbPath(machineID, addonID, "databases"), nil, &out)
	return out.Databases, out.Default, err
}

func (c *Client) DBDescribe(machineID, addonID, schema, table, database string) (*DBTableInfo, error) {
	var out DBTableInfo
	q := "structure?schema=" + url.QueryEscape(schema) + "&table=" + url.QueryEscape(table) + dbParam(database)
	return &out, c.do("GET", dbPath(machineID, addonID, q), nil, &out)
}

func (c *Client) DBBrowse(machineID, addonID string, req DBBrowseRequest) (*DBResult, error) {
	var out DBResult
	return &out, c.do("POST", dbPath(machineID, addonID, "rows"), req, &out)
}

// DBQuery runs one statement. A write without WHERE comes back as a 409
// until confirmUnbounded is set.
func (c *Client) DBQuery(machineID, addonID, sql string, write, confirmUnbounded bool, database string) (*DBResult, error) {
	var out DBResult
	body := map[string]any{"sql": sql, "write": write, "confirm_unbounded": confirmUnbounded, "database": database}
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

// ---- MongoDB ----------------------------------------------------------------

type MongoCollection struct {
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	EstDocs *int64 `json:"est_docs"`
}

type MongoResult struct {
	Documents   []json.RawMessage `json:"documents"`
	IDs         []json.RawMessage `json:"ids"`
	Cut         []int             `json:"cut,omitempty"`
	Truncated   bool              `json:"truncated"`
	ExactCount  *int64            `json:"exact_count,omitempty"`
	CountCapped bool              `json:"count_capped,omitempty"`
	CountError  string            `json:"count_error,omitempty"`
	EstDocs     *int64            `json:"est_docs,omitempty"`
	DurationMs  int64             `json:"duration_ms"`
	Command     string            `json:"command"`
}

type MongoFindRequest struct {
	Collection string `json:"collection"`
	Filter     string `json:"filter,omitempty"`
	Projection string `json:"projection,omitempty"`
	Sort       string `json:"sort,omitempty"`
	Limit      int    `json:"limit"`
	Skip       int    `json:"skip,omitempty"`
	After      string `json:"after,omitempty"`
	Count      bool   `json:"count,omitempty"`
	Record     bool   `json:"record"`
}

func (c *Client) MongoCollections(machineID, addonID string) ([]MongoCollection, string, error) {
	var out struct {
		Database    string            `json:"database"`
		Collections []MongoCollection `json:"collections"`
	}
	err := c.do("GET", dbPath(machineID, addonID, "mongo/collections"), nil, &out)
	return out.Collections, out.Database, err
}

func (c *Client) MongoFind(machineID, addonID string, req MongoFindRequest) (*MongoResult, error) {
	var out MongoResult
	return &out, c.do("POST", dbPath(machineID, addonID, "mongo/find"), req, &out)
}

func (c *Client) MongoAggregate(machineID, addonID, collection, pipeline string, write bool) (*MongoResult, error) {
	var out MongoResult
	return &out, c.do("POST", dbPath(machineID, addonID, "mongo/aggregate"), map[string]any{"collection": collection, "pipeline": pipeline, "write": write}, &out)
}

// ---- Redis ------------------------------------------------------------------

type RedisKey struct {
	Key   string `json:"key"`
	Type  string `json:"type"`
	TTLms int64  `json:"ttl_ms"`
}

type RedisScanResult struct {
	Keys   []RedisKey `json:"keys"`
	Cursor string     `json:"cursor"`
	Prefix string     `json:"prefix"`
	DBSize *int64     `json:"db_size"`
}

type RedisValue struct {
	Key       string `json:"key"`
	Type      string `json:"type"`
	TTLms     int64  `json:"ttl_ms"`
	Length    int64  `json:"length"`
	Truncated bool   `json:"truncated"`
	Value     any    `json:"value"`
}

type RedisReply struct {
	Command    string `json:"command"`
	Reply      any    `json:"reply"`
	Nil        bool   `json:"nil"`
	DurationMs int64  `json:"duration_ms"`
}

func (c *Client) RedisScan(machineID, addonID, pattern, cursor, typ string, limit int) (*RedisScanResult, error) {
	var out RedisScanResult
	body := map[string]any{"pattern": pattern, "cursor": cursor, "type": typ, "limit": limit}
	return &out, c.do("POST", dbPath(machineID, addonID, "redis/scan"), body, &out)
}

func (c *Client) RedisGet(machineID, addonID, key string) (*RedisValue, error) {
	var out RedisValue
	return &out, c.do("GET", dbPath(machineID, addonID, "redis/key?key="+url.QueryEscape(key)), nil, &out)
}

func (c *Client) RedisCommand(machineID, addonID, command string, confirm bool) (*RedisReply, error) {
	var out RedisReply
	return &out, c.do("POST", dbPath(machineID, addonID, "redis/command"), map[string]any{"command": command, "confirm": confirm}, &out)
}
