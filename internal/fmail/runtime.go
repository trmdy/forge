package fmail

import (
	"context"

	"github.com/spf13/cobra"
)

type runtimeKey struct{}

// Runtime holds derived settings for the current invocation.
type Runtime struct {
	Root  string
	Agent string
}

func RuntimeFromContext(ctx context.Context) (*Runtime, bool) {
	runtime, ok := ctx.Value(runtimeKey{}).(*Runtime)
	return runtime, ok
}

func EnsureRuntime(cmd *cobra.Command) (*Runtime, error) {
	if runtime, ok := RuntimeFromContext(cmd.Context()); ok {
		return runtime, nil
	}

	root, err := DiscoverProjectRoot("")
	if err != nil {
		return nil, Exitf(ExitCodeFailure, "resolve project root: %v", err)
	}

	agent, err := ResolveAgentName(false, nil, nil)
	if err != nil {
		return nil, Exitf(ExitCodeFailure, "resolve agent name: %v", err)
	}
	runtime := &Runtime{
		Root:  root,
		Agent: agent,
	}
	cmd.SetContext(context.WithValue(cmd.Context(), runtimeKey{}, runtime))
	return runtime, nil
}
