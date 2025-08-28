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

// In your BLE manager:
/*func (b *BLEManager) ScanDevice(deviceName string, timeout time.Duration, onFound func(addr string)) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	return adapter.Scan(func(adapter *bluetooth.Adapter, result bluetooth.ScanResult) {
		if result.LocalName() == deviceName {
			adapter.StopScan()
			onFound(result.Address.String()) // call the callback with MAC string
		}
	})
}*/

func (b *BLEManager) ScanDevice(deviceName string, timeout time.Duration, onFound func(addr string)) (bluetooth.Address, error) {
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
				deviceAddress = result.Address
				fmt.Println("Found", deviceName, "at", deviceAddress, "! Stopping scan...")
				adapter.StopScan()
				close(found)
			})
		}
	})
	if err != nil {
		return bluetooth.Address{}, fmt.Errorf("failed to start scan: %w", err)
	}

	select {
	case <-found:
		onFound(deviceAddress.String())
		return deviceAddress, nil
	case <-ctx.Done():
		return bluetooth.Address{}, fmt.Errorf("scan timeout: device not found")
	}
}

// Connect scans and connects to the named device
// ConnectDevice connects to a specific device by its Bluetooth address.
func (b *BLEManager) ConnectDevice(addr bluetooth.Address) error {
	fmt.Println("Connecting to device at", addr)
	device, err := adapter.Connect(addr, bluetooth.ConnectionParams{})
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	services, err := device.DiscoverServices(nil)
	if err != nil {
		return fmt.Errorf("failed to discover services: %w", err)
	}

	var targetService *bluetooth.DeviceService
	for _, s := range services {
		if s.UUID().String() == serviceUUIDStr {
			targetService = &s
			break
		}
	}
	if targetService == nil {
		return fmt.Errorf("service not found")
	}

	chars, err := targetService.DiscoverCharacteristics(nil)
	if err != nil {
		return fmt.Errorf("failed to discover characteristics: %w", err)
	}

	var targetChar *bluetooth.DeviceCharacteristic
	for _, c := range chars {
		if c.UUID().String() == characteristicUUIDStr {
			targetChar = &c
			break
		}
	}
	if targetChar == nil {
		return fmt.Errorf("characteristic not found")
	}

	b.mu.Lock()
	b.device = device
	b.char = targetChar
	b.ready = true
	b.mu.Unlock()

	fmt.Println("Connected and ready to send data to", addr)
	return nil
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
		b.ready = false // mark as disconnected
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
func (b *BLEManager) Ready() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.ready
}
