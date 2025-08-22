package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"tinygo.org/x/bluetooth"
)

var adapter = bluetooth.DefaultAdapter

const (
	deviceName         = "TouchyTails"
	serviceUUID        = "0000ab00-0000-1000-8000-00805f9b34fb"
	characteristicUUID = "0000ab01-0000-1000-8000-00805f9b34fb"
)

func main() {
	// Enable BLE adapter
	if err := adapter.Enable(); err != nil {
		log.Fatalf("Enable failed: %v", err)
	}

	fmt.Println("Scanning for device:", deviceName)

	var device bluetooth.Device
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Scan for devices
	err := adapter.Scan(func(adapter *bluetooth.Adapter, result bluetooth.ScanResult) {
		if result.LocalName() == deviceName {
			fmt.Println("Found device:", result.Address.String(), result.LocalName())
			adapter.StopScan()
			// Try connect with its own timeout
			go func() {
				fmt.Println("Attempting to connect...")
				connectCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				done := make(chan struct{})
				var err error
				go func() {
					device, err = adapter.Connect(result.Address, bluetooth.ConnectionParams{})
					close(done)
				}()
				select {
				case <-done:
					if err != nil {
						fmt.Println("Connect error:", err)
					} else {
						fmt.Println("Connected!")
					}
				case <-connectCtx.Done():
					fmt.Println("Connect timeout")
				}
			}()
			cancel()
		}
	})
	if err != nil {
		log.Fatalf("Scan failed: %v", err)
	}

	<-ctx.Done()
	if (device == bluetooth.Device{}) {
		log.Fatal("Device not found or failed to connect")
	}
	defer device.Disconnect()

	// Discover service
	srvUUID, _ := bluetooth.ParseUUID(serviceUUID)
	services, err := device.DiscoverServices([]bluetooth.UUID{srvUUID})
	if err != nil || len(services) == 0 {
		log.Fatalf("Service not found: %v", err)
	}

	// Discover characteristic
	charUUID, _ := bluetooth.ParseUUID(characteristicUUID)
	chars, err := services[0].DiscoverCharacteristics([]bluetooth.UUID{charUUID})
	if err != nil || len(chars) == 0 {
		log.Fatalf("Characteristic not found: %v", err)
	}

	char := chars[0]

	// Example: turn LED ON
	n, err := char.WriteWithoutResponse([]byte("on"))
	if err != nil {
		log.Fatalf("Write failed: %v", err)
	}
	fmt.Printf("Wrote 'on' (%d bytes)\n", n)

	time.Sleep(2 * time.Second)

	// Example: turn LED OFF
	n, err = char.WriteWithoutResponse([]byte("off"))
	if err != nil {
		log.Fatalf("Write failed: %v", err)
	}
	fmt.Printf("Wrote 'off' (%d bytes)\n", n)
}
