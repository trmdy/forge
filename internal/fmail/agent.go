package fmail

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

// ResolveAgentName resolves the agent name from env or prompts if interactive.
func ResolveAgentName(interactive bool, in io.Reader, out io.Writer) (string, error) {
	if name := strings.TrimSpace(os.Getenv(EnvAgent)); name != "" {
		return NormalizeAgentName(name)
	}

	if !interactive {
		return fmt.Sprintf("anon-%d", os.Getpid()), nil
	}

	if in == nil {
		in = os.Stdin
	}
	if out == nil {
		out = os.Stdout
	}

	fmt.Fprint(out, "Enter agent name: ")
	reader := bufio.NewReader(in)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	name := strings.TrimSpace(line)
	if name == "" {
		return fmt.Sprintf("anon-%d", os.Getpid()), nil
	}
	return NormalizeAgentName(name)
}
