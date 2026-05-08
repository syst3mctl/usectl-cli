package cmd

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/giorgi/usectl/api"
	"github.com/giorgi/usectl/config"
	"github.com/spf13/cobra"
)

// `usectl google login` — sign into usectl via a Google OAuth browser flow.
// Mirrors the structural shape of `usectl github login` but produces a
// usectl JWT (saved to cfg.Token), not a separate Google token. The
// platform admin must have added the loopback redirect URI below to the
// Google Cloud Console OAuth client's "Authorized redirect URIs".
const googleCallbackPort = 17250

var googleCmd = &cobra.Command{
	Use:   "google",
	Short: "Sign into usectl using your Google account",
	Long: `Google Sign In is an alternative to email + password authentication.
Running 'usectl google login' opens a browser, completes the OAuth dance
with Google, exchanges the code for a usectl JWT on the backend, and
saves the JWT to ~/.usectl/config.json — same place 'usectl login' writes
to.

Subcommands:
  login   Open the browser, authenticate, save the JWT.

Setup (platform admin only):
  - GOOGLE_CLIENT_ID + GOOGLE_CLIENT_SECRET must be set on the API server.
  - The Google Cloud Console OAuth client must list
    http://127.0.0.1:17250/callback under "Authorized redirect URIs".`,
}

var googleLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Sign into usectl with Google (browser OAuth)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		// Use the unauth client for the discovery call so the user can run
		// `google login` even without an existing token in their config.
		unauth := api.NewClientUnauth(apiURL)

		info, err := unauth.GetGoogleClientInfo()
		if err != nil {
			return fmt.Errorf("get Google client info: %w (is Google Sign In configured on this instance?)", err)
		}

		// Bind the loopback listener up front. Google OAuth (Web app type)
		// needs the EXACT redirect_uri to match the Cloud Console allowlist
		// — that's why we pin to a single fixed port instead of picking
		// one randomly. If the port is busy, fail loudly rather than fall
		// back to a port the OAuth client doesn't know about.
		listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", googleCallbackPort))
		if err != nil {
			return fmt.Errorf("port %d is busy — close whatever is using it and retry: %w", googleCallbackPort, err)
		}
		redirectURI := fmt.Sprintf("http://127.0.0.1:%d/callback", googleCallbackPort)

		codeCh := make(chan string, 1)
		errCh := make(chan error, 1)

		mux := http.NewServeMux()
		mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
			// Surface OAuth errors (consent denied, invalid_request, ...) so
			// the caller sees something useful instead of a generic timeout.
			if oauthErr := r.URL.Query().Get("error"); oauthErr != "" {
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprintf(w, "<h1>Sign in failed</h1><p>%s</p>", oauthErr)
				errCh <- fmt.Errorf("Google returned error=%s", oauthErr)
				return
			}
			code := r.URL.Query().Get("code")
			if code == "" {
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprint(w, "<h1>Error</h1><p>Missing code parameter.</p>")
				errCh <- fmt.Errorf("Google did not return a code")
				return
			}
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, "<h1>✓ Signed in</h1><p>You can close this tab and return to the terminal.</p>")
			codeCh <- code
		})

		srv := &http.Server{Handler: mux}
		go func() {
			if err := srv.Serve(listener); err != nil && err != http.ErrServerClosed {
				errCh <- err
			}
		}()

		// Build Google's consent URL. `openid email profile` is the minimum
		// the backend needs to populate the user row (sub / email / name /
		// picture). prompt=select_account prevents Google from silently
		// re-authing as the last user when multiple accounts are signed in
		// — important for shared workstations.
		params := url.Values{}
		params.Set("client_id", info.ClientID)
		params.Set("redirect_uri", redirectURI)
		params.Set("response_type", "code")
		params.Set("scope", "openid email profile")
		params.Set("access_type", "online")
		params.Set("prompt", "select_account")
		authURL := "https://accounts.google.com/o/oauth2/v2/auth?" + params.Encode()

		fmt.Println("Opening browser for Google sign in...")
		if err := openBrowser(authURL); err != nil {
			fmt.Printf("Could not open browser. Please visit this URL manually:\n\n  %s\n\n", authURL)
		}
		fmt.Println("Waiting for sign in...")

		var code string
		select {
		case code = <-codeCh:
		case err := <-errCh:
			srv.Shutdown(context.Background())
			return err
		case <-time.After(2 * time.Minute):
			srv.Shutdown(context.Background())
			return fmt.Errorf("timed out waiting for Google authorization")
		}
		srv.Shutdown(context.Background())

		// Exchange the code on the backend (it talks to Google's token
		// endpoint with the client_secret we don't have access to here).
		auth, err := client.LoginWithGoogle(code, redirectURI)
		if err != nil {
			return fmt.Errorf("exchange code: %w", err)
		}

		// Same persistence path as the email/password login — overwrites
		// any existing token so the user lands in a clean signed-in state.
		cfg, _ := config.Load()
		cfg.Token = auth.Token
		if apiURL != "" {
			cfg.APIURL = apiURL
		}
		if err := config.Save(cfg); err != nil {
			return fmt.Errorf("save config: %w", err)
		}

		fmt.Printf("✓ Signed in as %s (%s)\n", auth.User.Username, auth.User.Email)
		return nil
	},
}

func init() {
	googleCmd.AddCommand(googleLoginCmd)
	rootCmd.AddCommand(googleCmd)
}
