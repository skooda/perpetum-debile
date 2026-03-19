// animator_test.go
package main

import "testing"

func TestFormatTokens(t *testing.T) {
	tests := []struct {
		n    int64
		want string
	}{
		{0, "0"},
		{999, "999"},
		{1000, "1k"},
		{1234, "1.2k"},
		{9999, "10k"},
		{12345, "12.3k"},
		{100000, "100k"},
	}
	for _, tc := range tests {
		got := formatTokens(tc.n)
		if got != tc.want {
			t.Errorf("formatTokens(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}
