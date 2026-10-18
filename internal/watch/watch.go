package watch

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"

	"github.com/eonicman/pinscher/internal/probe"
)

// EventType represents the type of hardware event
type EventType string

const (
	EventAdd    EventType = "add"
	EventRemove EventType = "remove"
	EventChange EventType = "change"
)

// HardwareEvent represents a detected hardware change
type HardwareEvent struct {
	Timestamp time.Time    `json:"timestamp"`
	Type      EventType    `json:"type"`
	Device    probe.Device `json:"device"`
	Message   string       `json:"message"`
}

// WatchResult contains the final state when watch ends
type WatchResult struct {
	StartTime  time.Time          `json:"start_time"`
	EndTime    time.Time          `json:"end_time"`
	EventsSeen int                `json:"events_seen"`
	SnapBefore *probe.ProbeResult `json:"snap_before"`
	SnapAfter  *probe.ProbeResult `json:"snap_after"`
}

// WatchOptions configures the watch behavior
type WatchOptions struct {
	Duration      time.Duration       // 0 = run until interrupted
	JSON          bool                // output events as JSON
	Quiet         bool                // suppress continuous output
	OnEvent       func(HardwareEvent) // callback for each event
	SnapshotFirst bool                // take initial probe snapshot before watching
}

// Watcher monitors hardware changes via udev/netlink and periodic snapshots
type Watcher struct {
	mu         sync.Mutex
	options    WatchOptions
	events     []HardwareEvent
	snapBefore *probe.ProbeResult
	snapAfter  *probe.ProbeResult
	stopCh     chan struct{}
}

// NewWatcher creates a new hardware watcher
func NewWatcher(opts WatchOptions) *Watcher {
	return &Watcher{
		options: opts,
		stopCh:  make(chan struct{}),
	}
}

// Watch starts monitoring hardware changes
func (w *Watcher) Watch() (*WatchResult, error) {
	startTime := time.Now()

	// Take initial snapshot if requested
	if w.options.SnapshotFirst {
		snap, err := probe.Probe()
		if err == nil {
			w.snapBefore = snap
		}
	}

	// Try udev monitor (preferred — real-time events)
	udevCh, err := startUdevMonitor()
	if err != nil {
		// Fall back to polling
		return w.watchByPolling(startTime)
	}

	// Use udev monitor for real-time events
	return w.watchUdev(startTime, udevCh)
}

// watchUdev monitors udev events in real-time
func (w *Watcher) watchUdev(startTime time.Time, eventCh <-chan udevEvent) (*WatchResult, error) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	defer signal.Stop(sigCh)

	if !w.options.Quiet {
		fmt.Println("Pinscher Watch — monitoring hardware changes (Ctrl+C to stop)")
		fmt.Println()
	}

	for {
		select {
		case evt, ok := <-eventCh:
			if !ok {
				goto done
			}
			hwEvt := w.processUdevEvent(evt)
			if hwEvt != nil {
				w.mu.Lock()
				w.events = append(w.events, *hwEvt)
				w.mu.Unlock()

				if w.options.OnEvent != nil {
					w.options.OnEvent(*hwEvt)
				}

				if !w.options.Quiet {
					if w.options.JSON {
						data, _ := json.Marshal(hwEvt)
						fmt.Println(string(data))
					} else {
						printEventHuman(*hwEvt)
					}
				}
			}

		case <-sigCh:
			goto done

		case <-w.stopCh:
			goto done

		case <-time.After(w.options.Duration):
			if w.options.Duration > 0 {
				goto done
			}
		}
	}

done:
	endTime := time.Now()

	// Take final snapshot if we had an initial one
	if w.snapBefore != nil {
		snap, err := probe.Probe()
		if err == nil {
			w.snapAfter = snap
		}
	}

	return &WatchResult{
		StartTime:  startTime,
		EndTime:    endTime,
		EventsSeen: len(w.events),
		SnapBefore: w.snapBefore,
		SnapAfter:  w.snapAfter,
	}, nil
}

// watchByPolling uses periodic probing to detect changes (fallback)
func (w *Watcher) watchByPolling(startTime time.Time) (*WatchResult, error) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	defer signal.Stop(sigCh)

	if !w.options.Quiet {
		fmt.Println("Pinscher Watch — polling for hardware changes (Ctrl+C to stop)")
		fmt.Println("(udev monitor unavailable; using polling fallback)")
		fmt.Println()
	}

	// Take initial snapshot
	prevSnap, err := probe.Probe()
	if err != nil {
		return nil, fmt.Errorf("initial probe failed: %w", err)
	}
	w.snapBefore = prevSnap

	pollInterval := 5 * time.Second
	if w.options.Duration > 0 && w.options.Duration < pollInterval {
		pollInterval = w.options.Duration
	}

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			newSnap, err := probe.Probe()
			if err != nil {
				continue
			}

			events := DiffProbes(prevSnap, newSnap)
			for _, evt := range events {
				w.mu.Lock()
				w.events = append(w.events, evt)
				w.mu.Unlock()

				if w.options.OnEvent != nil {
					w.options.OnEvent(evt)
				}

				if !w.options.Quiet {
					if w.options.JSON {
						data, _ := json.Marshal(evt)
						fmt.Println(string(data))
					} else {
						printEventHuman(evt)
					}
				}
			}

			prevSnap = newSnap

		case <-sigCh:
			goto pollDone

		case <-w.stopCh:
			goto pollDone
		}
	}

pollDone:
	endTime := time.Now()
	w.snapAfter = prevSnap

	return &WatchResult{
		StartTime:  startTime,
		EndTime:    endTime,
		EventsSeen: len(w.events),
		SnapBefore: w.snapBefore,
		SnapAfter:  w.snapAfter,
	}, nil
}

// Stop signals the watcher to stop
func (w *Watcher) Stop() {
	close(w.stopCh)
}

// Events returns all collected events
func (w *Watcher) Events() []HardwareEvent {
	w.mu.Lock()
	defer w.mu.Unlock()
	result := make([]HardwareEvent, len(w.events))
	copy(result, w.events)
	return result
}

// processUdevEvent converts a udev event to a HardwareEvent
func (w *Watcher) processUdevEvent(evt udevEvent) *HardwareEvent {
	// Map udev action to our event type
	var etype EventType
	switch evt.Action {
	case "add":
		etype = EventAdd
	case "remove":
		etype = EventRemove
	case "change":
		etype = EventChange
	default:
		return nil // ignore other actions
	}

	// Build a device from udev properties
	dev := probe.Device{
		Bus:         evt.Properties["SUBSYSTEM"],
		Vendor:      strings.TrimPrefix(evt.Properties["ID_VENDOR_ID"], "0x"),
		DeviceID:    strings.TrimPrefix(evt.Properties["ID_MODEL_ID"], "0x"),
		Driver:      evt.Properties["DRIVER"],
		Description: evt.Properties["ID_MODEL_FROM_DATABASE"],
		Modalias:    evt.Properties["MODALIAS"],
		Path:        evt.Properties["DEVPATH"],
	}

	// Enhance description
	if dev.Description == "" {
		dev.Description = evt.Properties["ID_MODEL"]
	}
	if dev.Description == "" {
		dev.Description = evt.Properties["DEVNAME"]
	}
	if dev.Description == "" {
		dev.Description = fmt.Sprintf("%s device", dev.Bus)
	}

	// Build human-readable message
	var msg string
	switch etype {
	case EventAdd:
		msg = fmt.Sprintf("Added %s: %s", dev.Bus, dev.Description)
		if dev.Driver != "" {
			msg += fmt.Sprintf(" (driver: %s)", dev.Driver)
		}
	case EventRemove:
		msg = fmt.Sprintf("Removed %s: %s", dev.Bus, dev.Description)
	case EventChange:
		msg = fmt.Sprintf("Changed %s: %s", dev.Bus, dev.Description)
	}

	return &HardwareEvent{
		Timestamp: time.Now(),
		Type:      etype,
		Device:    dev,
		Message:   msg,
	}
}

// DiffProbes compares two probe results and returns events for differences (exported)
func DiffProbes(old, new *probe.ProbeResult) []HardwareEvent {
	var events []HardwareEvent

	// Build device maps for comparison
	oldDevices := make(map[string]probe.Device)
	for _, d := range old.Devices {
		key := deviceKey(d)
		oldDevices[key] = d
	}

	newDevices := make(map[string]probe.Device)
	for _, d := range new.Devices {
		key := deviceKey(d)
		newDevices[key] = d
	}

	// Find added devices
	for key, d := range newDevices {
		if _, exists := oldDevices[key]; !exists {
			desc := d.Description
			if desc == "" {
				desc = fmt.Sprintf("%s:%s", d.Vendor, d.DeviceID)
			}
			events = append(events, HardwareEvent{
				Timestamp: time.Now(),
				Type:      EventAdd,
				Device:    d,
				Message:   fmt.Sprintf("Added %s: %s", d.Bus, desc),
			})
		}
	}

	// Find removed devices
	for key, d := range oldDevices {
		if _, exists := newDevices[key]; !exists {
			desc := d.Description
			if desc == "" {
				desc = fmt.Sprintf("%s:%s", d.Vendor, d.DeviceID)
			}
			events = append(events, HardwareEvent{
				Timestamp: time.Now(),
				Type:      EventRemove,
				Device:    d,
				Message:   fmt.Sprintf("Removed %s: %s", d.Bus, desc),
			})
		}
	}

	// Find changed devices (driver change)
	for key, newDev := range newDevices {
		if oldDev, exists := oldDevices[key]; exists {
			if oldDev.Driver != newDev.Driver {
				desc := newDev.Description
				if desc == "" {
					desc = fmt.Sprintf("%s:%s", newDev.Vendor, newDev.DeviceID)
				}
				events = append(events, HardwareEvent{
					Timestamp: time.Now(),
					Type:      EventChange,
					Device:    newDev,
					Message:   fmt.Sprintf("Changed %s: %s (driver: %s → %s)", newDev.Bus, desc, oldDev.Driver, newDev.Driver),
				})
			}
		}
	}

	return events
}

// deviceKey creates a unique key for device comparison
func deviceKey(d probe.Device) string {
	if d.Vendor != "" && d.DeviceID != "" {
		return d.Bus + ":" + d.Vendor + ":" + d.DeviceID
	}
	return d.Bus + ":" + d.Path
}

// printEventHuman prints an event in human-readable format
func printEventHuman(evt HardwareEvent) {
	icon := "⊕"
	switch evt.Type {
	case EventAdd:
		icon = "⊕"
	case EventRemove:
		icon = "⊖"
	case EventChange:
		icon = "⟳"
	}
	fmt.Printf("%s [%s] %s\n", icon, evt.Type, evt.Message)
}

// JSON returns the watch result as formatted JSON
func (r *WatchResult) JSON() (string, error) {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}
