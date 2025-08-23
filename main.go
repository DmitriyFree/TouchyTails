package main

import (
	"fmt"
	"strconv"
	"time"

	"touchytails/blemanager"
	"touchytails/oscmanager"
)

func main() {
	// Channel for OSC touch events
	touchChan := make(chan float32, 1)

	// --- Start OSC manager ---
	oscMgr := oscmanager.New("127.0.0.1:9001", touchChan)
	go oscMgr.Run()

	// --- Start BLE manager ---
	ble := blemanager.New()
	ble.Connect("TouchyTails", 10*time.Second)
	defer ble.Disconnect()

	// Send a welcome beep
	ble.Send("1")
	fmt.Println("System ready. Waiting for tail touches...")

	// --- Main loop: reflect tail touch state immediately ---
	for val := range touchChan {
		// Only send positive values
		if val > 0 {
			// Format float with 2 decimal places
			msg := strconv.FormatFloat(float64(val), 'f', 2, 32)
			ble.Send(msg)
		}
	}
}
