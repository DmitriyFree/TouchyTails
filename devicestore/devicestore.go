package devicestore

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"touchytails/blemanager"

	"fyne.io/fyne/v2/canvas"
)

// Device represents a BLE device.
// Persistent fields are saved to JSON.
// Runtime fields are ignored during save/load.
type Device struct {
	// Persistent
	ID      string `json:"id"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
	Event   string `json:"event"`

	// Runtime-only
	Online bool                   `json:"-"`
	Status *canvas.Text           `json:"-"`
	BLEPtr *blemanager.BLEManager `json:"-"`
}

// DeviceStore manages devices with thread safety and persistence
type DeviceStore struct {
	mu      sync.Mutex
	path    string
	devices []*Device
}

// New creates a new DeviceStore with a given path for JSON storage
func New(path string) *DeviceStore {
	return &DeviceStore{path: path, devices: []*Device{}}
}

// Load devices from JSON file
func (s *DeviceStore) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // no devices yet
		}
		return err
	}

	if err := json.Unmarshal(data, &s.devices); err != nil {
		return err
	}

	// Initialize runtime fields
	for _, dev := range s.devices {
		dev.Status = nil // GUI status will be assigned later
		dev.Online = false
		dev.BLEPtr = nil
	}

	return nil
}

// Save devices to JSON file (only persistent fields)
func (s *DeviceStore) Save() error {
	s.mu.Lock()
	devicesCopy := make([]*Device, len(s.devices))
	copy(devicesCopy, s.devices)
	s.mu.Unlock()

	data, err := json.MarshalIndent(devicesCopy, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.path, data, 0644)
}

// All returns a copy of all devices (thread-safe)
func (s *DeviceStore) All() []*Device {
	s.mu.Lock()
	defer s.mu.Unlock()
	copyDevices := make([]*Device, len(s.devices))
	copy(copyDevices, s.devices)
	return copyDevices
}

// Add a new device (ignores duplicates)
func (s *DeviceStore) Add(dev *Device) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, d := range s.devices {
		if d.ID == dev.ID {
			return // already exists
		}
	}
	s.devices = append(s.devices, dev)
}

// Remove deletes a device by ID
func (s *DeviceStore) Remove(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	newDevices := []*Device{}
	for _, d := range s.devices {
		if d.ID != id {
			newDevices = append(newDevices, d)
		} else {
			// Clean up runtime state
			if d.BLEPtr != nil {
				d.BLEPtr.Disconnect()
				d.BLEPtr = nil
			}
			d.Online = false
			d.Status = nil
		}
	}
	s.devices = newDevices
}

// Find returns a device by ID, nil if not found
func (s *DeviceStore) Find(id string) *Device {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, d := range s.devices {
		if d.ID == id {
			return d
		}
	}
	return nil
}

// Exists returns true if a device with given ID exists
func (s *DeviceStore) Exists(id string) bool {
	return s.findUnlocked(id) != nil
}

// FindByName returns a device by Name, nil if not found
func (s *DeviceStore) FindByName(name string) *Device {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, d := range s.devices {
		if d.Name == name {
			return d
		}
	}
	return nil
}

// Count returns the number of devices in the store
func (s *DeviceStore) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.devices)
}

// --- devicestore/device_runtime_helpers.go ---
func (s *DeviceStore) SetBLE(id string, ble *blemanager.BLEManager) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if dev := s.findUnlocked(id); dev != nil {
		dev.BLEPtr = ble
	}
}

func (s *DeviceStore) ClearBLE(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if dev := s.findUnlocked(id); dev != nil {
		dev.BLEPtr = nil
	}
}

func (s *DeviceStore) SetOnline(id string, online bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if dev := s.findUnlocked(id); dev != nil {
		dev.Online = online
	}
}

func (s *DeviceStore) IsEnabled(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if dev := s.findUnlocked(id); dev != nil {
		return dev.Enabled
	}
	return false
}

func (s *DeviceStore) findUnlocked(id string) *Device {
	for _, d := range s.devices {
		if d.ID == id {
			return d
		}
	}
	return nil
}

// Assigns a unique display name like "Device A", "Device B", etc.
func NextDeviceLetter(store *DeviceStore) string {
	for i := 0; i < 26; i++ { // A-Z
		id := fmt.Sprintf("%c", 'A'+i)
		if store.FindByName("Device "+id) == nil {
			return id
		}
	}
	// fallback if all letters used
	return fmt.Sprintf("%d", store.Count()+1)
}
