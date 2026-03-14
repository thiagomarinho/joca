package aws

import "testing"

func TestShortSHA(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"a1b2c3d4e5f6789", "a1b2c3d"},
		{"abc1234", "abc1234"},
		{"abc", "abc"},
		{"", ""},
	}
	for _, tt := range tests {
		got := shortSHA(tt.in)
		if got != tt.want {
			t.Errorf("shortSHA(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
