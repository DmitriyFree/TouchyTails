package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/go-ble/ble"
	"github.com/go-ble/ble/examples/lib/dev"
)

func main() {
	// Initialize BLE device
	d, err := dev.NewDevice("default")
	if err != nil {
		log.Fatalf("Can't initialize device: %s", err)
	}
	ble.SetDefaultDevice(d)

	// Create a context with timeout
	ctx := ble.WithSigHandler(context.WithTimeout(context.Background(), 15*time.Second))

	fmt.Println("Scanning for device named 'TouchyTails'...")

	// Scan until we find our target device
	err = ble.Scan(ctx, false, func(a ble.Advertisement) {
		if a.LocalName() == "TouchyTails" {
			fmt.Printf("Found target device: %s [%s]\n", a.LocalName(), a.Addr())

			// Stop scanning by canceling the context
			ctx.Done()

			// Try connecting
			client, err := ble.Connect(context.Background(), func(a ble.Advertisement) bool {
				return a.LocalName() == "TouchyTails"
			})
			if err != nil {
				log.Fatalf("Failed to connect: %s", err)
			}
			defer client.CancelConnection()

			fmt.Println("Connected to TouchyTails!")

			// Discover services
			services, err := client.DiscoverServices(nil)
			if err != nil {
				log.Fatalf("Failed to discover services: %s", err)
			}

			for _, s := range services {
				fmt.Printf("Service: %s\n", s.UUID)
				chars, _ := client.DiscoverCharacteristics(nil, s)
				for _, c := range chars {
					fmt.Printf("  Characteristic: %s\n", c.UUID)
				}
			}
		}
	}, nil)

	if err != nil {
		log.Fatalf("Scan failed: %s", err)
	}
}
