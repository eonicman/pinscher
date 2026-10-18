package watch

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
)

// udevEvent represents a raw event from udevadm monitor
type udevEvent struct {
	Action     string
	Properties map[string]string
}

// startUdevMonitor starts udevadm monitor and returns events as a channel
func startUdevMonitor() (<-chan udevEvent, error) {
	// Try udevadm monitor — this gives us real-time kernel and udev events
	cmd := exec.Command("udevadm", "monitor", "--environment", "--subsystem-match=pci", "--subsystem-match=usb", "--subsystem-match=platform", "--subsystem-match=net", "--subsystem-match=input", "--subsystem-match=bluetooth")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("udevadm pipe failed: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("udevadm start failed: %w", err)
	}

	ch := make(chan udevEvent, 100)

	go func() {
		defer close(ch)
		defer cmd.Process.Kill()

		scanner := bufio.NewScanner(stdout)
		var currentEvent *udevEvent
		var mu sync.Mutex

		for scanner.Scan() {
			line := scanner.Text()

			// Parse udevadm monitor output
			// Format: "ACTION=add" or "KERNEL[1234.56] add /devices/..."
			// Environment lines are KEY=VALUE
			// Blank line separates events

			if strings.TrimSpace(line) == "" {
				// End of event block — emit
				mu.Lock()
				if currentEvent != nil {
					evt := *currentEvent
					currentEvent = nil
					mu.Unlock()
					ch <- evt
				} else {
					mu.Unlock()
				}
				continue
			}

			// Parse header line (KERNEL[timestamp] action devpath)
			if strings.HasPrefix(line, "KERNEL") || strings.HasPrefix(line, "UDEV") {
				// If we have a pending event, emit it
				mu.Lock()
				if currentEvent != nil {
					evt := *currentEvent
					currentEvent = nil
					mu.Unlock()
					ch <- evt
					mu.Lock()
				}

				// Start new event
				parts := strings.SplitN(line, " ", 4)
				action := ""
				if len(parts) >= 2 {
					action = parts[1]
				}

				// Only track add/remove/change actions
				if action == "add" || action == "remove" || action == "change" || action == "move" {
					currentEvent = &udevEvent{
						Action:     action,
						Properties: make(map[string]string),
					}
					// Store the devpath if available
					if len(parts) >= 3 {
						currentEvent.Properties["DEVPATH"] = parts[2]
					}
				}
				mu.Unlock()
				continue
			}

			// Parse KEY=VALUE property lines
			if idx := strings.Index(line, "="); idx > 0 {
				key := line[:idx]
				value := line[idx+1:]

				mu.Lock()
				if currentEvent != nil {
					currentEvent.Properties[key] = value
				}
				mu.Unlock()
			}
		}
	}()

	return ch, nil
}

// ParseUdevJSONEvent parses a udev event from JSON format (for testing)
func ParseUdevJSONEvent(data string) (*udevEvent, error) {
	var props map[string]string
	if err := json.Unmarshal([]byte(data), &props); err != nil {
		return nil, err
	}

	action := props["ACTION"]
	if action == "" {
		action = props["action"]
	}

	return &udevEvent{
		Action:     action,
		Properties: props,
	}, nil
}

// NetlinkWatch watches netlink events directly (more reliable but more complex)
// This is a fallback if udevadm is not available
func NetlinkWatch() (<-chan udevEvent, error) {
	// For now, fall back to udevadm
	// Netlink implementation would use syscall.Socket(AF_NETLINK, ...)
	// but requires root for NETLINK_KOBJECT_UEVENT
	return nil, fmt.Errorf("netlink watch not implemented — use udevadm monitor")
}

// Suppress unused import warning
var _ = strings.TrimSpace
var _ = json.Unmarshal
var _ = os.Args
