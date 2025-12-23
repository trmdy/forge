package node

import (
	"context"
	"fmt"

	"github.com/opencode-ai/swarm/internal/models"
	"github.com/opencode-ai/swarm/internal/ssh"
)

// StartPortForward opens a local port forward to a remote node service.
func (s *Service) StartPortForward(ctx context.Context, node *models.Node, spec ssh.PortForwardSpec) (ssh.PortForward, error) {
	if node == nil {
		return nil, ErrNodeNotFound
	}
	if node.IsLocal {
		return nil, fmt.Errorf("local nodes do not require SSH port forwarding")
	}

	executor, err := s.executorForNode(node)
	if err != nil {
		return nil, fmt.Errorf("failed to create executor: %w", err)
	}

	forwarder, ok := executor.(ssh.PortForwarder)
	if !ok {
		_ = executor.Close()
		return nil, ssh.ErrPortForwardUnsupported
	}

	forward, err := forwarder.StartPortForward(ctx, spec)
	if err != nil {
		_ = executor.Close()
		return nil, err
	}

	return &nodePortForward{forward: forward, cleanup: executor.Close}, nil
}

type nodePortForward struct {
	forward ssh.PortForward
	cleanup func() error
}

func (n *nodePortForward) LocalAddr() string {
	return n.forward.LocalAddr()
}

func (n *nodePortForward) RemoteAddr() string {
	return n.forward.RemoteAddr()
}

func (n *nodePortForward) Wait() error {
	return n.forward.Wait()
}

func (n *nodePortForward) Close() error {
	err := n.forward.Close()
	if n.cleanup != nil {
		if cleanupErr := n.cleanup(); err == nil {
			err = cleanupErr
		}
	}
	return err
}
