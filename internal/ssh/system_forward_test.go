package ssh

import "testing"

func TestBuildSSHForwardArgs(t *testing.T) {
	options := ConnectionOptions{
		Host: "example.com",
		User: "deploy",
	}

	spec := PortForwardSpec{
		LocalHost:  "127.0.0.1",
		LocalPort:  8080,
		RemoteHost: "127.0.0.1",
		RemotePort: 3000,
	}

	args, target := buildSSHForwardArgs(options, spec)
	if target != "deploy@example.com" {
		t.Fatalf("expected target deploy@example.com, got %q", target)
	}

	assertFlagValue(t, args, "-L", "127.0.0.1:8080:127.0.0.1:3000")
	if !hasFlag(args, "-N") {
		t.Fatalf("expected -N in args: %#v", args)
	}
	if !hasFlag(args, "-T") {
		t.Fatalf("expected -T in args: %#v", args)
	}
	assertFlagValue(t, args, "-o", "ExitOnForwardFailure=yes")
}

func hasFlag(args []string, flag string) bool {
	for _, arg := range args {
		if arg == flag {
			return true
		}
	}
	return false
}
