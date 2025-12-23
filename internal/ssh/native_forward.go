package ssh

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"

	"github.com/rs/zerolog"
)

// StartPortForward creates a local port forward using the native SSH client.
func (e *NativeExecutor) StartPortForward(ctx context.Context, spec PortForwardSpec) (PortForward, error) {
	if e.options.Host == "" {
		return nil, ErrMissingHost
	}

	normalized, err := normalizePortForwardSpec(spec)
	if err != nil {
		return nil, err
	}

	client, err := e.dial(ctx)
	if err != nil {
		return nil, err
	}

	listener, err := net.Listen("tcp", normalized.localAddr())
	if err != nil {
		_ = client.Close()
		return nil, err
	}

	forward := &nativePortForward{
		listener:   listener,
		client:     client,
		localAddr:  listener.Addr().String(),
		remoteAddr: normalized.remoteAddr(),
		done:       make(chan error, 1),
		logger:     e.logger,
	}

	go forward.serve(ctx)

	return forward, nil
}

type nativePortForward struct {
	listener net.Listener
	client   interface {
		Dial(network, addr string) (net.Conn, error)
		Close() error
	}
	localAddr  string
	remoteAddr string
	done       chan error
	closeOnce  sync.Once
	finishOnce sync.Once
	logger     zerolog.Logger
}

func (f *nativePortForward) LocalAddr() string {
	return f.localAddr
}

func (f *nativePortForward) RemoteAddr() string {
	return f.remoteAddr
}

func (f *nativePortForward) Wait() error {
	if f == nil {
		return nil
	}
	err, ok := <-f.done
	if !ok {
		return nil
	}
	return err
}

func (f *nativePortForward) Close() error {
	if f == nil {
		return nil
	}
	f.closeOnce.Do(func() {
		if f.listener != nil {
			_ = f.listener.Close()
		}
		if f.client != nil {
			_ = f.client.Close()
		}
	})
	return f.Wait()
}

func (f *nativePortForward) serve(ctx context.Context) {
	if ctx != nil {
		go func() {
			<-ctx.Done()
			_ = f.Close()
		}()
	}

	for {
		conn, err := f.listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				f.finish(nil)
				return
			}
			f.finish(err)
			return
		}
		go f.handleConn(conn)
	}
}

func (f *nativePortForward) handleConn(localConn net.Conn) {
	defer localConn.Close()

	remoteConn, err := f.client.Dial("tcp", f.remoteAddr)
	if err != nil {
		f.logger.Warn().Err(err).Str("remote_addr", f.remoteAddr).Msg("port forward dial failed")
		return
	}
	defer remoteConn.Close()

	copyDone := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(remoteConn, localConn)
		copyDone <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(localConn, remoteConn)
		copyDone <- struct{}{}
	}()
	<-copyDone
}

func (f *nativePortForward) finish(err error) {
	f.finishOnce.Do(func() {
		f.done <- err
		close(f.done)
	})
}
