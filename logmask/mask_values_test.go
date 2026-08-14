package logmask

import (
	"net/url"
	"testing"
)

func TestMaskValues(t *testing.T) {
	urlEncodedPlaceholder := url.PathEscape(sensitivePlaceholder)

	tests := []struct {
		name         string
		sensitives   []string
		in, expected string // url.Values.Encode sorts the keys, simplifying the comparison
	}{
		{
			"single masked value",
			[]string{"pin", "access_token"},
			"pin=1111&user_id=123123123&access_token=0123456789abcdef",
			"access_token=0123" + urlEncodedPlaceholder + "&pin=" + urlEncodedPlaceholder + "&user_id=123123123",
		},
		{
			"multiple masked values",
			[]string{"tokens"},
			"tokens=1231312312&tokens=ghvjbgvhyhujgyuhjgyuhj&tokens=1231231221312318u7897897&user_id=123123123",
			"tokens=12" + urlEncodedPlaceholder + "&tokens=ghvjb" + urlEncodedPlaceholder + "&tokens=123123" + urlEncodedPlaceholder + "&user_id=123123123",
		},
	}

	for _, tt := range tests {
		values, err := url.ParseQuery(tt.in)
		if err != nil {
			t.Fatalf("%s: %v", tt.name, err)
		}

		got := NewValuesMasker(tt.sensitives).MaskValues(values).Encode()
		if got != tt.expected {
			t.Errorf("%s: got: %s, expected: %s", tt.name, got, tt.expected)
		}
	}
}
