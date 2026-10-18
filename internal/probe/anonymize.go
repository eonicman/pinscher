package probe

import (
	"regexp"
	"strings"
)

// AnonymizedHostname replaces the real hostname in an anonymized report.
const AnonymizedHostname = "anonymized"

// macRe matches a full six-octet MAC address (colon or dash separated). PCI
// addresses (01:00.0, 0000:01:00.0) and modaliases have fewer pairs and do not
// match.
var macRe = regexp.MustCompile(`(?i)\b[0-9a-f]{2}([:-])[0-9a-f]{2}([:-])[0-9a-f]{2}([:-])[0-9a-f]{2}([:-])[0-9a-f]{2}([:-])[0-9a-f]{2}\b`)

// maskMACs keeps the first three octets (the vendor OUI, which is useful for
// hardware reports) and masks the last three, which identify the device.
func maskMACs(s string) string {
	return macRe.ReplaceAllStringFunc(s, func(m string) string {
		sep := string(m[2])
		return m[:8] + sep + "xx" + sep + "xx" + sep + "xx"
	})
}

// Anonymize removes data that identifies a specific machine so a report can be
// posted publicly: the hostname is replaced, MAC addresses keep only their
// vendor prefix, and any capability whose key mentions "serial" is redacted.
// It changes the report in place. Hardware IDs, drivers and descriptions are
// left intact, since those are what make a report useful.
func (r *ProbeResult) Anonymize() {
	r.Hostname = AnonymizedHostname
	for i := range r.Devices {
		d := &r.Devices[i]
		d.Path = maskMACs(d.Path)
		d.Vendor = maskMACs(d.Vendor)
		d.DeviceID = maskMACs(d.DeviceID)
		d.Subsystem = maskMACs(d.Subsystem)
		d.Modalias = maskMACs(d.Modalias)
		d.Description = maskMACs(d.Description)
		d.Revision = maskMACs(d.Revision)
		for k, v := range d.Capabilities {
			if strings.Contains(strings.ToLower(k), "serial") {
				d.Capabilities[k] = "redacted"
				continue
			}
			d.Capabilities[k] = maskMACs(v)
		}
	}
}
