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

	pingInterval = 2 * time.Second
	pingTimeout  = 3 * time.Second
	minWriteGap  = 20 * time.Millisecond // ~50 writes/s max
)

// BLEManager owns one device connection.
//
// Haptic values are coalesced: Send only stores the latest value and a single
// writer goroutine pushes it with write-without-response, so a burst of OSC
// messages can never queue up and stall the link. Liveness is checked with a
// "ping" written with response every pingInterval.
type BLEManager struct {
	mu        sync.Mutex
	device    bluetooth.Device
	char      bluetooth.DeviceCharacteristic
	connected bool // an OS-level GATT session exists and must be closed
	ready     bool

	pending    string
	hasPending bool
	kick       chan struct{}
	stop       chan struct{}
	stopOnce   sync.Once
}

// New creates a new BLEManager and enables the adapter
func New() *BLEManager {
	if err := adapter.Enable(); err != nil {
		fmt.Println("BLE:", err)
	}
	return &BLEManager{
		kick: make(chan struct{}, 1),
		stop: make(chan struct{}),
	}
}

func (b *BLEManager) ScanDevice(
	deviceName string,
	timeout time.Duration,
	onEvent func(msg string),
	onFound func(addr string),
) {
	go func() {
		onEvent("Starting scan for " + deviceName + "...")

		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		go func() {
			<-ctx.Done()
			onEvent("Scanning done (timeout)")
			adapter.StopScan()
		}()

		err := adapter.Scan(func(adapter *bluetooth.Adapter, result bluetooth.ScanResult) {
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

// ConnectDevice connects to a specific device by its Bluetooth address.
// On any failure the GATT session is closed, otherwise Windows keeps it alive
// (MaintainConnection) and later reconnects fail until the app restarts.
func (b *BLEManager) ConnectDevice(addr string) (err error) {
	fmt.Println("Connecting to device at", addr)

	var address bluetooth.Address
	address.Set(addr)

	device, err := adapter.Connect(address, bluetooth.ConnectionParams{})
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer func() {
		if err != nil {
			device.Disconnect()
		}
	}()

	services, err := device.DiscoverServices(nil)
	if err != nil {
		return fmt.Errorf("failed to discover services: %w", err)
	}

	var targetService *bluetooth.DeviceService
	for i := range services {
		if services[i].UUID().String() == serviceUUIDStr {
			targetService = &services[i]
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
	for i := range chars {
		if chars[i].UUID().String() == characteristicUUIDStr {
			targetChar = &chars[i]
			break
		}
	}
	if targetChar == nil {
		return fmt.Errorf("characteristic not found")
	}

	b.mu.Lock()
	b.device = device
	b.char = *targetChar
	b.connected = true
	b.ready = true
	b.mu.Unlock()

	go b.writer()

	fmt.Println("Connected and ready to send data to", addr)
	return nil
}

// Send queues the latest value; it never blocks.
func (b *BLEManager) Send(data string) {
	b.mu.Lock()
	if !b.ready {
		b.mu.Unlock()
		log.Println("BLE device not ready, skipping send:", data)
		return
	}
	b.pending = data
	b.hasPending = true
	b.mu.Unlock()

	select {
	case b.kick <- struct{}{}:
	default: // writer already signalled
	}
}

func (b *BLEManager) writer() {
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()
	lastWrite := time.Now()

	for {
		select {
		case <-b.stop:
			return

		case <-b.kick:
			if gap := time.Since(lastWrite); gap < minWriteGap {
				time.Sleep(minWriteGap - gap)
			}
			b.mu.Lock()
			data, ok, char := b.pending, b.hasPending, b.char
			b.hasPending = false
			b.mu.Unlock()
			if !ok {
				continue
			}
			if _, err := char.WriteWithoutResponse([]byte(data)); err != nil {
				b.fail("write: " + err.Error())
				return
			}
			lastWrite = time.Now()

		case <-ticker.C:
			// Write with response: the only reliable way to notice a dead link,
			// Windows does not report remote disconnects to the app.
			b.mu.Lock()
			char := b.char
			b.mu.Unlock()
			done := make(chan error, 1)
			go func() {
				_, err := char.Write([]byte("ping"))
				done <- err
			}()
			select {
			case err := <-done:
				if err != nil {
					b.fail("ping: " + err.Error())
					return
				}
			case <-time.After(pingTimeout):
				b.fail("ping timeout")
				return
			case <-b.stop:
				return
			}
			lastWrite = time.Now()
		}
	}
}

func (b *BLEManager) fail(reason string) {
	log.Println("BLE link lost:", reason)
	b.mu.Lock()
	b.ready = false
	b.mu.Unlock()
}

// Disconnect always closes the GATT session if one exists, even when the link
// is already marked as not ready.
func (b *BLEManager) Disconnect() {
	if b == nil {
		return
	}
	b.stopOnce.Do(func() { close(b.stop) })

	b.mu.Lock()
	defer b.mu.Unlock()
	if b.connected {
		if err := b.device.Disconnect(); err != nil {
			log.Println("BLE disconnect:", err)
		}
		b.device = bluetooth.Device{}
		b.connected = false
	}
	b.ready = false
	b.hasPending = false
}

func (b *BLEManager) Ready() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.ready
}
