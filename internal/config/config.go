package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Operation represents a config read or write operation
type Operation struct {
	Timestamp  time.Time `json:"timestamp"`
	Module     string    `json:"module"`
	Param      string    `json:"param"`
	OldValue   string    `json:"old_value"`
	NewValue   string    `json:"new_value"`
	DryRun     bool      `json:"dry_run"`
	Success    bool      `json:"success"`
	Error      string    `json:"error,omitempty"`
	RequestedBy string   `json:"requested_by"`
}

// AuditLog records all config operations
var auditLog []Operation

// SafetyLevel defines how dangerous a parameter is
type SafetyLevel int

const (
	Safe    SafetyLevel = iota // read-only params, informational
	Warning                     // writable but reversible
	Dangerous                   // could affect system stability
	Blocked                     // refuse to write (storage controllers, USB host, etc.)
)

// paramSafety defines which module/param combinations are safe to write
var paramSafety = map[string]map[string]SafetyLevel{
	"amdgpu": {
		"si_support":      Safe,    // needed for Tahiti XT, well-tested
		"dc":              Safe,    // display core enable
		"hwpi":            Warning, // hardware page queue implementation
		"ppfeaturemask":   Warning, // power play features
		"vram_post_div":   Warning,
		"lockup_timeout":  Warning,
		"benchmarking":    Warning,
		"test":            Blocked, // debug only
	},
	"nvme": {
		"default_ps_max_latency_us": Warning,
		"poll_queues":               Warning,
		"write_queues":              Warning,
	},
	"snd_hda_intel": {
		"model":         Safe,
		"position_fix":  Warning,
		"probe_mask":    Warning,
		"single_cmd":    Warning,
		"snoop":         Warning,
	},
	"xhci_hcd": {
		"streams":        Warning,
		"quirks":         Warning,
	},
	// Default: unknown params are Blocked
}

// GetParam reads a module parameter value
func GetParam(module, param string) (string, error) {
	path := fmt.Sprintf("/sys/module/%s/parameters/%s", module, param)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("cannot read %s.%s: %w", module, param, err)
	}
	return strings.TrimSpace(string(data)), nil
}

// SetParam writes a module parameter value with safety checks
func SetParam(module, param, value string, dryRun bool) (*Operation, error) {
	op := &Operation{
		Timestamp:  time.Now(),
		Module:     module,
		Param:      param,
		NewValue:   value,
		DryRun:     dryRun,
		RequestedBy: "ami",
	}

	// 1. Safety check
	safety := getSafetyLevel(module, param)
	if safety == Blocked {
		op.Success = false
		op.Error = fmt.Sprintf("blocked: %s.%s is in the blocked list — refusing to write for safety", module, param)
		auditLog = append(auditLog, *op)
		return op, fmt.Errorf("refused: %s.%s is blocked (could affect system stability)", module, param)
	}

	// 2. Read current value
	oldValue, err := GetParam(module, param)
	if err != nil {
		op.Success = false
		op.Error = err.Error()
		auditLog = append(auditLog, *op)
		return op, fmt.Errorf("cannot read current value: %w", err)
	}
	op.OldValue = oldValue

	// 3. Check if already set
	if oldValue == value {
		op.Success = true
		op.Error = "already set to this value"
		auditLog = append(auditLog, *op)
		return op, nil
	}

	// 4. Dry run — don't actually write
	if dryRun {
		op.Success = true
		op.Error = "dry run — no changes made"
		auditLog = append(auditLog, *op)
		return op, nil
	}

	// 5. Write the value
	path := fmt.Sprintf("/sys/module/%s/parameters/%s", module, param)
	err = os.WriteFile(path, []byte(value), 0644)
	if err != nil {
		op.Success = false
		op.Error = err.Error()
		auditLog = append(auditLog, *op)
		return op, fmt.Errorf("write failed: %w (may need root: sudo pinscher set %s %s %s)", err, module, param, value)
	}

	op.Success = true
	auditLog = append(auditLog, *op)
	return op, nil
}

// getSafetyLevel returns the safety level for a module/param combination
func getSafetyLevel(module, param string) SafetyLevel {
	if params, ok := paramSafety[module]; ok {
		if level, ok := params[param]; ok {
			return level
		}
		// Unknown param for known module — default to Blocked
		return Blocked
	}
	// Unknown module — default to Blocked
	return Blocked
}

// ListParams lists all parameters for a module
func ListParams(module string) ([]string, error) {
	dir := fmt.Sprintf("/sys/module/%s/parameters", module)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("cannot read parameters for %s: %w", module, err)
	}
	var params []string
	for _, entry := range entries {
		params = append(params, entry.Name())
	}
	return params, nil
}

// GetAuditLog returns the audit log of all operations
func GetAuditLog() []Operation {
	return auditLog
}

// PrintAuditLog returns the audit log as human-readable text
func PrintAuditLog() string {
	if len(auditLog) == 0 {
		return "No operations logged.\n"
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Pinscher Audit Log (%d operations):\n\n", len(auditLog)))
	for _, op := range auditLog {
		status := "✓"
		if !op.Success {
			status = "✗"
		}
		dry := ""
		if op.DryRun {
			dry = " (DRY RUN)"
		}
		sb.WriteString(fmt.Sprintf("%s [%s] %s.%s: %s → %s%s\n",
			status, op.Timestamp.Format("15:04:05"), op.Module, op.Param, op.OldValue, op.NewValue, dry))
		if op.Error != "" {
			sb.WriteString(fmt.Sprintf("  Note: %s\n", op.Error))
		}
	}
	return sb.String()
}

// ApplyProfile applies multiple settings from a profile (JSON or key=value lines)
// This is a placeholder — full profile support will read JSON files
func ApplyProfile(profilePath string, dryRun bool) ([]Operation, error) {
	data, err := os.ReadFile(profilePath)
	if err != nil {
		return nil, fmt.Errorf("cannot read profile: %w", err)
	}

	// Parse simple key=value format: module.param=value per line
	var ops []Operation
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
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

		op, err := SetParam(module, param, value, dryRun)
		ops = append(ops, *op)
		if err != nil && !dryRun {
			// Continue applying other settings even if one fails
		}
	}

	return ops, nil
}

// Suppress unused import warning
var _ = filepath.Join