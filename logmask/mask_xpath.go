package logmask

import (
	"bytes"
	"strconv"

	"github.com/antchfx/jsonquery"
	"github.com/tidwall/pretty"
	"github.com/tidwall/sjson"
	"go4.org/bytereplacer"
)

// XPathMasker masks sensitive data inside JSON addressed by XPath paths. The
// matched values are masked with [MaskString] for strings, or replaced with
// the placeholder entirely for other types (TODO: would a debugging-friendly
// masking for structures be useful? no need seen so far).
//
// Paths use the jsonquery syntax (gjson cannot address arbitrary nesting
// levels: its * always matches exactly one level). Examples:
//
//   - key_name    - masks key_name in the root element only (and nowhere else)
//   - /*/key_name - masks key_name in all children of the root element
//   - //key_name  - masks key_name at any nesting level
//
// TODO An external system may also send broken JSON with sensitive data
// (there have been cases of XML arriving with the same fields). The only way
// to handle that is regexp processing. It is not 100% reliable and the
// situation is very rare, so it is not supported for now; possibly an XPath →
// regexp converter and a JSONMasker interface will appear, with
// implementations like:
//   - XPathMasker   - partial-parsing JSON libraries (jsonquery, gjson, sjson)
//   - MarshalMasker - unmarshal into map[string]interface{}, walk recursively
//     with type switch/reflect, marshal back
//   - RegexpMasker  - simple, unreliable, but necessary for (partial)
//     protection against broken JSON
type XPathMasker struct {
	// NeedTrimSpace enables additional whitespace removal from the JSON (for
	// servers responding with pretty-printed JSON). Off by default: the
	// operation is not fast. Implemented with github.com/tidwall/pretty.Ugly.
	NeedTrimSpace bool

	sensitives []string
}

// NewXPathMasker creates a masker removing sensitive data from JSON.
// sensitives lists the paths to mask, in XPath syntax.
func NewXPathMasker(sensitives []string) XPathMasker {
	return XPathMasker{
		sensitives: sensitives,
	}
}

// JSON with newlines is valid, but log records must stay single-line, so the
// newlines are cut out. The remaining escapes match what encoding/json emits
// for these characters (html.EscapeString is too powerful).
var prepareRawJSON = bytereplacer.New(
	"\n", "",
	"\r", "",
	"<", `\u003c`,
	">", `\u003e`,
	"&", `\u0026`,
	"\u2028", `\u2028`,
	"\u2029", `\u2029`,
)

// MaskJSON cuts all the sensitive data listed in the masker out of j. The
// JSON is parsed with jsonquery.Parse, so no prior validation
// (encoding/json.Valid) is required.
func (m XPathMasker) MaskJSON(j []byte) ([]byte, error) {
	doc, err := jsonquery.Parse(bytes.NewBuffer(j))
	if err != nil {
		return nil, err
	}

	for _, pattern := range m.sensitives {
		matches, err := jsonquery.QueryAll(doc, pattern)
		if err != nil {
			return nil, err
		}

		for _, match := range matches {
			masked := sensitivePlaceholder
			if match.FirstChild == match.LastChild { // a match with sub-objects is replaced with the placeholder entirely; otherwise masked as a string
				masked = MaskString(match.InnerText()) //nolint:staticcheck
			}

			j, err = sjson.SetBytes(j, nodePath(match), masked) // TODO something about efficiency ))
			if err != nil {
				return nil, err
			}
		}
	}

	j = prepareRawJSON.Replace(j)

	if m.NeedTrimSpace {
		j = pretty.Ugly(j)
	}

	return j, nil
}

func nodePath(node *jsonquery.Node) string {
	name := ""

	for ; node != nil; node = node.Parent {
		if node.Type == jsonquery.DocumentNode {
			break
		}

		element := node.Data
		if element == "" { // array elements have empty names - find the element index
			i := 0
			for p := node.PrevSibling; p != nil; p = p.PrevSibling {
				i++
			}

			element = strconv.Itoa(i)
		}

		if name == "" {
			name = element
		} else {
			name = element + "." + name // string concatenation is fine at these volumes
		}
	}

	return name
}

// TODO tests _and_ benchmarks. It looks like unmarshal/marshal would be faster
// than sjson.SetBytes rewriting the whole JSON on every replacement; however,
// there is rarely more than 1 replacement, so the difference may be
// insignificant.
