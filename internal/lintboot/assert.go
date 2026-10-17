package lintboot

import (
	"fmt"
	"strings"

	"github.com/eonicman/pinscher/internal/probe"
)

// AssertionResult is the outcome of one `--assert` check.
type AssertionResult struct {
	Raw      string `json:"raw"`             // the original "field:device=value" string, verbatim
	Field    string `json:"field"`
	Device   string `json:"device"`          // the device-identifier query, as given
	Expected string `json:"expected"`
	Actual   string `json:"actual"`          // empty if the device wasn't found
	Pass     bool   `json:"pass"`
	Error    string `json:"error,omitempty"` // set only when the device lookup itself failed
}

// assertableFields maps a --assert field name to how it reads off a probe.Device.
// Deliberately a small, explicit allow-list rather than reflection: a typo'd
// field name should be a clear parse error, not a silent "field not found"
// that looks like a failed assertion.
var assertableFields = map[string]func(*probe.Device) string{
	"driver":      func(d *probe.Device) string { return d.Driver },
	"bus":         func(d *probe.Device) string { return d.Bus },
	"vendor":      func(d *probe.Device) string { return d.Vendor },
	"device_id":   func(d *probe.Device) string { return d.DeviceID },
	"class":       func(d *probe.Device) string { return d.Class },
	"description": func(d *probe.Device) string { return d.Description },
}

// ParseAssertion splits one raw --assert argument into field, device query,
// and expected value. Format: "<field>:<device-query>=<value>".
//
// Field names never contain ':' or '=', so splitting on the FIRST ':' is
// always safe even though device queries routinely do contain both —
// PCI addresses ("0000:02:00.0") and vendor:device IDs ("1002:6798") have
// colons, and none of that can appear before the field name. The expected
// value is split off at the LAST '=' for the same reason: device queries
// never contain '=', but being conservative about where the split happens
// costs nothing.
func ParseAssertion(raw string) (field, device, value string, err error) {
	colon := strings.Index(raw, ":")
	if colon < 0 {
		return "", "", "", fmt.Errorf("--assert %q: expected \"field:device=value\" (missing ':')", raw)
	}
	field = raw[:colon]
	rest := raw[colon+1:]

	if _, ok := assertableFields[field]; !ok {
		known := make([]string, 0, len(assertableFields))
		for k := range assertableFields {
			known = append(known, k)
		}
		return "", "", "", fmt.Errorf("--assert %q: unknown field %q (known: %s)", raw, field, strings.Join(known, ", "))
	}

	eq := strings.LastIndex(rest, "=")
	if eq < 0 {
		return "", "", "", fmt.Errorf("--assert %q: expected \"field:device=value\" (missing '=')", raw)
	}
	device = rest[:eq]
	value = rest[eq+1:]
	if device == "" {
		return "", "", "", fmt.Errorf("--assert %q: empty device query", raw)
	}
	return field, device, value, nil
}

// CheckAssertions evaluates every raw --assert string against one shared
// probe of the current system (all assertions see the same hardware
// snapshot, not N separate probes). A malformed assertion string is a hard
// error — it's a typo the user should fix, not a result to report. A
// device that can't be found is reported as a failed AssertionResult with
// Error set, not a hard error, so one bad device query doesn't hide the
// results of every other assertion in the same run.
func CheckAssertions(raws []string) ([]AssertionResult, error) {
	if len(raws) == 0 {
		return nil, nil
	}

	probeResult, err := probe.Probe()
	if err != nil {
		return nil, fmt.Errorf("probe failed: %w", err)
	}

	results := make([]AssertionResult, 0, len(raws))
	for _, raw := range raws {
		field, deviceQuery, expected, err := ParseAssertion(raw)
		if err != nil {
			return nil, err
		}

		res := AssertionResult{Raw: raw, Field: field, Device: deviceQuery, Expected: expected}

		dev, err := probe.FindDevice(probeResult, deviceQuery)
		if err != nil {
			res.Error = err.Error()
			res.Pass = false
			results = append(results, res)
			continue
		}

		res.Actual = assertableFields[field](dev)
		res.Pass = res.Actual == expected
		results = append(results, res)
	}
	return results, nil
}
