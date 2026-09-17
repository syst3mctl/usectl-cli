package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/giorgi/usectl/api"
	"github.com/giorgi/usectl/output"
)

// usectl admin knowledge — push markdown into the platform's knowledge base.
//
// The vault (the operator's private notes) is pushed FROM THE LAPTOP on
// purpose: the cluster never holds a key to that repository. The docs
// corpus is the public documentation site's content directory.

var adminKnowledgeCmd = &cobra.Command{
	Use:   "knowledge",
	Short: "Sync and query the platform knowledge base (vault, docs)",
	Long: `Pushes a directory of markdown into the knowledge base. The API skips files
whose content is unchanged, re-embeds the rest with the in-cluster BGE-M3 and
deletes notes that disappeared.

  usectl admin knowledge sync ~/vault/usectl --corpus vault
  usectl admin knowledge sync ../usectl-documentation/content --corpus docs
  usectl admin knowledge status
  usectl admin knowledge search "traefik 502 after master restart" --corpus vault

The vault corpus is searchable only by platform admins (search_vault); the
docs corpus by everyone (search_docs, also used by Devo).`,
}

var adminKnowledgeSyncCmd = &cobra.Command{
	Use:   "sync <directory>",
	Short: "Push a directory of .md files into a corpus",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		corpus, _ := cmd.Flags().GetString("corpus")
		if corpus != "vault" && corpus != "docs" {
			return fmt.Errorf("--corpus must be vault or docs")
		}
		excludes, _ := cmd.Flags().GetStringSlice("exclude")
		root, err := filepath.Abs(args[0])
		if err != nil {
			return err
		}
		var docs []api.KnowledgeDoc
		var skipped []string
		err = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			base := info.Name()
			if info.IsDir() {
				if strings.HasPrefix(base, ".") && p != root {
					return filepath.SkipDir // .git, .obsidian, .trash
				}
				return nil
			}
			if !strings.HasSuffix(base, ".md") {
				return nil
			}
			rel, _ := filepath.Rel(root, p)
			rel = filepath.ToSlash(rel)
			for _, ex := range excludes {
				if ok, _ := filepath.Match(ex, rel); ok || strings.Contains(rel, ex) {
					skipped = append(skipped, rel)
					return nil
				}
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			docs = append(docs, api.KnowledgeDoc{Path: rel, Content: string(b)})
			return nil
		})
		if err != nil {
			return err
		}
		if len(docs) == 0 {
			return fmt.Errorf("no .md files under %s", root)
		}
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		if err := client.KnowledgeSync(corpus, docs); err != nil {
			return err
		}
		fmt.Printf("Pushed %d file(s) to corpus %q", len(docs), corpus)
		if len(skipped) > 0 {
			fmt.Printf(" (%d excluded)", len(skipped))
		}
		fmt.Println(" — embedding in the background.")
		if wait, _ := cmd.Flags().GetBool("wait"); wait {
			for i := 0; i < 90; i++ {
				time.Sleep(2 * time.Second)
				st, err := client.KnowledgeStatus()
				if err != nil {
					return err
				}
				for _, s := range st {
					if s.Corpus != corpus {
						continue
					}
					if s.Status == "error" {
						return fmt.Errorf("sync failed: %s", deref(s.Error))
					}
					if s.Status == "idle" && s.FinishedAt != nil && time.Since(*s.FinishedAt) < 3*time.Minute {
						fmt.Printf("Done: %d docs, %d chunks.\n", s.Docs, s.Chunks)
						return nil
					}
				}
			}
			fmt.Println("Still running — check 'usectl admin knowledge status'.")
		}
		return nil
	},
}

var adminKnowledgeStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show each corpus' size and last sync",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		st, err := client.KnowledgeStatus()
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(st)
		}
		if len(st) == 0 {
			fmt.Println("No corpus synced yet.")
			return nil
		}
		rows := make([][]string, 0, len(st))
		for _, s := range st {
			last := "—"
			if s.FinishedAt != nil {
				last = humanAge(s.FinishedAt.Format(time.RFC3339))
			}
			rows = append(rows, []string{s.Corpus, s.Status, fmt.Sprint(s.Docs), fmt.Sprint(s.Chunks), last, deref(s.Error)})
		}
		output.Table([]string{"CORPUS", "STATUS", "DOCS", "CHUNKS", "LAST SYNC", "ERROR"}, rows)
		return nil
	},
}

var adminKnowledgeSearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search a corpus (what the search_vault / search_docs tools return)",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		corpus, _ := cmd.Flags().GetString("corpus")
		k, _ := cmd.Flags().GetInt("k")
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		res, err := client.KnowledgeSearch(corpus, strings.Join(args, " "), k)
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(res)
		}
		if len(res) == 0 {
			fmt.Println("No matching notes.")
			return nil
		}
		for i, c := range res {
			head := c.Title
			if c.Heading != "" {
				head += " › " + c.Heading
			}
			fmt.Printf("%d. %s  (%s, score %.3f)\n%s\n\n", i+1, head, c.Path, c.Score, indent(strings.TrimSpace(c.Content)))
		}
		return nil
	},
}

func indent(s string) string {
	lines := strings.Split(s, "\n")
	if len(lines) > 12 {
		lines = append(lines[:12], "   …")
	}
	for i := range lines {
		lines[i] = "   " + lines[i]
	}
	return strings.Join(lines, "\n")
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func init() {
	adminKnowledgeSyncCmd.Flags().String("corpus", "vault", "vault | docs")
	adminKnowledgeSyncCmd.Flags().StringSlice("exclude", nil, "path globs/substrings to leave out (repeatable)")
	adminKnowledgeSyncCmd.Flags().Bool("wait", false, "wait for the embedding run to finish")
	adminKnowledgeSearchCmd.Flags().String("corpus", "vault", "vault | docs")
	adminKnowledgeSearchCmd.Flags().Int("k", 6, "results")
	adminKnowledgeCmd.AddCommand(adminKnowledgeSyncCmd, adminKnowledgeStatusCmd, adminKnowledgeSearchCmd)
	adminCmd.AddCommand(adminKnowledgeCmd)
}
