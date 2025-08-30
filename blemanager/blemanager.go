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
		fmt.Println("BLE:", err)
	}
	return &BLEManager{}
}

func (b *BLEManager) ScanDevice(
	deviceName string,
	timeout time.Duration,
	onEvent func(msg string), // <-- new callback
	onFound func(addr string),
) {
	go func() {
		onEvent("Starting scan for " + deviceName + "...")

		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		// goroutine to stop after timeout
		go func() {
			<-ctx.Done()
			onEvent("Scanning done (timeout)")
			adapter.StopScan()
		}()

		// This blocks until StopScan is called
		err := adapter.Scan(func(adapter *bluetooth.Adapter, result bluetooth.ScanResult) {
			onEvent("Found: " + result.LocalName() + " [" + result.Address.String() + "]")
			if result.LocalName() == deviceName {
				onEvent("Found target: " + result.LocalName() + " [" + result.Address.String() + "]")
				onFound(result.Address.String())
			}
		})
		if err != nil {
			onEvent("Failed to start scan:" + err.Error())
			return
		}
		onEvent("Scan finished")
	}()
}

// Connect scans and connects to the named device
// ConnectDevice connects to a specific device by its Bluetooth address.
func (b *BLEManager) ConnectDevice(addr string) error {
	fmt.Println("Connecting to device at", addr)

	var address bluetooth.Address
	address.Set(addr)

	device, err := adapter.Connect(address, bluetooth.ConnectionParams{})
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

func (b *BLEManager) Send(data string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.ready || b.char == nil {
		log.Println("BLE device not ready, skipping send:", data)
		return
	}

	done := make(chan error, 1)

	// Start write in a separate goroutine
	go func() {
		_, err := b.char.Write([]byte(data))
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			log.Println("Failed to send data:", err)
			b.ready = false
		} else {
			//fmt.Println("Sent:", data)
		}
	case <-time.After(1 * time.Second):
		log.Println("Send timeout:", data)
		b.ready = false
	}
}

// Disconnect safely disconnects from the device
func (b *BLEManager) Disconnect() {
	if b == nil {
		return // nothing to do
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.ready {
		b.device.Disconnect()
		b.ready = false
	}
}
func (b *BLEManager) Ready() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.ready
}
