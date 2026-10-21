package cmd

import (
	"fmt"
	"time"

	"github.com/giorgi/usectl/api"
	"github.com/giorgi/usectl/output"
	"github.com/spf13/cobra"
)

var (
	natsAuthMode       string
	natsAuthValidator  string
	natsAuthPath       string
	natsAuthPort       int
	natsAuthSubscribe  []string
	natsAuthPublish    []string
	natsAuthInbox      bool
	natsAuthMaxSession time.Duration
)

var addonsNatsAuthCmd = &cobra.Command{
	Use:   "nats-auth [machine] <addon> --mode password|readonly|callout",
	Short: "Choose how WebSocket clients of a dedicated NATS log in",
	Long: `Sets the WebSocket authentication of a dedicated NATS addon.

  password  Clients use the shared NATS_USER / NATS_PASSWORD. Servers only.
  readonly  No login, subscribe-only. Anyone with the URL can read EVERY
            subject — only for NATS where everything is public.
  callout   Per-user. Each client sends its login token; usectl POSTs it to
            --validator (one of this machine's pods, in the same group) and
            grants only the --subscribe / --publish subjects, with {name}
            placeholders filled from what the validator returns.

The validator receives {"token","client_ip","client_name","server"} with the
header X-Usectl-Nats-Auth: $NATS_AUTH_SECRET and answers 200 with
{"allow":true,"vars":{"org":"7","user":"42"},"expires_at":<unix>}; anything
else refuses the connection. It must be attached to the NATS addon (that is
how it receives NATS_AUTH_SECRET) and restarted after enabling.

Backends connecting with the default user are never affected.`,
	Example: `  usectl machines addons nats-auth fins-stage nats --mode callout \
    --validator fades-backend-stage --subscribe 'fins.org.{org}.>'

  usectl machines addons nats-auth fins-stage nats --mode password`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		machineID, addonID, _, err := resolveMachineAndAddon(client, args)
		if err != nil {
			return err
		}

		body := api.NATSAuthUpdate{Mode: natsAuthMode}
		switch natsAuthMode {
		case "password", "readonly":
		case "callout":
			if natsAuthValidator == "" {
				return fmt.Errorf("--validator is required with --mode callout (the pod that checks tokens)")
			}
			if len(natsAuthSubscribe)+len(natsAuthPublish) == 0 {
				return fmt.Errorf("give at least one --subscribe or --publish rule, e.g. --subscribe 'app.org.{org}.>'")
			}
			appID, err := resolvePod(client, machineID, natsAuthValidator)
			if err != nil {
				return err
			}
			body.ValidatorAppID = appID
			body.Path = natsAuthPath
			body.Port = natsAuthPort
			body.Rules = &api.NATSAuthRules{
				Subscribe:         natsAuthSubscribe,
				Publish:           natsAuthPublish,
				Inbox:             natsAuthInbox,
				MaxSessionSeconds: int64(natsAuthMaxSession / time.Second),
			}
		default:
			return fmt.Errorf("--mode must be password, readonly or callout")
		}

		res, err := client.UpdateNATSAuth(machineID, addonID, body)
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(res)
		}
		fmt.Println("✓ " + res.Message)
		if natsAuthMode == "callout" {
			fmt.Printf("  validator: %s\n", res.ValidatorURL)
			if res.ValidatorAttached != nil && !*res.ValidatorAttached {
				fmt.Printf("  ⚠ %s is not attached to this NATS addon, so it never receives NATS_AUTH_SECRET\n", natsAuthValidator)
				fmt.Println("    and every connection will be refused. Attach it, then redeploy it.")
			} else {
				fmt.Printf("  Restart %s so it picks up NATS_AUTH_SECRET.\n", natsAuthValidator)
			}
		}
		return nil
	},
}

func init() {
	f := addonsNatsAuthCmd.Flags()
	f.StringVar(&natsAuthMode, "mode", "", "password | readonly | callout")
	f.StringVar(&natsAuthValidator, "validator", "", "callout: pod that validates tokens (name or id)")
	f.StringVar(&natsAuthPath, "path", "/nats/auth", "callout: HTTP path on the validator")
	f.IntVar(&natsAuthPort, "port", 80, "callout: validator Service port (80 = its main port, or one of its extra ports)")
	f.StringArrayVar(&natsAuthSubscribe, "subscribe", nil, "callout: subject a client may subscribe to; repeatable; {name} = validator var")
	f.StringArrayVar(&natsAuthPublish, "publish", nil, "callout: subject a client may publish to; repeatable; none = cannot publish")
	f.BoolVar(&natsAuthInbox, "inbox", true, "callout: private reply inbox _INBOX_{user}.> per user")
	f.DurationVar(&natsAuthMaxSession, "max-session", 24*time.Hour, "callout: longest a connection lives before re-auth")
	_ = addonsNatsAuthCmd.MarkFlagRequired("mode")
	addonsCmd.AddCommand(addonsNatsAuthCmd)
}
