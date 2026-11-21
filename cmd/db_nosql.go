package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/giorgi/usectl/api"
	"github.com/giorgi/usectl/output"
	"github.com/spf13/cobra"
)

// MongoDB and Redis commands of `usectl db`. Same endpoints and roles as the
// dashboard's Explore tab.

var (
	mongoFilter     string
	mongoSort       string
	mongoProjection string
	mongoAfter      string
	mongoPretty     bool
	redisType       string
	redisCursor     string
	mongoLimit      int
	redisLimit      int
)

func printMongoCollections(client *api.Client, mID, aID string) error {
	cols, db, err := client.MongoCollections(mID, aID)
	if err != nil {
		return err
	}
	if jsonOutput {
		return output.JSON(cols)
	}
	rows := [][]string{}
	for _, c := range cols {
		est := "—"
		if c.EstDocs != nil {
			est = fmt.Sprintf("~%d", *c.EstDocs)
		}
		rows = append(rows, []string{c.Name, c.Kind, est})
	}
	fmt.Println(output.Dim("database " + db))
	output.Table([]string{"COLLECTION", "KIND", "DOCUMENTS"}, rows)
	return nil
}

func printMongoResult(res *api.MongoResult) {
	for _, d := range res.Documents {
		if mongoPretty {
			var v any
			if json.Unmarshal(d, &v) == nil {
				b, _ := json.MarshalIndent(v, "", "  ")
				fmt.Println(string(b))
				continue
			}
		}
		fmt.Println(string(d))
	}
	note := fmt.Sprintf("%d document(s) · %d ms", len(res.Documents), res.DurationMs)
	if res.Truncated {
		note += " · truncated at the limit"
	}
	if len(res.Cut) > 0 {
		note += fmt.Sprintf(" · %d over 64 KB shown as placeholders", len(res.Cut))
	}
	fmt.Println(output.Dim(note))
}

var dbFindCmd = &cobra.Command{
	Use:   "find [machine] <addon> <collection>",
	Short: "MongoDB: list documents with a filter, sort and projection",
	Example: `  usectl db find shop mongodb orders --filter '{"status":"paid"}' --sort '{"createdAt":-1}' --limit 20
  usectl db find shop mongodb events --limit 100                       # prints the next --after
  usectl db find shop mongodb events --after '{"$oid":"6b02…"}' --count`,
	Args: cobra.RangeArgs(2, 3),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, mID, aID, rest, err := dbClient(args)
		if err != nil {
			return err
		}
		if len(rest) != 1 {
			return errors.New("name the collection")
		}
		res, err := client.MongoFind(mID, aID, api.MongoFindRequest{
			Collection: rest[0], Filter: mongoFilter, Sort: mongoSort, Projection: mongoProjection,
			Limit: mongoLimit, Skip: dbOffset, After: mongoAfter, Count: dbCount, Record: mongoFilter != "" || mongoSort != "",
		})
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(res)
		}
		printMongoResult(res)
		switch {
		case res.ExactCount != nil && res.CountCapped:
			fmt.Println(output.Dim(fmt.Sprintf("more than %d matching documents", *res.ExactCount)))
		case res.ExactCount != nil:
			fmt.Println(output.Dim(fmt.Sprintf("%d matching documents", *res.ExactCount)))
		case res.CountError != "":
			fmt.Println(output.Yellow(res.CountError))
		case res.EstDocs != nil:
			fmt.Println(output.Dim(fmt.Sprintf("~%d documents in the collection", *res.EstDocs)))
		}
		if len(res.Documents) == mongoLimit && mongoSort == "" && len(res.IDs) > 0 {
			fmt.Println(output.Dim("next page: add --after '" + string(res.IDs[len(res.IDs)-1]) + "'"))
		}
		return nil
	},
}

var dbAggregateCmd = &cobra.Command{
	Use:   "aggregate [machine] <addon> <collection> '<pipeline JSON>'",
	Short: "MongoDB: run an aggregation pipeline ($out/$merge need --write)",
	Example: `  usectl db aggregate shop mongodb orders '[{"$group":{"_id":"$status","n":{"$sum":1}}}]'
  usectl db aggregate shop mongodb orders --file pipeline.json`,
	Args: cobra.RangeArgs(2, 4),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		mID, aID, rest, err := resolveMachineAndAddon(client, args)
		if err != nil {
			return err
		}
		if len(rest) == 0 {
			return errors.New("name the collection")
		}
		pipeline, err := querySQL(rest[1:])
		if err != nil {
			return err
		}
		res, err := client.MongoAggregate(mID, aID, rest[0], pipeline, dbWrite)
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(res)
		}
		printMongoResult(res)
		return nil
	},
}

func fmtTTL(ms int64) string {
	switch {
	case ms < 0:
		return "—"
	case ms < 60_000:
		return fmt.Sprintf("%ds", ms/1000)
	case ms < 3_600_000:
		return fmt.Sprintf("%dm", ms/60_000)
	case ms < 86_400_000:
		return fmt.Sprintf("%.1fh", float64(ms)/3_600_000)
	}
	return fmt.Sprintf("%.1fd", float64(ms)/86_400_000)
}

var dbKeysCmd = &cobra.Command{
	Use:   "keys [machine] <addon> [pattern]",
	Short: "Redis: list keys matching a pattern (SCAN — safe on any size)",
	Example: `  usectl db keys shop redis 'session:*' --type hash
  usectl db keys shop redis 'user:*' --cursor 29504          # continue a scan`,
	Args: cobra.RangeArgs(1, 3),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, mID, aID, rest, err := dbClient(args)
		if err != nil {
			return err
		}
		pattern := "*"
		if len(rest) > 0 {
			pattern = rest[0]
		}
		res, err := client.RedisScan(mID, aID, pattern, redisCursor, redisType, redisLimit)
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(res)
		}
		rows := [][]string{}
		for _, k := range res.Keys {
			rows = append(rows, []string{k.Key, k.Type, fmtTTL(k.TTLms)})
		}
		output.Table([]string{"KEY", "TYPE", "TTL"}, rows)
		if res.Prefix != "" {
			fmt.Println(output.Dim("shared Redis: only keys under " + res.Prefix + "* are visible"))
		}
		if res.Cursor != "0" {
			fmt.Println(output.Dim("more keys: add --cursor " + res.Cursor))
		} else {
			fmt.Println(output.Dim("scan complete"))
		}
		return nil
	},
}

var dbGetCmd = &cobra.Command{
	Use:   "get [machine] <addon> <key>",
	Short: "Redis: show a key's value (any type)",
	Args:  cobra.RangeArgs(2, 3),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, mID, aID, rest, err := dbClient(args)
		if err != nil {
			return err
		}
		if len(rest) != 1 {
			return errors.New("name the key")
		}
		v, err := client.RedisGet(mID, aID, rest[0])
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(v)
		}
		fmt.Printf("%s  %s  %s\n", output.Bold(v.Key), output.Cyan(v.Type), output.Dim("TTL "+fmtTTL(v.TTLms)))
		switch val := v.Value.(type) {
		case string:
			fmt.Println(val)
		case []any:
			for _, item := range val {
				if pair, ok := item.([]any); ok && len(pair) == 2 {
					fmt.Printf("  %v = %v\n", pair[0], pair[1])
				} else if m, ok := item.(map[string]any); ok {
					b, _ := json.Marshal(m)
					fmt.Println("  " + string(b))
				} else {
					fmt.Printf("  %v\n", item)
				}
			}
		default:
			b, _ := json.MarshalIndent(val, "", "  ")
			fmt.Println(string(b))
		}
		if v.Truncated {
			fmt.Println(output.Dim(fmt.Sprintf("showing part of %d — use 'usectl db redis' for ranges", v.Length)))
		}
		return nil
	},
}

var dbRedisCmd = &cobra.Command{
	Use:   "redis [machine] <addon> \"<command>\"",
	Short: "Redis: run one command (read commands for viewers)",
	Long: `Runs one Redis command. Viewers may run read commands only. Commands that
block the connection or administer the server (KEYS, MONITOR, SUBSCRIBE,
CONFIG, EVAL, MULTI, …) are not available; FLUSHDB/FLUSHALL need --yes.`,
	Example: `  usectl db redis shop redis "HGETALL user:1"
  usectl db redis shop redis "SET greeting hello EX 60"`,
	Args: cobra.RangeArgs(2, 3),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
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
		if len(rest) != 1 {
			return errors.New("quote the command as one argument, e.g. \"GET key\"")
		}
		rep, err := client.RedisCommand(mID, aID, rest[0], dbYes)
		var apiErr *api.APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == 409 && !dbYes {
			return fmt.Errorf("%s\nrerun with --yes to confirm", apiErr.Message)
		}
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(rep)
		}
		switch {
		case rep.Nil:
			fmt.Println("(nil)")
		default:
			switch r := rep.Reply.(type) {
			case string:
				fmt.Println(r)
			case map[string]any:
				keys := make([]string, 0, len(r))
				for k := range r {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				for _, k := range keys {
					fmt.Printf("%s = %v\n", k, r[k])
				}
			default:
				b, _ := json.MarshalIndent(r, "", "  ")
				fmt.Println(strings.TrimSpace(string(b)))
			}
		}
		return nil
	},
}

func init() {
	dbFindCmd.Flags().StringVar(&mongoFilter, "filter", "", `Filter as JSON, e.g. {"status":"paid"}`)
	dbFindCmd.Flags().StringVar(&mongoSort, "sort", "", `Sort as JSON, e.g. {"createdAt":-1} (default: by _id, keyset-paged)`)
	dbFindCmd.Flags().StringVar(&mongoProjection, "projection", "", "Projection as JSON")
	dbFindCmd.Flags().StringVar(&mongoAfter, "after", "", "Page after this _id (Extended JSON, printed by the previous page)")
	dbFindCmd.Flags().IntVar(&mongoLimit, "limit", 50, "Documents per page (max 500)")
	dbFindCmd.Flags().IntVar(&dbOffset, "skip", 0, "Documents to skip with --sort (max 100000)")
	dbFindCmd.Flags().BoolVar(&dbCount, "count", false, "Also count matching documents (stops at 1,000,000)")
	dbFindCmd.Flags().BoolVar(&mongoPretty, "pretty", false, "Indent each document")
	dbAggregateCmd.Flags().BoolVar(&dbWrite, "write", false, "Allow $out and $merge (developer or owner)")
	dbAggregateCmd.Flags().StringVarP(&dbFile, "file", "f", "", "Read the pipeline from a file")
	dbAggregateCmd.Flags().BoolVar(&mongoPretty, "pretty", false, "Indent each document")
	dbKeysCmd.Flags().StringVar(&redisType, "type", "", "Only keys of this type (string, hash, list, set, zset, stream)")
	dbKeysCmd.Flags().StringVar(&redisCursor, "cursor", "0", "Continue a previous scan")
	dbKeysCmd.Flags().IntVar(&redisLimit, "limit", 100, "Keys per call (max 500)")
	dbRedisCmd.Flags().BoolVar(&dbYes, "yes", false, "Confirm FLUSHDB / FLUSHALL")
}
