package logmask

import (
	"bytes"
	"testing"

	"github.com/tidwall/pretty"
)

func TestXPathMasker(t *testing.T) {
	tests := []struct {
		name         string
		sensitives   []string
		in, expected string
	}{
		{
			"full path",
			[]string{"/user/pin"},
			`{"foobar": "abcd", "user": {"id": 12345, "pin": "1111"}}`,
			`{"foobar": "abcd", "user": {"id": 12345, "pin": "` + sensitivePlaceholder + `"}}`,
		},
		{
			"wildcard path",
			[]string{"/data/*/access_token"},
			`{
				"foobar": "abcd",
				"data": {
					"client": {"access_token": "123sadf5t43wreghyu6574ewrfghryeu57s4623qert"},
					"server": {"access_token": "87654erdfgty654refdgyu67543ewdfgty67543edft"},
					"oauth": {"access_token": {"service_type": "test", "token": "1232131232189072389uhsjyidujdwefjsd"}}
				}
			}`,
			`{
				"foobar": "abcd",
				"data": {
					"client": {"access_token": "123sad` + sensitivePlaceholder + `"},
					"server": {"access_token": "87654e` + sensitivePlaceholder + `"},
					"oauth": {"access_token": "` + sensitivePlaceholder + `"}
				}
			}`,
		},
		{
			"recursive path",
			[]string{"//signature"},
			`{
				"signature": "sdahguytijhgyfty78uijhbgt7y8uihgyft67yuhgfdser5t6y7u8iojkhgf",
				"user": {
					"id": 768876867,
					"signature": "abcdefghijkl"
				}
			}`,
			`{
				"signature": "sdahgu` + sensitivePlaceholder + `",
				"user": {
					"id": 768876867,
					"signature": "abc` + sensitivePlaceholder + `"
				}
			}`,
		},
		{
			"logic",
			[]string{"/error/request_params/*[key='client_secret' or key='token']/value"},
			`{
				"error": {
					"request_params": [
						{"key": "token", "value": "sfdgjhkndrhjkbhreknhj4382452354343525234"},
						{"key": "client_secret", "value": "zcxnvhfhjrekerjdi845930202"},
						{"key": "user_name", "value": "vasya pupkin"}
					]
				}
			}`,
			`{
				"error": {
					"request_params": [
						{"key": "token", "value": "sfdgjh` + sensitivePlaceholder + `"},
						{"key": "client_secret", "value": "zcxnvh` + sensitivePlaceholder + `"},
						{"key": "user_name", "value": "vasya pupkin"}
					]
				}
			}`,
		},
	}

	for _, tt := range tests {
		got, err := NewXPathMasker(tt.sensitives).MaskJSON([]byte(tt.in))
		if err != nil {
			t.Errorf("%s: %v", tt.name, err)
		} else if !bytes.Equal(pretty.Ugly(got), pretty.Ugly([]byte(tt.expected))) {
			t.Errorf("%s: got: %s, expected: %s", tt.name, pretty.Pretty(got), pretty.Pretty([]byte(tt.expected)))
		}
	}
}
