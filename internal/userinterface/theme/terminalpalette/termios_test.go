//go:build linux || darwin || freebsd || netbsd || openbsd || dragonfly

package terminalpalette

import (
	"testing"
	"time"
)

func TestReadTimeout(test *testing.T) {
	tests := []struct {
		wait time.Duration
		want uint8
	}{
		{0, 1},
		{time.Millisecond, 1},
		{70 * time.Millisecond, 1},
		{100 * time.Millisecond, 1},
		{101 * time.Millisecond, 2},
		{25 * time.Second, 250},
		{time.Hour, 255},
	}
	for _, testCase := range tests {
		if got := readTimeout(testCase.wait); got != testCase.want {
			test.Errorf("readTimeout(%v) = %d, want %d", testCase.wait, got, testCase.want)
		}
	}
}
