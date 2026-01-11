package forged

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tOgg1/forge/internal/fmail"
)

type mailListener struct {
	listener   net.Listener
	socketPath string
}

func (d *Daemon) startMailServers(errCh chan<- error) error {
	if d.mailServer == nil {
		return nil
	}

	resolver := newWorkspaceProjectResolver(d.wsRepo)
	tcpAddr := fmt.Sprintf("%s:%d", DefaultHost, DefaultMailPort)
	listener, err := net.Listen("tcp", tcpAddr)
	if err != nil {
		return fmt.Errorf("mail tcp listen: %w", err)
	}
	d.mailListeners = append(d.mailListeners, mailListener{listener: listener})

	go func() {
		if err := d.mailServer.Serve(listener, resolver, true); err != nil {
			errCh <- fmt.Errorf("mail tcp: %w", err)
		}
	}()

	d.logger.Info().Str("bind", tcpAddr).Msg("forge mail tcp server listening")

	if d.wsRepo == nil {
		d.logger.Warn().Msg("mail unix sockets disabled: workspace repository unavailable")
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	workspaces, err := d.wsRepo.List(ctx)
	if err != nil {
		return fmt.Errorf("list workspaces for mail: %w", err)
	}

	for _, workspace := range workspaces {
		root := strings.TrimSpace(workspace.RepoPath)
		if root == "" {
			continue
		}
		if info, err := os.Stat(root); err != nil || !info.IsDir() {
			continue
		}

		if err := ensureMailRoot(root); err != nil {
			d.logger.Warn().Err(err).Str("root", root).Msg("mail root unavailable")
			continue
		}

		socketPath := filepath.Join(root, ".fmail", defaultMailSocketName)
		unixListener, err := listenUnixSocket(socketPath)
		if err != nil {
			d.logger.Warn().Err(err).Str("socket", socketPath).Msg("mail unix listen failed")
			continue
		}

		staticResolver, err := newStaticProjectResolver(root)
		if err != nil {
			d.logger.Warn().Err(err).Str("root", root).Msg("mail resolver init failed")
			_ = unixListener.Close()
			continue
		}

		d.mailListeners = append(d.mailListeners, mailListener{listener: unixListener, socketPath: socketPath})
		go func() {
			if err := d.mailServer.Serve(unixListener, staticResolver, false); err != nil {
				errCh <- fmt.Errorf("mail unix: %w", err)
			}
		}()

		d.logger.Info().Str("socket", socketPath).Msg("forge mail unix server listening")
	}

	return nil
}

func (d *Daemon) shutdownMailServers() {
	for _, entry := range d.mailListeners {
		if entry.listener != nil {
			_ = entry.listener.Close()
		}
		if entry.socketPath != "" {
			_ = os.Remove(entry.socketPath)
		}
	}
	d.mailListeners = nil
}

func ensureMailRoot(root string) error {
	store, err := fmail.NewStore(root)
	if err != nil {
		return err
	}
	return store.EnsureRoot()
}

func listenUnixSocket(path string) (net.Listener, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return net.Listen("unix", path)
}
