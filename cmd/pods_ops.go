package cmd

import (
	"fmt"
	"strconv"

	"github.com/giorgi/usectl/api"
	"github.com/giorgi/usectl/output"
	"github.com/spf13/cobra"
)

// Pod operations carried over from the removed `usectl apps` group.
//
// The bodies are the originals; only target resolution changes: they took two
// raw UUIDs, and now accept machine and pod by name, in either order, with the
// machine optionally coming from -m / $USECTL_MACHINE / `usectl use`.

var podsStartCmd = &cobra.Command{
	Use:   "start [machine] <pod>",
	Short: "Start an app (restore replicas from 0)",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		machineID, podID, rest, err := resolveMachineAndPod(client, args)
		if err != nil {
			return err
		}
		_ = rest
		if err := client.StartApp(machineID, podID); err != nil {
			return err
		}
		fmt.Println("✓ App started")
		return nil
	},
}

var podsStopCmd = &cobra.Command{
	Use:   "stop [machine] <pod>",
	Short: "Stop an app (scale Deployment to 0)",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		machineID, podID, rest, err := resolveMachineAndPod(client, args)
		if err != nil {
			return err
		}
		_ = rest
		if err := client.StopApp(machineID, podID); err != nil {
			return err
		}
		fmt.Println("✓ App stopped")
		return nil
	},
}

var podsInternalCmd = &cobra.Command{
	Use:   "internal [machine] <pod>",
	Short: "Show the cluster-internal address for app-to-app calls",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		machineID, podID, rest, err := resolveMachineAndPod(client, args)
		if err != nil {
			return err
		}
		_ = rest
		addr, err := client.GetAppInternalAddress(machineID, podID)
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(addr)
		}
		rows := [][]string{
			{"Service", addr.ServiceName},
			{"Namespace", addr.Namespace},
		}
		if len(addr.Ports) > 0 {
			// mig 059: one address per port (primary :80 + any extras).
			for _, p := range addr.Ports {
				rows = append(rows,
					[]string{fmt.Sprintf("%s (%d/%s)", p.Name, p.Port, p.Protocol), p.URLShort},
					[]string{"  FQDN", p.URLFQDN},
				)
			}
		} else {
			rows = append(rows,
				[]string{"Port", strconv.Itoa(addr.Port)},
				[]string{"Short DNS", addr.ShortDNS},
				[]string{"FQDN", addr.FQDN},
				[]string{"URL (short)", addr.URLShort},
				[]string{"URL (FQDN)", addr.URLFQDN},
			)
		}
		output.Table([]string{"FIELD", "VALUE"}, rows)
		return nil
	},
}

var podsRevealCmd = &cobra.Command{
	Use:   "reveal [machine] <pod> <key>",
	Short: "Reveal the unmasked value of a single variable (audited)",
	Args:  cobra.RangeArgs(2, 3),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		machineID, podID, rest, err := resolveMachineAndPod(client, args)
		if err != nil {
			return err
		}
		_ = rest
		val, err := client.RevealAppVariable(machineID, podID, rest[0])
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(map[string]string{"key": args[2], "value": val})
		}
		fmt.Println(val)
		return nil
	},
}

// --- per-app envs ---

var podsTrafficCmd = &cobra.Command{
	Use:   "traffic [machine] <pod>",
	Short: "Show Traefik request metrics (rate, p50/p95/p99, bytes, codes)",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		machineID, podID, rest, err := resolveMachineAndPod(client, args)
		if err != nil {
			return err
		}
		_ = rest
		t, err := client.GetAppTraffic(machineID, podID)
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(t)
		}
		if t.NoRouters {
			fmt.Println("No Traefik routers found for this app yet.")
			return nil
		}
		output.Table([]string{"FIELD", "VALUE"}, [][]string{
			{"Total Requests", strconv.FormatInt(t.RequestsTotal, 10)},
			{"Last 5m Requests", strconv.FormatInt(t.Requests5m, 10)},
			{"Request Rate (req/s)", fmt.Sprintf("%.3f", t.RequestRate)},
			{"Avg Duration", fmt.Sprintf("%.1f ms", t.AvgDurationMs)},
			{"p50 / p95 / p99", fmt.Sprintf("%.1f / %.1f / %.1f ms", t.P50Ms, t.P95Ms, t.P99Ms)},
			{"Bytes In Rate", fmt.Sprintf("%.1f B/s", t.BytesInRate)},
			{"Bytes Out Rate", fmt.Sprintf("%.1f B/s", t.BytesOutRate)},
			{"Open Connections", strconv.FormatInt(t.OpenConnections, 10)},
		})
		if len(t.RequestsByCode) > 0 {
			fmt.Println("\nBy code class:")
			rows := [][]string{}
			for k, v := range t.RequestsByCode {
				rows = append(rows, []string{k, strconv.FormatInt(v, 10)})
			}
			output.Table([]string{"CLASS", "COUNT"}, rows)
		}
		if t.GrafanaURL != "" {
			fmt.Printf("\nGrafana: %s\n", t.GrafanaURL)
		}
		return nil
	},
}

var podsInsightsCmd = &cobra.Command{
	Use:   "insights [machine] <pod>",
	Short: "Show per-pod CPU/memory history + recent error logs",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(apiURL)
		if err != nil {
			return err
		}
		machineID, podID, rest, err := resolveMachineAndPod(client, args)
		if err != nil {
			return err
		}
		_ = rest
		ins, err := client.GetAppInsights(machineID, podID)
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(ins)
		}
		fmt.Printf("Window: %ds (step %ds)\n\n", ins.WindowSeconds, ins.StepSeconds)
		if len(ins.ResourceHistory) == 0 {
			fmt.Println("No resource history available.")
		} else {
			rows := make([][]string, 0, len(ins.ResourceHistory))
			for _, p := range ins.ResourceHistory {
				lastCPU, lastMem := "-", "-"
				if n := len(p.CPU); n > 0 {
					lastCPU = fmt.Sprintf("%.3f cores", p.CPU[n-1].V)
				}
				if n := len(p.Memory); n > 0 {
					lastMem = fmt.Sprintf("%.0f MiB", p.Memory[n-1].V/(1024*1024))
				}
				rows = append(rows, []string{p.Pod, lastCPU, lastMem,
					strconv.Itoa(len(p.CPU)), strconv.Itoa(len(p.Memory))})
			}
			output.Table([]string{"POD", "CPU (last)", "MEM (last)", "CPU pts", "MEM pts"}, rows)
		}
		if !ins.ErrorsAvailable {
			fmt.Println("\n(error log lookup unavailable)")
			return nil
		}
		fmt.Printf("\nRecent errors (%d):\n", len(ins.RecentErrors))
		for _, e := range ins.RecentErrors {
			fmt.Printf("[%s] %s/%s: %s\n", e.Timestamp, e.Pod, e.Container, e.Line)
		}
		return nil
	},
}
