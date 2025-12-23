package ssh

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
)

// StartPortForward creates a local port forward using the system ssh binary.
func (e *SystemExecutor) StartPortForward(ctx context.Context, spec PortForwardSpec) (PortForward, error) {
	if e.options.Host == "" {
		return nil, ErrMissingHost
	}

	normalized, err := normalizePortForwardSpec(spec)
	if err != nil {
		return nil, err
	}
	if normalized.LocalPort == 0 {
		return nil, ErrPortForwardLocalRequired
	}

	args, target := buildSSHForwardArgs(e.options, normalized)
	cmdCtx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(cmdCtx, e.binary, append(args, target)...)
	cmd.Stdout = io.Discard

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, err
	}

	forward := &systemPortForward{
		cmd:        cmd,
		cancel:     cancel,
		localAddr:  normalized.localAddr(),
		remoteAddr: normalized.remoteAddr(),
		done:       make(chan error, 1),
		stderr:     &stderr,
	}

	go forward.wait()

	return forward, nil
}

type systemPortForward struct {
	cmd        *exec.Cmd
	cancel     context.CancelFunc
	localAddr  string
	remoteAddr string
	done       chan error
	closeOnce  sync.Once
	finishOnce sync.Once
	stderr     *bytes.Buffer
}

func (f *systemPortForward) LocalAddr() string {
	return f.localAddr
}

func (f *systemPortForward) RemoteAddr() string {
	return f.remoteAddr
}

func (f *systemPortForward) Wait() error {
	if f == nil {
		return nil
	}
	err, ok := <-f.done
	if !ok {
		return nil
	}
	return err
}

func (f *systemPortForward) Close() error {
	if f == nil {
		return nil
	}
	f.closeOnce.Do(func() {
		if f.cancel != nil {
			f.cancel()
		}
		if f.cmd != nil && f.cmd.Process != nil {
			_ = f.cmd.Process.Kill()
		}
	})
	return f.Wait()
}

func (f *systemPortForward) wait() {
	err := f.cmd.Wait()
	if errors.Is(err, context.Canceled) {
		err = nil
	}
	if err != nil && f.stderr != nil && f.stderr.Len() > 0 {
		err = fmt.Errorf("%w: %s", err, f.stderr.String())
	}
	f.finish(err)
}

func (f *systemPortForward) finish(err error) {
	f.finishOnce.Do(func() {
		f.done <- err
		close(f.done)
	})
}

func buildSSHForwardArgs(options ConnectionOptions, spec PortForwardSpec) ([]string, string) {
	args, target := buildSSHArgs(options)
	forward := formatForwardSpec(spec)
	args = append(args, "-N", "-T", "-o", "ExitOnForwardFailure=yes", "-L", forward)
	return args, target
}
