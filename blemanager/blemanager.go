package blemanager

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"tinygo.org/x/bluetooth"
)

var adapter = bluetooth.DefaultAdapter

const (
	serviceUUIDStr        = "0000ab00-0000-1000-8000-00805f9b34fb"
	characteristicUUIDStr = "0000ab01-0000-1000-8000-00805f9b34fb"
)

// BLEManager encapsulates the BLE device connection
type BLEManager struct {
	device bluetooth.Device
	char   *bluetooth.DeviceCharacteristic
	ready  bool
	mu     sync.Mutex
}

// New creates a new BLEManager and enables the adapter
func New() *BLEManager {
	if err := adapter.Enable(); err != nil {
		log.Fatal("failed to enable BLE adapter:", err)
	}
	return &BLEManager{}
}

// Connect scans and connects to the named device
func (b *BLEManager) Connect(deviceName string, timeout time.Duration) {
	fmt.Println("Scanning for BLE devices...")
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var deviceAddress bluetooth.Address
	found := make(chan struct{})
	var once sync.Once

	err := adapter.Scan(func(adapter *bluetooth.Adapter, result bluetooth.ScanResult) {
		fmt.Printf("Found device: %s [%s]\n", result.Address.String(), result.LocalName())

		if result.LocalName() == deviceName {
			once.Do(func() {
				fmt.Println("Found", deviceName, "! Stopping scan...")
				deviceAddress = result.Address
				adapter.StopScan()
				close(found)
			})
		}
	})
	if err != nil {
		log.Fatal("failed to start scan:", err)
	}

	select {
	case <-found:
	case <-ctx.Done():
		log.Fatal("Scan timeout, device not found")
	}

	// Connect to device
	fmt.Println("Trying to connect")
	device, err := adapter.Connect(deviceAddress, bluetooth.ConnectionParams{})
	if err != nil {
		log.Fatal("failed to connect:", err)
	}

	services, err := device.DiscoverServices(nil)
	if err != nil {
		log.Fatal("failed to discover services:", err)
	}

	var targetService *bluetooth.DeviceService
	for _, s := range services {
		if s.UUID().String() == serviceUUIDStr {
			targetService = &s
			break
		}
	}
	if targetService == nil {
		log.Fatal("service not found")
	}

	chars, err := targetService.DiscoverCharacteristics(nil)
	if err != nil {
		log.Fatal("failed to discover characteristics:", err)
	}

	var targetChar *bluetooth.DeviceCharacteristic
	for _, c := range chars {
		if c.UUID().String() == characteristicUUIDStr {
			targetChar = &c
			break
		}
	}
	if targetChar == nil {
		log.Fatal("characteristic not found")
	}

	b.mu.Lock()
	b.device = device
	b.char = targetChar
	b.ready = true
	b.mu.Unlock()

	fmt.Println("Connected and ready to send data to", deviceName)
}

// Send writes data to the device safely
func (b *BLEManager) Send(data string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.ready || b.char == nil {
		log.Println("BLE device not ready, skipping send:", data)
		return
	}

	_, err := b.char.Write([]byte(data))
	if err != nil {
		log.Println("Failed to send data:", err)
	} else {
		fmt.Println("Sent:", data)
	}
}

// Disconnect safely disconnects from the device
func (b *BLEManager) Disconnect() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.ready {
		b.device.Disconnect()
		b.ready = false
		fmt.Println("Disconnected from BLE device")
	}
}
