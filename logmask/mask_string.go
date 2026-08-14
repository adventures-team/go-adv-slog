// Package logmask masks sensitive values in logged data: plain strings, URL
// query parameters ([ValuesMasker]) and JSON documents addressed by XPath
// ([XPathMasker]).
package logmask

const (
	maskMaxSnippet       = 6
	maskMaxRatio         = 4
	maskMinLen           = maskMaxSnippet * maskMaxRatio / 3
	sensitivePlaceholder = "*****"
)

// MaskString masks a string. Strings shorter than 8 bytes are replaced with
// the placeholder entirely; longer ones keep their first len/4 bytes (at most
// 6) to ease debugging.
func MaskString(in string) string {
	if len(in) < maskMinLen {
		return sensitivePlaceholder
	}

	snippetLen := len(in) / maskMaxRatio
	if snippetLen > maskMaxSnippet {
		snippetLen = maskMaxSnippet
	}

	return in[:snippetLen] + sensitivePlaceholder
}
