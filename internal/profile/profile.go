package profile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/eonicman/pinscher/internal/config"
	"github.com/eonicman/pinscher/internal/probe"
	"github.com/eonicman/pinscher/internal/quirk"
)

// Profile represents a hardware configuration profile
type Profile struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Intent      string            `json:"intent"`      // "gaming", "low-power", "audio", "vr", "server", "development"
	Author      string            `json:"author"`
	Version     string            `json:"version"`
	Settings    []ProfileSetting  `json:"settings"`
	Matches     []ProfileMatch    `json:"matches,omitempty"` // hardware conditions for auto-apply
	Conflicts   []string          `json:"conflicts,omitempty"` // profile IDs this conflicts with
}

// ProfileSetting is a single configuration entry
type ProfileSetting struct {
	Module     string            `json:"module"`
	Param      string            `json:"param"`
	Value      string            `json:"value"`
	Reason     string            `json:"reason"`
	Severity   string            `json:"severity"` // "safe", "warning", "dangerous"
	Source     string            `json:"source"`   // "quirk_db", "profile", "manual"
}

// ProfileMatch defines hardware conditions for a profile
type ProfileMatch struct {
	Type     string   `json:"type"`     // "vendor_id", "device_id", "driver", "bus", "class"
	Values   []string `json:"values"`   // matching values
	Required bool     `json:"required"` // must all match (AND) vs any match (OR)
}

// ApplyResult is the result of applying a profile
type ApplyResult struct {
	Profile   string             `json:"profile"`
	Applied   []config.Operation `json:"applied"`
	Skipped   []SkippedSetting   `json:"skipped"`
	Errors    []string           `json:"errors"`
}

// SkippedSetting records why a setting was skipped
type SkippedSetting struct {
	Setting ProfileSetting `json:"setting"`
	Reason  string         `json:"reason"`
}

// Built-in profiles
var BuiltinProfiles = []Profile{
	{
		Name:        "gaming",
		Description: "Optimize for gaming: max GPU performance, low-latency audio, disable powersave",
		Intent:      "gaming",
		Author:      "pinscher",
		Version:     "0.6.0",
		Settings: []ProfileSetting{
			{Module: "amdgpu", Param: "ppfeaturemask", Value: "0xffffffff", Reason: "Enable all PowerPlay features for full GPU control", Severity: "warning", Source: "profile"},
			{Module: "amdgpu", Param: "dc", Value: "1", Reason: "Enable Display Core for multi-monitor support", Severity: "safe", Source: "profile"},
			{Module: "amdgpu", Param: "si_support", Value: "1", Reason: "Enable Southern Islands support (needed for Tahiti XT GPUs)", Severity: "safe", Source: "profile"},
			{Module: "nvme", Param: "default_ps_max_latency_us", Value: "0", Reason: "Disable NVMe APST for consistent I/O latency", Severity: "warning", Source: "profile"},
			{Module: "iwlwifi", Param: "power_save", Value: "0", Reason: "Disable WiFi power save for consistent latency", Severity: "safe", Source: "profile"},
			{Module: "snd_hda_intel", Param: "power_save", Value: "0", Reason: "Disable audio power save to avoid pops and clicks", Severity: "safe", Source: "profile"},
		},
		Matches: []ProfileMatch{
			{Type: "driver", Values: []string{"amdgpu", "nvidia"}, Required: false},
			{Type: "class", Values: []string{"0300", "0302"}, Required: false},
		},
		Conflicts: []string{"low-power"},
	},
	{
		Name:        "low-power",
		Description: "Minimize power consumption: aggressive powersave, disable unused features",
		Intent:      "low-power",
		Author:      "pinscher",
		Version:     "0.6.0",
		Settings: []ProfileSetting{
			{Module: "amdgpu", Param: "ppfeaturemask", Value: "0x0", Reason: "Disable all PowerPlay overdrive (use GPU's default power profile)", Severity: "warning", Source: "profile"},
			{Module: "amdgpu", Param: "abm_level", Value: "3", Reason: "Enable adaptive backlight management (reduces backlight power)", Severity: "safe", Source: "profile"},
			{Module: "nvme", Param: "default_ps_max_latency_us", Value: "120000", Reason: "Allow NVMe deep power states (default is no limit)", Severity: "warning", Source: "profile"},
			{Module: "iwlwifi", Param: "power_save", Value: "1", Reason: "Enable WiFi power save for battery life", Severity: "safe", Source: "profile"},
			{Module: "snd_hda_intel", Param: "power_save", Value: "1", Reason: "Enable audio power save", Severity: "safe", Source: "profile"},
		},
		Matches: []ProfileMatch{
			{Type: "driver", Values: []string{"amdgpu", "i915"}, Required: false},
		},
		Conflicts: []string{"gaming"},
	},
	{
		Name:        "audio-production",
		Description: "Optimize for audio production: low-latency audio, disable powersave, prioritize audio IRQs",
		Intent:      "audio",
		Author:      "pinscher",
		Version:     "0.6.0",
		Settings: []ProfileSetting{
			{Module: "snd_hda_intel", Param: "power_save", Value: "0", Reason: "Disable audio power save (eliminates pops/clicks)", Severity: "safe", Source: "profile"},
			{Module: "snd_hda_intel", Param: "model", Value: "auto", Reason: "Force codec re-probe for correct pin configuration", Severity: "safe", Source: "profile"},
			{Module: "iwlwifi", Param: "power_save", Value: "0", Reason: "Disable WiFi power save (reduces audio-interfering WiFi bursts)", Severity: "safe", Source: "profile"},
		},
		Matches: []ProfileMatch{
			{Type: "driver", Values: []string{"snd_hda_intel", "snd_usb_audio"}, Required: false},
		},
	},
	{
		Name:        "vr",
		Description: "Optimize for VR headsets: low-latency GPU, stable USB 3.0, disable GPU powersave",
		Intent:      "vr",
		Author:      "pinscher",
		Version:     "0.6.0",
		Settings: []ProfileSetting{
			{Module: "amdgpu", Param: "ppfeaturemask", Value: "0xffffffff", Reason: "Enable all PowerPlay features for consistent frame timing", Severity: "warning", Source: "profile"},
			{Module: "amdgpu", Param: "dc", Value: "1", Reason: "Enable Display Core for VR headset detection", Severity: "safe", Source: "profile"},
			{Module: "amdgpu", Param: "si_support", Value: "1", Reason: "Enable Southern Islands support (needed for Tahiti XT)", Severity: "safe", Source: "profile"},
			{Module: "iwlwifi", Param: "power_save", Value: "0", Reason: "Disable WiFi power save (reduces VR latency spikes)", Severity: "safe", Source: "profile"},
			{Module: "nvme", Param: "default_ps_max_latency_us", Value: "0", Reason: "Disable NVMe APST for consistent asset loading", Severity: "warning", Source: "profile"},
		},
		Matches: []ProfileMatch{
			{Type: "driver", Values: []string{"amdgpu", "nvidia"}, Required: false},
		},
		Conflicts: []string{"low-power"},
	},
	{
		Name:        "server",
		Description: "Optimize for server: stable network, disable powersave, prioritize I/O",
		Intent:      "server",
		Author:      "pinscher",
		Version:     "0.6.0",
		Settings: []ProfileSetting{
			{Module: "nvme", Param: "default_ps_max_latency_us", Value: "0", Reason: "Disable NVMe APST for consistent I/O performance", Severity: "warning", Source: "profile"},
			{Module: "iwlwifi", Param: "power_save", Value: "0", Reason: "Disable WiFi power save for consistent network latency", Severity: "safe", Source: "profile"},
			{Module: "amdgpu", Param: "si_support", Value: "1", Reason: "Enable SI support if AMD GPU present (headless servers still need GPU init)", Severity: "safe", Source: "profile"},
		},
	},
	{
		Name:        "macpro61",
		Description: "Mac Pro 6,1 (trash can) — full hardware profile for CachyOS/Linux",
		Intent:      "development",
		Author:      "ami",
		Version:     "0.6.0",
		Settings: []ProfileSetting{
			{Module: "amdgpu", Param: "si_support", Value: "1", Reason: "Tahiti XT GPUs (D300/D500/D700) need si_support=1 for SMC init", Severity: "safe", Source: "quirk_db"},
			{Module: "amdgpu", Param: "dc", Value: "1", Reason: "Enable Display Core for actual display output on mini-DisplayPort", Severity: "safe", Source: "quirk_db"},
		},
		Matches: []ProfileMatch{
			{Type: "vendor_id", Values: []string{"1002"}, Required: false},   // AMD
			{Type: "device_id", Values: []string{"6799", "6798", "679a"}, Required: false}, // Tahiti XT
		},
	},
}

// LoadProfile loads a profile from a file or returns a built-in profile
func LoadProfile(nameOrPath string) (*Profile, error) {
	// Check if it's a built-in profile name
	for i := range BuiltinProfiles {
		if BuiltinProfiles[i].Name == nameOrPath {
			return &BuiltinProfiles[i], nil
		}
	}

	// Try to load from file
	data, err := os.ReadFile(nameOrPath)
	if err != nil {
		return nil, fmt.Errorf("profile '%s' not found (not a built-in profile and not a file: %w)", nameOrPath, err)
	}

	var profile Profile
	if err := json.Unmarshal(data, &profile); err != nil {
		// Try key=value format
		if strings.Contains(string(data), "=") && !strings.Contains(string(data), "{") {
			profile, err = parseKeyValueProfile(string(data), nameOrPath)
			if err != nil {
				return nil, fmt.Errorf("cannot parse profile: %w", err)
			}
		} else {
			return nil, fmt.Errorf("cannot parse profile JSON: %w", err)
		}
	}

	return &profile, nil
}

// parseKeyValueProfile parses a simple key=value profile (backward compat)
func parseKeyValueProfile(content, path string) (Profile, error) {
	profile := Profile{
		Name:    filepath.Base(path),
		Author:  "user",
		Version: "0.6.0",
	}

	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			if strings.HasPrefix(line, "# ") {
				// Use comment as description
				profile.Description = strings.TrimPrefix(line, "# ")
			}
			continue
		}

		// Parse module.param=value
		eqIdx := strings.Index(line, "=")
		if eqIdx < 0 {
			continue
		}
		key := line[:eqIdx]
		value := line[eqIdx+1:]

		dotIdx := strings.Index(key, ".")
		if dotIdx < 0 {
			continue
		}

		module := key[:dotIdx]
		param := key[dotIdx+1:]

		profile.Settings = append(profile.Settings, ProfileSetting{
			Module:   module,
			Param:    param,
			Value:    value,
			Severity: "safe",
			Source:   "profile",
		})
	}

	return profile, nil
}

// ApplyProfile applies a profile's settings with safety checks
func ApplyProfile(profile *Profile, dryRun bool) (*ApplyResult, error) {
	result := &ApplyResult{
		Profile: profile.Name,
	}

	for _, setting := range profile.Settings {
		// Apply via config.SetParam
		op, err := config.SetParam(setting.Module, setting.Param, setting.Value, dryRun)
		if err != nil {
			// Check if it's a safety block
			if strings.Contains(err.Error(), "blocked") {
				result.Skipped = append(result.Skipped, SkippedSetting{
					Setting: setting,
					Reason:  fmt.Sprintf("blocked: %s.%s is not in the safety whitelist", setting.Module, setting.Param),
				})
			} else {
				result.Errors = append(result.Errors, fmt.Sprintf("%s.%s: %v", setting.Module, setting.Param, err))
			}
			continue
		}

		result.Applied = append(result.Applied, *op)
	}

	return result, nil
}

// RecommendProfiles recommends profiles based on the current hardware
func RecommendProfiles(probeResult *probe.ProbeResult) []Profile {
	var recommended []Profile

	for i := range BuiltinProfiles {
		profile := BuiltinProfiles[i]
		if matchesHardware(profile, probeResult) {
			recommended = append(recommended, profile)
		}
	}

	return recommended
}

// matchesHardware checks if a profile matches the current hardware
func matchesHardware(profile Profile, probeResult *probe.ProbeResult) bool {
	if len(profile.Matches) == 0 {
		return true // No match criteria = always applies
	}

	for _, match := range profile.Matches {
		matched := false

		switch match.Type {
		case "driver":
			for _, dev := range probeResult.Devices {
				for _, value := range match.Values {
					if dev.Driver == value {
						matched = true
						break
					}
				}
				if matched {
					break
				}
			}
		case "vendor_id":
			for _, dev := range probeResult.Devices {
				for _, value := range match.Values {
					if strings.EqualFold(dev.Vendor, value) {
						matched = true
						break
					}
				}
				if matched {
					break
				}
			}
		case "device_id":
			for _, dev := range probeResult.Devices {
				for _, value := range match.Values {
					if strings.EqualFold(dev.DeviceID, value) {
						matched = true
						break
					}
				}
				if matched {
					break
				}
			}
		case "class":
			for _, dev := range probeResult.Devices {
				for _, value := range match.Values {
					if strings.HasPrefix(dev.Class, value) {
						matched = true
						break
					}
				}
				if matched {
					break
				}
			}
		case "bus":
			for _, dev := range probeResult.Devices {
				for _, value := range match.Values {
					if dev.Bus == value {
						matched = true
						break
					}
				}
				if matched {
					break
				}
			}
		}

		if !matched && match.Required {
			return false
		}
	}

	return true
}

// EnrichProfileFromQuirks adds quirk-mandated settings to a profile
func EnrichProfileFromQuirks(profile *Profile, probeResult *probe.ProbeResult) *Profile {
	enriched := *profile
	enriched.Settings = make([]ProfileSetting, len(profile.Settings))
	copy(enriched.Settings, profile.Settings)

	for _, dev := range probeResult.Devices {
		// Check device-specific quirks
		if dev.Vendor != "" && dev.DeviceID != "" {
			quirks := quirk.FindByDevice(dev.Vendor, dev.DeviceID)
			for _, q := range quirks {
				for param, value := range q.Params {
					// Check if already in profile
					found := false
					for _, s := range enriched.Settings {
						if s.Module == q.Module && s.Param == param {
							found = true
							break
						}
					}
					if !found {
						enriched.Settings = append(enriched.Settings, ProfileSetting{
							Module:   q.Module,
							Param:    param,
							Value:    value,
							Reason:   q.Description,
							Severity: q.Severity,
							Source:   "quirk_db",
						})
					}
				}
			}
		}

		// Check driver-specific quirks
		if dev.Driver != "" {
			quirks := quirk.FindByModule(dev.Driver)
			for _, q := range quirks {
				for param, value := range q.Params {
					found := false
					for _, s := range enriched.Settings {
						if s.Module == q.Module && s.Param == param {
							found = true
							break
						}
					}
					if !found {
						enriched.Settings = append(enriched.Settings, ProfileSetting{
							Module:   q.Module,
							Param:    param,
							Value:    value,
							Reason:   q.Description,
							Severity: q.Severity,
							Source:   "quirk_db",
						})
					}
				}
			}
		}
	}

	return &enriched
}

// ListProfiles returns the names of all built-in profiles
func ListProfiles() []string {
	names := make([]string, len(BuiltinProfiles))
	for i, p := range BuiltinProfiles {
		names[i] = p.Name
	}
	return names
}

// PrintProfileHuman prints a profile in human-readable format
func PrintProfileHuman(p *Profile) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Profile: %s\n", p.Name))
	sb.WriteString(fmt.Sprintf("Description: %s\n", p.Description))
	sb.WriteString(fmt.Sprintf("Intent: %s\n", p.Intent))
	sb.WriteString(fmt.Sprintf("Author: %s\n", p.Author))
	sb.WriteString(fmt.Sprintf("Version: %s\n\n", p.Version))

	if len(p.Settings) == 0 {
		sb.WriteString("No settings defined.\n")
		return sb.String()
	}

	sb.WriteString(fmt.Sprintf("Settings (%d):\n\n", len(p.Settings)))
	for i, s := range p.Settings {
		icon := "✓"
		switch s.Severity {
		case "warning":
			icon = "⚠"
		case "dangerous":
			icon = "✗"
		}
		sb.WriteString(fmt.Sprintf("%d. %s %s.%s = %s\n", i+1, icon, s.Module, s.Param, s.Value))
		sb.WriteString(fmt.Sprintf("   Reason: %s\n", s.Reason))
		sb.WriteString(fmt.Sprintf("   Source: %s | Severity: %s\n\n", s.Source, s.Severity))
	}

	if len(p.Matches) > 0 {
		sb.WriteString("Hardware matches:\n")
		for _, m := range p.Matches {
			sb.WriteString(fmt.Sprintf("  %s: %s (required=%v)\n", m.Type, strings.Join(m.Values, ", "), m.Required))
		}
		sb.WriteString("\n")
	}

	if len(p.Conflicts) > 0 {
		sb.WriteString(fmt.Sprintf("Conflicts with: %s\n", strings.Join(p.Conflicts, ", ")))
	}

	return sb.String()
}

// JSON returns the profile as formatted JSON
func (p *Profile) JSON() (string, error) {
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}