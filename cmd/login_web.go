package cmd

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/giorgi/usectl/api"
	"github.com/giorgi/usectl/config"
)

// Browser-based login (OAuth PKCE over a loopback redirect).
//
//	1. Generate a random verifier; send only sha256(verifier) to the browser.
//	2. Listen on 127.0.0.1 on an ephemeral port.
//	3. Open the dashboard's /cli/auth page, which authenticates the user with
//	   the session they already have and asks them to approve.
//	4. The dashboard redirects back to the loopback with a one-time code.
//	5. Exchange code + verifier for a token pair.
//
// The code is visible in the address bar and browser history, so it is treated
// as public; it is worthless without the verifier, which never leaves this
// process. Binding to 127.0.0.1 (not 0.0.0.0) keeps the callback unreachable
// from the network.

// loginTimeout bounds the wait for a human to approve in the browser.
const loginTimeout = 5 * time.Minute

type webLoginResult struct {
	code string
	err  error
}

func runWebLogin(apiBase string) (*api.AuthResponse, error) {
	verifier, err := randomURLSafe(32)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256([]byte(verifier))
	challenge := hex.EncodeToString(sum[:])

	state, err := randomURLSafe(16)
	if err != nil {
		return nil, err
	}

	// Port 0 lets the OS pick a free port; binding before opening the browser
	// guarantees the callback cannot arrive before something is listening.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("start local callback listener: %w", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	results := make(chan webLoginResult, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if gotErr := q.Get("error"); gotErr != "" {
			writeBrowserPage(w, false, gotErr)
			results <- webLoginResult{err: fmt.Errorf("authorization denied: %s", gotErr)}
			return
		}
		// The state check rejects a callback from any flow other than this
		// process's — including a stale tab from an earlier attempt.
		if q.Get("state") != state {
			writeBrowserPage(w, false, "state mismatch")
			results <- webLoginResult{err: fmt.Errorf("state mismatch — ignoring an unexpected callback")}
			return
		}
		code := q.Get("code")
		if code == "" {
			writeBrowserPage(w, false, "no code in callback")
			results <- webLoginResult{err: fmt.Errorf("callback carried no authorization code")}
			return
		}
		writeBrowserPage(w, true, "")
		results <- webLoginResult{code: code}
	})

	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(ln) }()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	authURL := buildAuthorizeURL(apiBase, port, state, challenge)
	fmt.Println("Opening your browser to authorize this CLI…")
	fmt.Printf("  %s\n\n", authURL)
	if err := openBrowser(authURL); err != nil {
		fmt.Println("Could not open a browser automatically — open the URL above manually.")
	}
	fmt.Printf("Waiting for approval (%s)…\n", loginTimeout)

	select {
	case res := <-results:
		if res.err != nil {
			return nil, res.err
		}
		client := api.NewClientUnauth(apiBase)
		return client.ExchangeCLICode(res.code, verifier)
	case <-time.After(loginTimeout):
		return nil, fmt.Errorf("timed out after %s waiting for browser approval — run 'usectl login --password' to sign in with email and password instead", loginTimeout)
	}
}

// buildAuthorizeURL derives the dashboard URL from the API base. The dashboard
// and the API share a host (manager.usectl.com), with the API mounted at /api,
// so stripping a trailing /api yields the web origin.
func buildAuthorizeURL(apiBase string, port int, state, challenge string) string {
	web := strings.TrimSuffix(strings.TrimSuffix(apiBase, "/"), "/api")
	q := url.Values{}
	q.Set("callback", fmt.Sprintf("http://127.0.0.1:%d/callback", port))
	q.Set("state", state)
	q.Set("challenge", challenge)
	q.Set("client", clientDescription())
	return web + "/cli/auth?" + q.Encode()
}

// clientDescription labels the request on the approval screen so the user can
// see which machine is asking.
func clientDescription() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "unknown host"
	}
	return fmt.Sprintf("usectl %s on %s", Version, host)
}

func randomURLSafe(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func writeBrowserPage(w http.ResponseWriter, ok bool, detail string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	title, body := "Login complete", "You can close this tab and return to your terminal."
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		title, body = "Login failed", detail
	}
	fmt.Fprintf(w, `<!doctype html><meta charset="utf-8">
<title>usectl — %s</title>
<style>
 body{font:15px/1.6 -apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;
      display:grid;place-items:center;height:100vh;margin:0;background:#0d0d10;color:#e6e6e6}
 .card{text-align:center;padding:2.5rem 3rem;border:1px solid #2a2a33;border-radius:12px;background:#15151b}
 h1{font-size:1.15rem;margin:0 0 .4rem}
 p{margin:0;color:#9a9aa8}
</style>
<div class=card><h1>%s</h1><p>%s</p></div>`, title, title, body)
}

// saveLogin persists the token pair and reports who signed in.
func saveLogin(resp *api.AuthResponse, apiOverride string) error {
	cfg, _ := config.Load()
	cfg.Token = resp.Token
	cfg.RefreshToken = resp.RefreshToken
	if apiOverride != "" {
		cfg.APIURL = apiOverride
	}
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	fmt.Printf("✓ Logged in as %s (%s)\n", resp.User.Username, resp.User.Email)
	return nil
}
