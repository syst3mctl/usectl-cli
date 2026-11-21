package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/giorgi/usectl/api"
	"github.com/giorgi/usectl/output"
	"github.com/spf13/cobra"
)

// `usectl db` — the CLI side of the dashboard's database panel. Same server
// endpoints, same rules: viewers read, developers and owners may write
// (--write), read-only agent keys only read.

var (
	dbSchema   string
	dbWhere    []string
	dbSortFlag []string
	dbLimit    int
	dbOffset   int
	dbCount    bool
	dbWrite    bool
	dbYes      bool
	dbFile     string
	dbMine     bool
	dbHistLim  int
	dbAfter    []string
	dbFull     bool
	dbDatabase string
)

var dbCmd = &cobra.Command{
	Use:   "db",
	Short: "Browse and query a machine's database addons (PostgreSQL, SQL Server, MongoDB, Redis)",
	Long: `Browse and query database addons without their passwords ever leaving the
server.

  PostgreSQL, SQL Server   tables, describe, rows, query   (--database picks a SQL Server database)
  MongoDB                  tables (collections), find, aggregate
  Redis                    keys, get, redis "<command>"

Access follows your machine role, exactly as in the dashboard:
  viewer             read-only
  developer, owner   may also change data (--write; redis write commands)

Every statement is recorded in the machine's shared query history.

The machine may be omitted when one is set with 'usectl use' or -m.`,
}

func dbClient(args []string) (*api.Client, string, string, []string, error) {
	client, err := api.NewClient(apiURL)
	if err != nil {
		return nil, "", "", nil, err
	}
	machineID, addonID, rest, err := resolveMachineAndAddon(client, args)
	return client, machineID, addonID, rest, err
}

// splitTable accepts "table" or "schema.table".
func splitTable(ref, defaultSchema string) (string, string) {
	if s, t, ok := strings.Cut(ref, "."); ok && s != "" && t != "" {
		return s, t
	}
	return defaultSchema, ref
}

var dbAccessCmd = &cobra.Command{
	Use:   "access [machine] <addon>",
	Short: "Show what you may do on this database",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, mID, aID, _, err := dbClient(args)
		if err != nil {
			return err
		}
		a, err := client.DBAccess(mID, aID)
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(a)
		}
		mode := output.Green("read & write")
		if !a.Access.CanWrite {
			mode = output.Yellow("read-only")
		}
		fmt.Printf("%s  %s · %s\n", output.Bold(a.Addon.Name), a.Access.Role, mode)
		if a.Access.ReadOnlyReason != "" {
			fmt.Println(output.Dim(a.Access.ReadOnlyReason))
		}
		fmt.Println(output.Dim(fmt.Sprintf("limits: %d rows per result, %ds per statement", a.Limits.MaxRows, a.Limits.TimeoutSeconds)))
		return nil
	},
}

var dbTablesCmd = &cobra.Command{
	Use:     "tables [machine] <addon>",
	Short:   "List tables and views",
	Example: "  usectl db tables fins postgres\n  usectl db tables fins postgres --schema billing",
	Args:    cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, mID, aID, _, err := dbClient(args)
		if err != nil {
			return err
		}
		if acc, aerr := client.DBAccess(mID, aID); aerr == nil {
			switch acc.Addon.Kind {
			case "mongodb":
				return printMongoCollections(client, mID, aID)
			case "redis":
				return fmt.Errorf("Redis has keys, not tables — use 'usectl db keys'")
			case "mssql":
				if dbSchema == "public" {
					dbSchema = "dbo"
				}
			}
		}
		tables, err := client.DBTables(mID, aID, dbSchema, dbDatabase)
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(tables)
		}
		rows := [][]string{}
		for _, t := range tables {
			est := "—"
			if t.EstRows != nil {
				est = fmt.Sprintf("~%d", *t.EstRows)
			}
			rows = append(rows, []string{t.Name, strings.ReplaceAll(t.Kind, "_", " "), est, humanBytes(t.SizeBytes)})
		}
		if len(rows) == 0 {
			if schemas, sErr := client.DBSchemas(mID, aID, dbDatabase); sErr == nil {
				fmt.Printf("No tables in schema %q. Schemas: %s\n", dbSchema, strings.Join(schemas, ", "))
				return nil
			}
		}
		output.Table([]string{"TABLE", "KIND", "ROWS", "SIZE"}, rows)
		return nil
	},
}

var dbDescribeCmd = &cobra.Command{
	Use:     "describe [machine] <addon> <table>",
	Aliases: []string{"desc"},
	Short:   "Show a table's columns, indexes and constraints",
	Example: "  usectl db describe fins postgres users\n  usectl db describe fins postgres billing.invoices",
	Args:    cobra.RangeArgs(2, 3),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, mID, aID, rest, err := dbClient(args)
		if err != nil {
			return err
		}
		if len(rest) != 1 {
			return errors.New("name the table, e.g. 'usectl db describe <machine> <addon> users'")
		}
		schema, table := splitTable(rest[0], sqlSchema(client, mID, aID))
		info, err := client.DBDescribe(mID, aID, schema, table, dbDatabase)
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(info)
		}
		fmt.Printf("%s.%s  %s\n\n", output.Bold(info.Schema), output.Bold(info.Name), output.Dim(info.Kind))
		rows := [][]string{}
		for _, c := range info.Columns {
			name := c.Name
			if c.IsPK {
				name += " " + output.Yellow("PK")
			}
			null := "NOT NULL"
			if c.Nullable {
				null = ""
			}
			def := ""
			if c.Default != nil {
				def = *c.Default
			}
			if c.Identity {
				def = "identity"
			}
			rows = append(rows, []string{name, c.Type, null, def})
		}
		output.Table([]string{"COLUMN", "TYPE", "NULL", "DEFAULT"}, rows)
		if len(info.Indexes) > 0 {
			fmt.Println("\n" + output.Bold("Indexes"))
			for _, ix := range info.Indexes {
				fmt.Printf("  %s\n", ix.Def)
			}
		}
		if len(info.Constraints) > 0 {
			fmt.Println("\n" + output.Bold("Constraints"))
			for _, c := range info.Constraints {
				fmt.Printf("  %s  %s\n", c.Name, output.Dim(c.Def))
			}
		}
		return nil
	},
}

var (
	whereCmpRe  = regexp.MustCompile(`^\s*([^\s=<>!~^]+)\s*(>=|<=|!=|<>|=|>|<|!~|~|\^=)\s*(.*)$`)
	whereNullRe = regexp.MustCompile(`(?i)^\s*(\S+)\s+is\s+(not\s+)?null\s*$`)
	whereOps    = map[string]string{"=": "eq", "!=": "neq", "<>": "neq", ">": "gt", ">=": "gte", "<": "lt", "<=": "lte", "~": "contains", "!~": "not_contains", "^=": "starts_with"}
)

func parseWhere(s string) (api.DBFilter, error) {
	if m := whereNullRe.FindStringSubmatch(s); m != nil {
		op := "is_null"
		if m[2] != "" {
			op = "not_null"
		}
		return api.DBFilter{Column: m[1], Op: op}, nil
	}
	m := whereCmpRe.FindStringSubmatch(s)
	if m == nil {
		return api.DBFilter{}, fmt.Errorf("can't read --where %q — use 'column OP value' with = != > >= < <= ~ (contains) !~ ^= (starts with), or 'column is [not] null'", s)
	}
	v := strings.Trim(m[3], `"'`)
	return api.DBFilter{Column: m[1], Op: whereOps[m[2]], Value: &v}, nil
}

var dbRowsCmd = &cobra.Command{
	Use:   "rows [machine] <addon> <table>",
	Short: "Show rows of a table, with filters and sorting",
	Example: `  usectl db rows fins postgres users --limit 20
  usectl db rows fins postgres users --where 'status = paid' --where 'total >= 100' --sort -created_at
  usectl db rows fins postgres users --where 'email ~ @gmail' --count
  usectl db rows fins postgres users --where 'deleted_at is null' --json`,
	Args: cobra.RangeArgs(2, 3),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, mID, aID, rest, err := dbClient(args)
		if err != nil {
			return err
		}
		if len(rest) != 1 {
			return errors.New("name the table, e.g. 'usectl db rows <machine> <addon> users'")
		}
		schema, table := splitTable(rest[0], sqlSchema(client, mID, aID))
		req := api.DBBrowseRequest{Database: dbDatabase, Schema: schema, Table: table, Limit: dbLimit, Offset: dbOffset, Count: dbCount,
			Filters: []api.DBFilter{}, Sort: []api.DBSort{}}
		for _, w := range dbWhere {
			f, err := parseWhere(w)
			if err != nil {
				return err
			}
			req.Filters = append(req.Filters, f)
		}
		if len(dbAfter) > 0 {
			req.After = map[string]*string{}
			for _, kv := range dbAfter {
				k, v, ok := strings.Cut(kv, "=")
				if !ok || k == "" {
					return fmt.Errorf("--after takes column=value, e.g. --after id=1042")
				}
				val := v
				req.After[strings.TrimSpace(k)] = &val
			}
		}
		for _, s := range dbSortFlag {
			desc := strings.HasPrefix(s, "-")
			req.Sort = append(req.Sort, api.DBSort{Column: strings.TrimPrefix(strings.TrimPrefix(s, "-"), "+"), Desc: desc})
		}
		res, err := client.DBBrowse(mID, aID, req)
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(res)
		}
		printDBResult(res)
		switch {
		case res.ExactCount != nil && res.CountCapped:
			fmt.Println(output.Dim(fmt.Sprintf("more than %d matching rows", *res.ExactCount)))
		case res.ExactCount != nil:
			fmt.Println(output.Dim(fmt.Sprintf("%d matching rows in total", *res.ExactCount)))
		case res.CountError != "":
			fmt.Println(output.Yellow(res.CountError))
		case res.EstimatedRows != nil:
			fmt.Println(output.Dim(fmt.Sprintf("~%d matching rows (planner estimate)", *res.EstimatedRows)))
		}
		if len(res.Rows) == dbLimit {
			// Primary-key order (no --sort): print the cursor for the next
			// page — fast at any depth, unlike --offset.
			if len(dbSortFlag) == 0 {
				if info, err := client.DBDescribe(mID, aID, schema, table, dbDatabase); err == nil && len(info.PrimaryKey) > 0 {
					last := res.Rows[len(res.Rows)-1]
					var parts []string
					for _, pk := range info.PrimaryKey {
						for i, c := range res.Columns {
							if c.Name == pk && last[i] != nil {
								parts = append(parts, "--after "+pk+"="+*last[i])
							}
						}
					}
					if len(parts) == len(info.PrimaryKey) {
						fmt.Println(output.Dim("next page: add " + strings.Join(parts, " ")))
						return nil
					}
				}
			}
			fmt.Println(output.Dim(fmt.Sprintf("more rows may follow — use --offset %d (max 100000)", dbOffset+dbLimit)))
		}
		return nil
	},
}

var dbQueryCmd = &cobra.Command{
	Use:   "query [machine] <addon> [SQL]",
	Short: "Run one SQL statement",
	Long: `Runs one SQL statement. Without --write it runs in a read-only transaction
that is always rolled back. --write (developer or owner) commits it; an
UPDATE or DELETE without WHERE also needs --yes.

The SQL comes from the argument, --file, or standard input.`,
	Example: `  usectl db query fins postgres "SELECT count(*) FROM users"
  usectl db query fins postgres --file report.sql --json
  echo "SELECT now()" | usectl db query fins postgres
  usectl db query fins postgres --write "UPDATE users SET plan = 'pro' WHERE id = 42"`,
	Args: cobra.RangeArgs(1, 3),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		// The SQL is a free-form last argument, so resolve the addon from the
		// leading arguments only: try "<machine> <addon> SQL", then "<addon> SQL".
		var mID, aID string
		var rest []string
		if len(args) == 3 {
			mID, aID, _, err = resolveMachineAndAddon(client, args[:2])
			rest = args[2:]
		} else {
			mID, aID, rest, err = resolveMachineAndAddon(client, args)
		}
		if err != nil {
			return err
		}
		sql, err := querySQL(rest)
		if err != nil {
			return err
		}
		res, err := client.DBQuery(mID, aID, sql, dbWrite, dbYes, dbDatabase)
		var apiErr *api.APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == 409 && dbWrite && !dbYes {
			return fmt.Errorf("%s\nrerun with --yes to confirm", apiErr.Message)
		}
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(res)
		}
		printDBResult(res)
		return nil
	},
}

func querySQL(rest []string) (string, error) {
	switch {
	case dbFile != "":
		b, err := os.ReadFile(dbFile)
		if err != nil {
			return "", err
		}
		return string(b), nil
	case len(rest) == 1 && rest[0] != "-":
		return rest[0], nil
	case len(rest) > 1:
		return "", errors.New("quote the SQL as one argument, e.g. \"SELECT * FROM users\"")
	}
	if fi, err := os.Stdin.Stat(); err == nil && fi.Mode()&os.ModeCharDevice != 0 && (len(rest) == 0) {
		return "", errors.New("give the SQL as an argument, with --file, or on standard input")
	}
	b, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(string(b)) == "" {
		return "", errors.New("no SQL given")
	}
	return string(b), nil
}

var dbHistoryCmd = &cobra.Command{
	Use:   "history [machine] <addon>",
	Short: "Show the machine's shared query history for this database",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, mID, aID, _, err := dbClient(args)
		if err != nil {
			return err
		}
		entries, err := client.DBHistory(mID, aID, dbMine, dbHistLim)
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(entries)
		}
		for _, e := range entries {
			who := e.UserName
			if who == "" {
				who = e.UserEmail
			}
			mode := output.Green(e.Mode)
			if e.Mode == "write" {
				mode = output.Yellow(e.Mode)
			}
			status := fmt.Sprintf("%d ms", e.DurationMs)
			if e.Status == "error" {
				status = output.Red("failed")
			}
			ts := e.CreatedAt
			if len(ts) >= 16 {
				ts = strings.Replace(ts[:16], "T", " ", 1)
			}
			fmt.Printf("%s  %s  %s  %s  %s\n", output.Dim(ts), mode, output.Dim(e.Source), who, status)
			fmt.Printf("  %s\n", strings.ReplaceAll(strings.TrimSpace(e.SQL), "\n", "\n  "))
			if e.Error != nil {
				fmt.Printf("  %s\n", output.Red(*e.Error))
			}
		}
		if len(entries) == 0 {
			fmt.Println("No queries yet.")
		}
		return nil
	},
}

// printDBResult prints rows as a table, NULL dimmed, long values cut unless --full.
func printDBResult(res *api.DBResult) {
	if len(res.Columns) == 0 {
		fmt.Printf("%s (%d ms)\n", output.Green(res.Command), res.DurationMs)
		return
	}
	headers := make([]string, len(res.Columns))
	for i, c := range res.Columns {
		headers[i] = strings.ToUpper(c.Name)
	}
	rows := make([][]string, 0, len(res.Rows))
	for _, r := range res.Rows {
		row := make([]string, len(r))
		for i, v := range r {
			switch {
			case v == nil:
				row[i] = output.Dim("NULL")
			default:
				s := strings.ReplaceAll(*v, "\n", "⏎")
				if !dbFull && len([]rune(s)) > 60 {
					s = string([]rune(s)[:57]) + "..."
				}
				row[i] = s
			}
		}
		rows = append(rows, row)
	}
	output.Table(headers, rows)
	note := fmt.Sprintf("%d row(s) · %d ms", len(res.Rows), res.DurationMs)
	if res.Truncated {
		note += " · truncated — add LIMIT/WHERE or use --json for the first rows only"
	}
	fmt.Println(output.Dim(note))
}

func init() {
	for _, c := range []*cobra.Command{dbTablesCmd, dbDescribeCmd, dbRowsCmd} {
		c.Flags().StringVar(&dbSchema, "schema", "public", "Schema to use (SQL Server defaults to dbo)")
	}
	for _, c := range []*cobra.Command{dbTablesCmd, dbDescribeCmd, dbRowsCmd, dbQueryCmd} {
		c.Flags().StringVar(&dbDatabase, "database", "", "SQL Server: which database (default: the addon's)")
	}
	dbRowsCmd.Flags().StringArrayVar(&dbWhere, "where", nil, "Filter, e.g. 'status = paid', 'email ~ gmail', 'deleted_at is null' (repeatable)")
	dbRowsCmd.Flags().StringArrayVar(&dbSortFlag, "sort", nil, "Sort column; prefix with - for descending (repeatable)")
	dbRowsCmd.Flags().IntVar(&dbLimit, "limit", 50, "Rows per page (max 500)")
	dbRowsCmd.Flags().IntVar(&dbOffset, "offset", 0, "Rows to skip (max 100000; use --after on big tables)")
	dbRowsCmd.Flags().StringArrayVar(&dbAfter, "after", nil, "Page by primary key: rows after column=value (repeat for composite keys)")
	dbRowsCmd.Flags().BoolVar(&dbCount, "count", false, "Also count matching rows (stops at 1,000,000)")
	dbRowsCmd.Flags().BoolVar(&dbFull, "full", false, "Don't shorten long values")
	dbQueryCmd.Flags().BoolVar(&dbWrite, "write", false, "Commit the statement (developer or owner)")
	dbQueryCmd.Flags().BoolVar(&dbYes, "yes", false, "Confirm an UPDATE/DELETE without WHERE")
	dbQueryCmd.Flags().StringVarP(&dbFile, "file", "f", "", "Read the SQL from a file")
	dbQueryCmd.Flags().BoolVar(&dbFull, "full", false, "Don't shorten long values")
	dbHistoryCmd.Flags().BoolVar(&dbMine, "mine", false, "Only your own statements")
	dbHistoryCmd.Flags().IntVar(&dbHistLim, "limit", 30, "How many entries")

	dbCmd.AddCommand(dbAccessCmd, dbTablesCmd, dbDescribeCmd, dbRowsCmd, dbQueryCmd, dbHistoryCmd,
		dbFindCmd, dbAggregateCmd, dbKeysCmd, dbGetCmd, dbRedisCmd)
	rootCmd.AddCommand(dbCmd)
}

// sqlSchema is the default schema when --schema was not given: dbo on SQL
// Server, public on PostgreSQL.
func sqlSchema(client *api.Client, mID, aID string) string {
	if dbSchema != "public" {
		return dbSchema
	}
	if acc, err := client.DBAccess(mID, aID); err == nil && acc.Addon.Kind == "mssql" {
		return "dbo"
	}
	return dbSchema
}
