package version

import "testing"

func TestStringPrefersLdflagsVersion(test *testing.T) {
	old := Version
	test.Cleanup(func() { Version = old })

	Version = "v1.2.3"
	if got := String(); got != "v1.2.3" {
		test.Fatalf("String() = %q, want %q", got, "v1.2.3")
	}
}

func TestStringFallsBack(test *testing.T) {
	old := Version
	test.Cleanup(func() { Version = old })

	Version = ""
	if got := String(); got == "" {
		test.Fatal("String() returned empty string")
	}
}
