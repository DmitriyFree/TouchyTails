// --- devicestore/device_runtime.go ---
package devicestore

import (
	"fmt"
	"sync"
	"time"

	"touchytails/blemanager"
)

type RuntimeManager struct {
	console guiConsole // interface to send GUI updates
	active  map[string]struct{}
	mu      sync.Mutex
}

// guiConsole is minimal interface for RunBLEManagers
type guiConsole interface {
	Append(msg string)
	ApplyStatus(dev *Device, status string)
}

// NewRuntimeManager creates a new runtime BLE manager for devices
func NewRuntimeManager(console guiConsole) *RuntimeManager {
	return &RuntimeManager{
		console: console,
		active:  make(map[string]struct{}),
	}
}

// Run starts BLE management loop
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

				// Start device management
				go rm.manageDevice(store, dev)
			}
			time.Sleep(3 * time.Second)
		}
	}()
}

// manageDevice handles connection/heartbeat for a single device
func (rm *RuntimeManager) manageDevice(store *DeviceStore, dev *Device) {
	defer func() {
		rm.mu.Lock()
		delete(rm.active, dev.ID)
		rm.mu.Unlock()
	}()

	ble := blemanager.New()
	store.SetBLE(dev.ID, ble)

	for store.IsEnabled(dev.ID) {
		rm.console.Append(fmt.Sprintf("Scanning/connecting to %s (%s)...", dev.Name, dev.ID))

		ble := blemanager.New() // <-- create a fresh one each attempt
		store.SetBLE(dev.ID, ble)

		if err := ble.ConnectDevice(dev.ID); err != nil {
			rm.console.Append(fmt.Sprintf("Failed to connect %s: %v", dev.Name, err))
			store.ClearBLE(dev.ID) // cleanup reference
			time.Sleep(5 * time.Second)
			continue
		}

		rm.console.Append(fmt.Sprintf("%s connected!", dev.Name))
		store.SetOnline(dev.ID, true)
		rm.console.ApplyStatus(dev, "Online")

		// Heartbeat loop
		for store.IsEnabled(dev.ID) && ble.Ready() {
			ble.Send("ping")
			time.Sleep(2 * time.Second)
		}

		// Disconnect and cleanup
		ble.Disconnect()
		store.SetOnline(dev.ID, false)
		store.ClearBLE(dev.ID)
		rm.console.ApplyStatus(dev, "Offline")

		if !store.IsEnabled(dev.ID) {
			rm.console.ApplyStatus(dev, "Disabled")
			return
		}
	}
}
