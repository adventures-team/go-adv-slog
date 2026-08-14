package logmask

import "net/url"

// ValuesMasker masks sensitive data in URL parameters. It holds the internal
// representation of the parameter names to mask.
type ValuesMasker struct {
	sensitives map[string]struct{}
}

// NewValuesMasker creates a masker for the given sensitive URL parameter
// names.
func NewValuesMasker(sensitives []string) ValuesMasker {
	sensitivesMap := make(map[string]struct{})
	for _, s := range sensitives {
		sensitivesMap[s] = struct{}{}
	}

	return ValuesMasker{
		sensitives: sensitivesMap,
	}
}

// MaskValues masks the url.Values parameters listed in the ValuesMasker.
func (vm ValuesMasker) MaskValues(in url.Values) url.Values {
	masked := make(url.Values, len(in))

	for key, values := range in {
		if _, ok := vm.sensitives[key]; ok {
			newValues := make([]string, len(values))
			for i, value := range values {
				newValues[i] = MaskString(value)
			}

			masked[key] = newValues
		} else {
			masked[key] = values
		}
	}

	return masked
}
