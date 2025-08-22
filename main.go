package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"tinygo.org/x/bluetooth"
)

var adapter = bluetooth.DefaultAdapter

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

	// Keep connection alive for demo
	time.Sleep(5 * time.Second)

	// Disconnect
	device.Disconnect()
	fmt.Println("Disconnected.")
}

func must(action string, err error) {
	if err != nil {
		panic(fmt.Sprintf("failed to %s: %v", action, err))
	}
}
