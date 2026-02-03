package cli

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tOgg1/forge/internal/db"
	"github.com/tOgg1/forge/internal/node"
	"github.com/tOgg1/forge/internal/ssh"
)

var (
	nodeForwardLocalHost string
	nodeForwardLocalPort int
	nodeForwardRemote    string
)

func init() {
	nodeCmd.AddCommand(nodeForwardCmd)

	nodeForwardCmd.Flags().StringVar(&nodeForwardRemote, "remote", "", "remote host:port to forward to (required)")
	nodeForwardCmd.Flags().IntVar(&nodeForwardLocalPort, "local-port", 0, "local port to bind (required)")
	nodeForwardCmd.Flags().StringVar(&nodeForwardLocalHost, "local-host", "127.0.0.1", "local host to bind")
	if err := nodeForwardCmd.MarkFlagRequired("remote"); err != nil {
		panic(err)
	}
	if err := nodeForwardCmd.MarkFlagRequired("local-port"); err != nil {
		panic(err)
	}
}

var nodeForwardCmd = &cobra.Command{
	Use:   "forward <name-or-id>",
	Short: "Forward a remote port over SSH",
	Long: `Create an SSH port forward to access a remote service safely.

The remote target should typically be bound to 127.0.0.1 on the node.
The local bind defaults to 127.0.0.1 for safety.`,
	Example: `  # Forward local 8080 to a remote service on port 3000
  forge node forward prod-server --local-port 8080 --remote 127.0.0.1:3000

  # Bind to a different local host
  forge node forward prod-server --local-host 0.0.0.0 --local-port 9090 --remote 127.0.0.1:9090`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()

		nameOrID := args[0]
		remoteHost, remotePort, err := parseForwardTarget(nodeForwardRemote)
		if err != nil {
			return err
		}
		if nodeForwardLocalPort < 1 || nodeForwardLocalPort > 65535 {
			return fmt.Errorf("local port must be between 1 and 65535")
		}

		database, err := openDatabase()
		if err != nil {
			return err
		}
		defer database.Close()

		repo := db.NewNodeRepository(database)
		service := node.NewService(repo, node.WithPublisher(newEventPublisher(database)))

		n, err := findNode(ctx, service, nameOrID)
		if err != nil {
			return err
		}

		spec := ssh.PortForwardSpec{
			LocalHost:  nodeForwardLocalHost,
			LocalPort:  nodeForwardLocalPort,
			RemoteHost: remoteHost,
			RemotePort: remotePort,
		}

		forward, err := service.StartPortForward(ctx, n, spec)
		if err != nil {
			return err
		}
		defer forward.Close()

		if IsJSONOutput() || IsJSONLOutput() {
			if err := WriteOutput(os.Stdout, map[string]any{
				"node_id":     n.ID,
				"node_name":   n.Name,
				"local_addr":  forward.LocalAddr(),
				"remote_addr": forward.RemoteAddr(),
			}); err != nil {
				return err
			}
		} else {
			fmt.Printf("Forwarding %s -> %s via %s. Press Ctrl+C to stop.\n", forward.LocalAddr(), forward.RemoteAddr(), n.Name)
		}

		err = forward.Wait()
		if err != nil && !errors.Is(err, context.Canceled) {
			return fmt.Errorf("port forward stopped: %w", err)
		}
		return nil
	},
}

func parseForwardTarget(value string) (string, int, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", 0, fmt.Errorf("remote target is required")
	}

	if strings.Contains(trimmed, ":") {
		host, portStr, err := net.SplitHostPort(trimmed)
		if err != nil {
			return "", 0, fmt.Errorf("invalid remote target: %w", err)
		}
		port, err := strconv.Atoi(portStr)
		if err != nil {
			return "", 0, fmt.Errorf("invalid remote port: %w", err)
		}
		if port < 1 || port > 65535 {
			return "", 0, fmt.Errorf("remote port must be between 1 and 65535")
		}
		if host == "" {
			host = "127.0.0.1"
		}
		return host, port, nil
	}

	port, err := strconv.Atoi(trimmed)
	if err != nil {
		return "", 0, fmt.Errorf("invalid remote port: %w", err)
	}
	if port < 1 || port > 65535 {
		return "", 0, fmt.Errorf("remote port must be between 1 and 65535")
	}
	return "127.0.0.1", port, nil
}
