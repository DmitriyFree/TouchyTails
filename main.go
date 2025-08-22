package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"tinygo.org/x/bluetooth"
)

var adapter = bluetooth.DefaultAdapter

const (
	serviceUUIDStr        = "0000ab00-0000-1000-8000-00805f9b34fb"
	characteristicUUIDStr = "0000ab01-0000-1000-8000-00805f9b34fb"
)

func main() {
	// Enable the BLE adapter
	must("enable BLE stack", adapter.Enable())

	fmt.Println("Scanning for BLE devices...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var deviceAddress bluetooth.Address
	found := make(chan struct{})
	var once sync.Once

	// Start scanning
	err := adapter.Scan(func(adapter *bluetooth.Adapter, result bluetooth.ScanResult) {
		fmt.Printf("Found device: %s [%s]\n", result.Address.String(), result.LocalName())

		if result.LocalName() == "TouchyTails" {
			once.Do(func() {
				fmt.Println("Found TouchyTails device! Stopping scan...")
				deviceAddress = result.Address
				adapter.StopScan() // stop scanning immediately
				close(found)
			})
		}
	})
	must("start scan", err)

	select {
	case <-found:
		// proceed to connect
	case <-ctx.Done():
		fmt.Println("Scan timeout, device not found.")
		return
	}

	// Connect to the device
	device, err := adapter.Connect(deviceAddress, bluetooth.ConnectionParams{})
	must("connect to device", err)

	fmt.Println("Connected to TouchyTails!")

	// Discover services
	services, err := device.DiscoverServices(nil)
	must("discover services", err)

	var targetService *bluetooth.DeviceService
	for _, s := range services {
		if s.UUID().String() == serviceUUIDStr {
			targetService = &s
			break
		}
	}
	if targetService == nil {
		panic("service not found")
	}

	// Discover characteristics
	chars, err := targetService.DiscoverCharacteristics(nil)
	must("discover characteristics", err)

	var targetChar *bluetooth.DeviceCharacteristic
	for _, c := range chars {
		if c.UUID().String() == characteristicUUIDStr {
			targetChar = &c
			break
		}
	}
	if targetChar == nil {
		panic("characteristic not found")
	}

	fmt.Println("Found characteristic, writing data...")

	// Write "on" / "off" 5 times
	for i := 0; i < 5; i++ {
		_, err := targetChar.Write([]byte("on"))
		must("write on", err)
		fmt.Println("Sent: on")
		time.Sleep(500 * time.Millisecond)

		_, err = targetChar.Write([]byte("off"))
		must("write off", err)
		fmt.Println("Sent: off")
		time.Sleep(500 * time.Millisecond)
	}

	fmt.Println("Done. Device should have blinked 5 times.")

	// Disconnect
	device.Disconnect()
	fmt.Println("Disconnected.")
}

func must(action string, err error) {
	if err != nil {
		panic(fmt.Sprintf("failed to %s: %v", action, err))
	}
}
