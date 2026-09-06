package cmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/giorgi/usectl/api"
	"github.com/spf13/cobra"
)

// Opening and closing extra ports.
//
// The API replaces the extra-port list wholesale (mig 059: a non-nil
// ExtraPorts overwrites everything). Exposing that directly would mean any
// "add one port" command silently deletes every port it did not mention, so
// both commands here read the current list, modify it, and write it back.
//
// Extra ports are cluster-internal only: each yields a Service port but no
// IngressRoute, so they are reachable from sibling pods and nothing else.
// Making a port public is a matter of `pods set <m> <p> port=N`, which moves
// the PRIMARY port — that distinction is why these are separate commands.

var podsOpenPortCmd = &cobra.Command{
	Use:   "open-port [machine] <pod> <port>[/tcp|/udp] [name]",
	Short: "Open an extra cluster-internal port on a pod",
	Long: `Add a port to the pod's Service. Extra ports are reachable only from other
pods in the same namespace — no IngressRoute is created, so nothing is
exposed to the internet.

The port name defaults to "p-<port>". Existing ports are preserved.`,
	Example: `  usectl machines pods open-port api web 9090
  usectl machines pods open-port api web 9094/tcp grpc
  usectl machines pods open-port api web 5353/udp dns`,
	Args: cobra.RangeArgs(2, 4),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		machineID, podID, app, rest, err := resolvePodAppArgs(client, args)
		if err != nil {
			return err
		}
		if len(rest) == 0 {
			return fmt.Errorf("a port is required")
		}

		spec := rest[0]
		proto := "TCP"
		if p, pr, ok := strings.Cut(spec, "/"); ok {
			spec, proto = p, strings.ToUpper(pr)
		}
		if proto != "TCP" && proto != "UDP" {
			return fmt.Errorf("protocol must be tcp or udp, got %q", proto)
		}
		port, err := strconv.Atoi(strings.TrimSpace(spec))
		if err != nil || port < 1 || port > 65535 {
			return fmt.Errorf("port must be 1-65535, got %q", rest[0])
		}
		name := fmt.Sprintf("p-%d", port)
		if len(rest) > 1 {
			name = rest[1]
		}

		if port == app.Port {
			return fmt.Errorf("port %d is already this pod's primary port — extra ports must differ", port)
		}
		next := append([]api.AppPort{}, app.ExtraPorts...)
		for _, p := range next {
			if p.Port == port {
				return fmt.Errorf("port %d is already open on this pod (name %q)", port, p.Name)
			}
		}
		next = append(next, api.AppPort{Name: name, Port: port, Protocol: proto})

		if _, warning, err := client.UpdateProjectApp(machineID, podID, api.UpdateProjectAppRequest{
			ExtraPorts: &next,
		}); err != nil {
			return err
		} else if warning != "" {
			fmt.Printf("⚠ %s\n", warning)
		}
		fmt.Printf("✓ Opened %d/%s (%s) — internal only. Pod rolls to pick it up.\n", port, proto, name)
		return nil
	},
}

var podsClosePortCmd = &cobra.Command{
	Use:     "close-port <machine> <pod> <port>",
	Short:   "Close an extra port on a pod",
	Example: `  usectl machines pods close-port api web 9090`,
	Args:    cobra.RangeArgs(2, 3),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		machineID, podID, app, rest, err := resolvePodAppArgs(client, args)
		if err != nil {
			return err
		}
		if len(rest) == 0 {
			return fmt.Errorf("a port is required")
		}
		port, err := strconv.Atoi(strings.TrimSpace(rest[0]))
		if err != nil {
			return fmt.Errorf("port must be a number, got %q", rest[0])
		}
		if port == app.Port {
			return fmt.Errorf("%d is the pod's primary port and cannot be closed — change it with 'pods set %s port=<n>'",
				port, app.Name)
		}

		next := make([]api.AppPort, 0, len(app.ExtraPorts))
		found := false
		for _, p := range app.ExtraPorts {
			if p.Port == port {
				found = true
				continue
			}
			next = append(next, p)
		}
		if !found {
			return fmt.Errorf("port %d is not open on this pod", port)
		}
		if _, _, err := client.UpdateProjectApp(machineID, podID, api.UpdateProjectAppRequest{
			ExtraPorts: &next,
		}); err != nil {
			return err
		}
		fmt.Printf("✓ Closed %d. Pod rolls to pick it up.\n", port)
		return nil
	},
}

// resolvePodAppArgs resolves the leading machine/pod arguments (with the
// machine optional, per resolveMachineAndPod) and additionally returns the
// pod's current record, which the port commands need in order to modify the
// existing extra-port list rather than replace it.
func resolvePodAppArgs(client *api.Client, args []string) (string, string, *api.ProjectApp, []string, error) {
	machineID, podID, rest, err := resolveMachineAndPod(client, args)
	if err != nil {
		return "", "", nil, nil, err
	}
	apps, err := client.ListProjectApps(machineID)
	if err != nil {
		return "", "", nil, nil, err
	}
	for i := range apps {
		if apps[i].ID == podID {
			return machineID, podID, &apps[i], rest, nil
		}
	}
	return "", "", nil, nil, fmt.Errorf("pod %s not found", podID)
}

// parseExtraPorts parses repeatable --extra-port specs into AppPort values.
// Accepted forms: "name:port/proto", "name:port", "port/proto", "port"
// (e.g. "grpc:9094/tcp", "9094"). Name and protocol defaults are filled in by
// the backend, which also validates ranges and uniqueness — this parser is
// deliberately lenient.
//
// Carried over from the removed `usectl apps` group, which owned it.
func parseExtraPorts(specs []string) ([]api.AppPort, error) {
	out := make([]api.AppPort, 0, len(specs))
	for _, raw := range specs {
		s := strings.TrimSpace(raw)
		if s == "" {
			continue
		}
		proto := ""
		if i := strings.LastIndex(s, "/"); i >= 0 {
			proto = strings.ToUpper(strings.TrimSpace(s[i+1:]))
			s = s[:i]
		}
		name := ""
		portStr := s
		if i := strings.Index(s, ":"); i >= 0 {
			name = strings.TrimSpace(s[:i])
			portStr = strings.TrimSpace(s[i+1:])
		}
		port, err := strconv.Atoi(strings.TrimSpace(portStr))
		if err != nil {
			return nil, fmt.Errorf("invalid --extra-port %q: expected [name:]port[/proto]", raw)
		}
		out = append(out, api.AppPort{Name: name, Port: port, Protocol: proto})
	}
	return out, nil
}
