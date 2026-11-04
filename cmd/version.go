package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the CLI version",
	Long: `Print the CLI version — the same as --version, for people who type it as a
subcommand. With --json it also reports the platform it was built for.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if jsonOutput {
			return json.NewEncoder(os.Stdout).Encode(map[string]string{
				"version": Version,
				"os":      runtime.GOOS,
				"arch":    runtime.GOARCH,
			})
		}
		fmt.Printf("usectl %s (%s/%s)\n", Version, runtime.GOOS, runtime.GOARCH)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}

// ── Update notice ────────────────────────────────────────────────────────
//
// A customer ran 2.7.0 for weeks while 2.13.0 was out, with nothing telling
// them. The check is deliberately unobtrusive:
//
//   - the latest release is looked up at most once a day and cached in
//     ~/.usectl/update-check.json; the notice itself is printed from the cache
//   - a stale cache is refreshed in the background while the command runs,
//     and Execute waits at most a moment for it — a slow or blocked GitHub
//     never delays a command
//   - it goes to stderr, only on a terminal, never with --json, never for
//     unstamped dev builds, and can be disabled with USECTL_NO_UPDATE_CHECK=1
//   - every failure is silent: it can never fail a command

const (
	updateCheckURL   = "https://api.github.com/repos/syst3mctl/usectl-cli/releases/latest"
	updateCheckEvery = 24 * time.Hour
)

type updateCache struct {
	CheckedAt time.Time `json:"checked_at"`
	Latest    string    `json:"latest"`
}

func updateCachePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".usectl", "update-check.json")
}

func readUpdateCache() updateCache {
	var c updateCache
	if p := updateCachePath(); p != "" {
		if b, err := os.ReadFile(p); err == nil {
			_ = json.Unmarshal(b, &c)
		}
	}
	return c
}

func updateCheckEnabled() bool {
	if os.Getenv("USECTL_NO_UPDATE_CHECK") != "" || jsonOutput {
		return false
	}
	if _, ok := parseSemver(Version); !ok {
		return false // "dev" or a snapshot build: nothing meaningful to compare
	}
	return term.IsTerminal(int(os.Stderr.Fd()))
}

// startUpdateCheck refreshes a stale cache in the background. The returned
// channel closes when the refresh is done (or immediately if none is needed).
func startUpdateCheck() <-chan struct{} {
	done := make(chan struct{})
	if !updateCheckEnabled() || time.Since(readUpdateCache().CheckedAt) < updateCheckEvery {
		close(done)
		return done
	}
	go func() {
		defer close(done)
		defer func() { _ = recover() }()
		req, err := http.NewRequest(http.MethodGet, updateCheckURL, nil)
		if err != nil {
			return
		}
		req.Header.Set("User-Agent", "usectl-cli/"+Version)
		req.Header.Set("Accept", "application/vnd.github+json")
		resp, err := (&http.Client{Timeout: 3 * time.Second}).Do(req)
		if err != nil {
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return
		}
		var rel struct {
			TagName string `json:"tag_name"`
		}
		if json.NewDecoder(resp.Body).Decode(&rel) != nil || rel.TagName == "" {
			return
		}
		p := updateCachePath()
		if p == "" {
			return
		}
		b, _ := json.Marshal(updateCache{CheckedAt: time.Now(), Latest: rel.TagName})
		_ = os.MkdirAll(filepath.Dir(p), 0o700)
		_ = os.WriteFile(p, b, 0o600)
	}()
	return done
}

// printUpdateNotice waits briefly for a background refresh, then prints a
// one-line notice if the cached latest release is newer than this binary.
func printUpdateNotice(done <-chan struct{}) {
	if !updateCheckEnabled() {
		return
	}
	select {
	case <-done:
	case <-time.After(300 * time.Millisecond):
	}
	latest := readUpdateCache().Latest
	lv, ok1 := parseSemver(latest)
	cv, ok2 := parseSemver(Version)
	if !ok1 || !ok2 || !semverLess(cv, lv) {
		return
	}
	fmt.Fprintf(os.Stderr, "\nA new usectl is available: %s (you have %s). Upgrade: brew upgrade --cask usectl, or re-run curl -fsSL https://manager.usectl.com/install.sh | bash\n",
		latest, Version)
}

// parseSemver reads "v1.2.3" / "1.2.3" (ignoring any -pre or +build suffix).
func parseSemver(v string) ([3]int, bool) {
	var out [3]int
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

func semverLess(a, b [3]int) bool {
	for i := 0; i < 3; i++ {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}
