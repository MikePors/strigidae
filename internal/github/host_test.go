package github

import "testing"

func TestNormalizeHost(test *testing.T) {
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
	for _, testCase := range tests {
		if got := NormalizeHost(testCase.in); got != testCase.want {
			test.Errorf("NormalizeHost(%q) = %q, want %q", testCase.in, got, testCase.want)
		}
	}
}
