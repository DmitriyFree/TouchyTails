package main

import (
	"fmt"
	"time"

	"touchytails/blemanager"
	"touchytails/oscmanager"
)

func main() {
	// Channel for OSC touch events
	touchChan := make(chan float32, 10)

	// --- Start OSC manager ---
	oscMgr := oscmanager.New("127.0.0.1:9001", touchChan)
	go oscMgr.Run()

	// --- Start BLE manager ---
	ble := blemanager.New()
	ble.Connect("TouchyTails", 10*time.Second)
	defer ble.Disconnect()

	fmt.Println("System ready. Waiting for tail touches...")

	// --- Main loop: reflect tail touch state immediately ---
	currentState := false // tracks what we sent to the device

	for val := range touchChan {
		newState := val > 0
		if newState != currentState {
			currentState = newState
			if currentState {
				ble.Send("on")
				time.Sleep(100 * time.Millisecond)
				ble.Send("off")
			}
		}
	}
}
