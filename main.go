package main

import (
	"fmt"
	"time"
	"touchytails/blemanager"
	"touchytails/oscmanager"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"tinygo.org/x/bluetooth"
)

//var blePtr atomic.Pointer[blemanager.BLEManager]

type Device struct {
	ID      bluetooth.Address
	Name    string
	Enabled bool
	Online  bool
	Status  *canvas.Text
	Event   string
	blePtr  *blemanager.BLEManager
	Conn    *bluetooth.Device // <--- add this
}

var devices = []*Device{}

func nextDeviceID(devices []*Device) string {
	base := ""
	for i := 0; i < 26; i++ { // A-Z
		id := fmt.Sprintf("%s%c", base, 'A'+i)
		unique := true
		for _, d := range devices {
			if d.ID.String() == id { // check ID, not Name
				unique = false
				break
			}
		}
		if unique {
			return id
		}
	}
	// fallback if all letters used
	return fmt.Sprintf("%s%d", base, len(devices)+1)
}

// Background task that keeps BLE connected
// RunBLEManagers starts BLE loops for all enabled devices
func RunBLEManagers(console *Console) {
	for _, d := range devices {
		if !d.Enabled || d.blePtr != nil {
			continue // skip disabled or already managed devices
		}

		// Create a BLE manager per device
		var ble = blemanager.New()
		d.blePtr = ble

		go func(dev *Device) {
			for {
				if !dev.Enabled {
					time.Sleep(2 * time.Second)
					continue
				}

				console.append(fmt.Sprintf("Scanning/connecting to %s (%s)...", dev.Name, dev.ID))
				err := ble.ConnectDevice(dev.ID) // connect by stored MAC
				if err != nil {
					console.append(fmt.Sprintf("Failed to connect %s: %v", dev.Name, err))
					time.Sleep(5 * time.Second)
					continue
				}

				console.append(fmt.Sprintf("%s connected!", dev.Name))
				d.blePtr.Send("1") // welcome beep
				d.Online = true
				applyStatus(d.Status, "Online")

				// Heartbeat loop
				for {
					if !dev.Enabled || !ble.Ready() {
						console.append(fmt.Sprintf("%s disconnected, reconnecting...", dev.Name))
						d.blePtr.Disconnect()
						d.Online = false
						applyStatus(d.Status, "Offline")
						break // retry connect
					}

					ble.Send("0") // heartbeat
					time.Sleep(2 * time.Second)
				}
			}
		}(d)
	}
}

// --- Main ---
func main() {
	a := app.New()
	w := a.NewWindow("Device Manager")

	// --- Console ---
	console := newConsole(100)
	consoleScroll := container.NewVScroll(console.widget)

	// --- Device list ---
	deviceListVBox := container.NewVBox()
	deviceListScroll := container.NewVScroll(deviceListVBox)
	deviceListScroll.SetMinSize(fyne.NewSize(0, 300))
	consoleScroll.SetMinSize(fyne.NewSize(0, 200))

	// --- Load devices ---
	devices = LoadDevices()
	console.append(fmt.Sprintf("Loaded %d devices", len(devices)))
	refreshDevices(deviceListVBox, console)

	discoverBtn := widget.NewButton("Discover Devices", func() {
		console.append("Discovery triggered")
		var ble = blemanager.New()
		ble.ScanDevice("TouchyTails", 10*time.Second, func(addrStr string) {
			console.append("Found device: " + addrStr)

			var addr bluetooth.Address
			addr.Set(addrStr) // no error returned

			id := nextDeviceID(devices)
			dev := &Device{
				ID:      addr,
				Name:    "Device " + id,
				Enabled: true,
				Online:  false,
				Status:  newStatus("Pending"),
				Event:   "",
			}
			devices = append(devices, dev)
			SaveDevices(devices)
			refreshDevices(deviceListVBox, console)
		})
	})

	buttonBox := container.NewHBox(discoverBtn)

	// --- Main layout ---
	mainUI := container.NewVBox(
		deviceListScroll,
		buttonBox,
		widget.NewLabel("Console:"),
		consoleScroll,
	)
	w.SetContent(mainUI)
	w.Resize(fyne.NewSize(800, 600))

	// --- Channels for tail touch processing ---
	touchChan := make(chan float32, 10) // OSC tail touch values
	guiChan := make(chan string, 20)    // messages for GUI console

	// --- Safe GUI updater ---
	go func() {
		for msg := range guiChan {
			console.append(msg)
		}
	}()

	// --- Start BLE manager ---
	go RunBLEManagers(console)

	// --- Start OSC manager ---
	oscMgr := oscmanager.New("127.0.0.1:9001", touchChan)
	go oscMgr.Run()

	// --- Process tail touches ---
	/*go func() {
		for val := range touchChan {
			if val > 0 {
				msg := fmt.Sprintf("%.2f", val)
				ble := blePtr.Load()
				if ble != nil {
					ble.Send(msg)
				}
				guiChan <- "Tail touched: " + msg
			}
		}
	}()*/

	// --- Run GUI ---
	w.ShowAndRun()
}
