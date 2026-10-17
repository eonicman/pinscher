package probe

import (
	"fmt"
	"strings"
)

// FindDevice finds the first device in result matching query by, in order:
// full sysfs path, PCI address suffix, vendor:device ID (case-insensitive),
// driver name, or a case-insensitive partial match on description.
//
// Added by Ghost, 2026-10-17: this exact matching logic previously existed
// as three separate, slightly-drifted copies (internal/diagnose's private
// findDevice, an inline loop in internal/submit.GenerateIssue, and a fourth
// about to be written for lint-boot --assert). Consolidated here so
// diagnose/quirk-submit/lint-boot --assert all resolve a device identifier
// the same way, and a future bug fix only has to land once.
func FindDevice(result *ProbeResult, query string) (*Device, error) {
	for i := range result.Devices {
		d := result.Devices[i]
		if d.Path == query ||
			strings.HasSuffix(d.Path, "/"+query) ||
			strings.EqualFold(d.Vendor+":"+d.DeviceID, query) ||
			d.Driver == query ||
			(d.Description != "" && strings.Contains(strings.ToLower(d.Description), strings.ToLower(query))) {
			return &d, nil
		}
	}
	return nil, fmt.Errorf("no device found matching '%s'", query)
}
