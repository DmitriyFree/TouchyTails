// --- devicestore/device_runtime.go ---
package devicestore

import (
	"fmt"
	"sync"
	"time"

	"touchytails/blemanager"
)

// RuntimeManager continuously manages BLE connections for enabled devices.
type RuntimeManager struct {
	console ConsoleProxy // interface for logging and status updates
	active  map[string]struct{}
	mu      sync.Mutex
}

// ConsoleProxy is a minimal interface between the runtime and the GUI.
// The main app implements this to safely update Gio UI state.
type ConsoleProxy interface {
	Log(msg string)                       // append text to GUI console
	SetStatus(dev *Device, status string) // update device status text
}

// NewRuntimeManager creates a new runtime BLE manager.
func NewRuntimeManager(console ConsoleProxy) *RuntimeManager {
	return &RuntimeManager{
		console: console,
		active:  make(map[string]struct{}),
	}
}

// Run periodically checks all devices in the store and starts managers for enabled ones.
func (rm *RuntimeManager) Run(store *DeviceStore) {
	go func() {
		for {
			for _, dev := range store.All() {
				if !dev.Enabled {
					continue
				}

				rm.mu.Lock()
				_, running := rm.active[dev.ID]
				if running || dev.BLEPtr != nil {
					rm.mu.Unlock()
					continue
				}
				rm.active[dev.ID] = struct{}{}
				rm.mu.Unlock()

				go rm.manageDevice(store, dev)
			}
			time.Sleep(3 * time.Second)
		}
	}()
}

// manageDevice handles the BLE connection lifecycle for a single device.
func (rm *RuntimeManager) manageDevice(store *DeviceStore, dev *Device) {
	defer func() {
		rm.mu.Lock()
		delete(rm.active, dev.ID)
		rm.mu.Unlock()
	}()

	rm.console.Log(fmt.Sprintf("Managing device %s (%s)", dev.Name, dev.ID))

	for store.IsEnabled(dev.ID) {
		rm.console.Log(fmt.Sprintf("Connecting to %s...", dev.Name))

		ble := blemanager.New()
		store.SetBLE(dev.ID, ble)

		if err := ble.ConnectDevice(dev.ID); err != nil {
			rm.console.Log(fmt.Sprintf("Failed to connect %s: %v", dev.Name, err))
			store.ClearBLE(dev.ID)
			time.Sleep(5 * time.Second)
			continue
		}

		rm.console.Log(fmt.Sprintf("%s connected", dev.Name))
		store.SetOnline(dev.ID, true)
		rm.console.SetStatus(dev, "Online")

		// Heartbeat loop
		for store.IsEnabled(dev.ID) && ble.Ready() {
			ble.Send("ping")
			time.Sleep(2 * time.Second)
		}

		// Disconnect and cleanup
		ble.Disconnect()
		store.SetOnline(dev.ID, false)
		store.ClearBLE(dev.ID)
		rm.console.SetStatus(dev, "Offline")

		if !store.IsEnabled(dev.ID) {
			rm.console.SetStatus(dev, "Disabled")
			return
		}
	}
}
