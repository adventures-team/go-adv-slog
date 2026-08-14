package logmask

import "testing"

func TestMaskString(t *testing.T) {
	tests := []struct {
		name, in, expected string
	}{
		{
			"short",
			"1111",
			sensitivePlaceholder,
		},
		{
			"medium",
			"0123456789abcdef",
			"0123" + sensitivePlaceholder,
		},
		{
			"long",
			"0123456789abcdefghijklmnopqrstuvwxyz",
			"012345" + sensitivePlaceholder,
		},
	}

	for _, tt := range tests {
		got := MaskString(tt.in)
		if got != tt.expected {
			t.Errorf("%s: got: %s, expected: %s", tt.name, got, tt.expected)
		}
	}
}
