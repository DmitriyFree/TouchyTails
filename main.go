package main

import (
	"fmt"
	"strconv"
	"sync/atomic"
	"time"

	"touchytails/blemanager"
	"touchytails/oscmanager"
)

// global reference to active BLE (atomic for goroutines)
var blePtr atomic.Pointer[blemanager.BLEManager]

func main() {
	// Channel for OSC touch events
	touchChan := make(chan float32, 1)

	// --- Start OSC manager ---
	oscMgr := oscmanager.New("127.0.0.1:9001", touchChan)
	go oscMgr.Run()

	// --- Start BLE connection manager ---
	go bleConnectionManager()

	// Send a welcome beep
	//ble.Send("1")
	fmt.Println("System ready. Waiting for tail touches...")

	// --- Main loop: reflect tail touch state immediately ---
	for val := range touchChan {
		// Only send positive values
		if val > 0 {
			// Format float with 2 decimal places
			msg := strconv.FormatFloat(float64(val), 'f', 2, 32)
			ble := blePtr.Load() // safely grab current BLE connection
			if ble != nil {
				ble.Send(msg)
			}
		}
	}
}

// Background task that keeps BLE connected
func bleConnectionManager() {
	for {
		ble := blemanager.New()

		fmt.Println("Scanning/connecting to TouchyTails...")
		// Connect() blocks until found or times out (and may log.Fatal if fails)
		// We wrap in recover to avoid killing program
		func() {
			defer func() {
				if r := recover(); r != nil {
					fmt.Println("Recovering from BLE error:", r)
				}
			}()
			ble.Connect("TouchyTails", 10*time.Second)
		}()

		// If connected, store pointer
		blePtr.Store(ble)

		// Send a welcome beep
		ble.Send("1")

		// Stay "connected" until device disappears
		// Check every 2s if sending still works
		for {
			ble := blePtr.Load()
			if ble == nil || !ble.Ready() {
				fmt.Println("BLE disconnected. Reconnecting...")
				if ble != nil {
					ble.Disconnect()
					blePtr.Store(nil)
				}
				break // exit inner loop to retry Connect()
			}

			// send heartbeat
			time.Sleep(2 * time.Second)
			ble.Send("0")
		}

		// wait before retrying
		time.Sleep(5 * time.Second)
	}
}

// helper: check if BLEManager is ready
func isReady(b *blemanager.BLEManager) bool {
	return b.Ready()
}
