package github

import "testing"

func TestNormalizeHost(t *testing.T) {
	tests := []struct{ in, want string }{
		{"github.com", "github.com"},
		{"GitHub.COM", "github.com"},
		{"ghe.example.com:8443", "ghe.example.com"},
		{"ssh.github.com", "github.com"},
		{"SSH.GitHub.com:443", "github.com"},
		{"[::1]:22", "::1"},
		{"::1", "::1"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := NormalizeHost(tt.in); got != tt.want {
			t.Errorf("NormalizeHost(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
