package ssh

import "testing"

func TestNormalizePortForwardSpecDefaults(t *testing.T) {
	spec := PortForwardSpec{LocalPort: 8080, RemotePort: 3000}
	normalized, err := normalizePortForwardSpec(spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if normalized.LocalHost != defaultForwardHost {
		t.Fatalf("expected default local host %q, got %q", defaultForwardHost, normalized.LocalHost)
	}
	if normalized.RemoteHost != defaultForwardHost {
		t.Fatalf("expected default remote host %q, got %q", defaultForwardHost, normalized.RemoteHost)
	}
}

func TestNormalizePortForwardSpecValidation(t *testing.T) {
	tests := []struct {
		name string
		spec PortForwardSpec
	}{
		{
			name: "invalid local port",
			spec: PortForwardSpec{LocalPort: -1, RemotePort: 3000},
		},
		{
			name: "invalid remote port",
			spec: PortForwardSpec{LocalPort: 8080, RemotePort: 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := normalizePortForwardSpec(tt.spec)
			if err == nil {
				t.Fatalf("expected validation error")
			}
		})
	}
}
