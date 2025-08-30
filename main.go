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

var guiChan = make(chan func(), 50)               // buffered
var oscChan = make(chan oscmanager.OSCMessage, 1) // OSC values

type Device struct {
	ID      bluetooth.Address
	Name    string
	Enabled bool
	Online  bool
	Status  *canvas.Text
	Event   string
	blePtr  *blemanager.BLEManager
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
	go func() {
		for {
			for _, d := range devices {
				// Skip disabled or already managed devices
				if !d.Enabled || d.blePtr != nil {
					continue
				}

				// Create a BLE manager per device
				ble := blemanager.New()
				d.blePtr = ble

				go func(dev *Device, ble *blemanager.BLEManager) {
					for {
						// if device got removed or disabled, stop goroutine
						if !dev.Enabled {
							if dev.blePtr != nil {
								dev.blePtr.Disconnect()
								dev.blePtr = nil
							}
							dev.Online = false
							guiChan <- func() {
								applyStatus(dev.Status, "Disabled")
							}
							return
						}

						guiChan <- func() {
							console.append(fmt.Sprintf("Scanning/connecting to %s (%s)...", dev.Name, dev.ID))
						}

						err := ble.ConnectDevice(dev.ID) // connect by stored MAC
						if err != nil {
							guiChan <- func() {
								console.append(fmt.Sprintf("Failed to connect %s: %v", dev.Name, err))
							}
							time.Sleep(5 * time.Second)
							continue
						}

						guiChan <- func() {
							console.append(fmt.Sprintf("%s connected!", dev.Name))
						}
						dev.Online = true
						guiChan <- func() {
							applyStatus(dev.Status, "Online")
						}

						// Heartbeat loop
						for {
							if !dev.Enabled || !ble.Ready() {
								guiChan <- func() {
									console.append(fmt.Sprintf("%s disconnected, reconnecting...", dev.Name))
								}
								ble.Disconnect()
								dev.Online = false
								dev.blePtr = nil
								guiChan <- func() {
									applyStatus(dev.Status, "Offline")
								}
								break // retry connect
							}

							ble.Send("ping") // heartbeat
							time.Sleep(2 * time.Second)
						}
					}
				}(d, ble)
			}

			time.Sleep(3 * time.Second) // poll device list again
		}
	}()
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
	guiChan <- func() {
		console.append(fmt.Sprintf("Loaded %d devices", len(devices)))
	}
	refreshDevices(deviceListVBox, console)

	discoverBtn := widget.NewButton("Discover Devices", func() {
		guiChan <- func() {
			console.append("Discovery triggered")
		}
		var ble = blemanager.New()
		ble.ScanDevice(
			"TouchyTails",
			5*time.Second,
			func(msg string) { //onmessage
				guiChan <- func() {
					console.append(msg)
				}
			},
			func(addrStr string) { // onfound
				guiChan <- func() {
					console.append("Found device: " + addrStr)
				}

				var addr bluetooth.Address
				addr.Set(addrStr) // no error returned

				// 🔍 Check if device already exists
				for _, d := range devices {
					if d.ID.String() == addr.String() {
						guiChan <- func() {
							console.append("Device already exists, skipping: " + addrStr)
						}
						return // 👈 ignore duplicate
					}
				}

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

	// --- Start BLE manager ---
	go RunBLEManagers(console)

	// --- Start OSC manager ---
	oscMgr := oscmanager.New("127.0.0.1:9001", oscChan)
	go oscMgr.Run()

	// --- Process tail touches ---
	go func() {
		for msg := range oscChan {
			// Only process positive values
			if msg.Value <= 0 {
				continue
			}

			// Map [0..1] → [0.4..1] // avoid too low values
			// to ensure minimum audible beep
			mapped := 0.4 + msg.Value*0.6
			if mapped < 0.4 {
				mapped = 0.4
			}

			valueStr := fmt.Sprintf("%.2f", msg.Value)

			for _, dev := range devices {
				// Skip disabled or offline devices
				if !dev.Enabled || !dev.Online {
					continue
				}

				// Check if this device listens to this OSC event
				if dev.Event != msg.Name {
					continue
				}

				// Send value via BLE if available
				if dev.blePtr != nil {
					dev.blePtr.Send(valueStr)
				}

				// Update console safely on GUI thread
				guiChan <- func(d *Device, val string) func() {
					return func() {
						console.append(fmt.Sprintf("Tail touched: %s -> %s", d.Name, val))
					}
				}(dev, valueStr) // pass dev and valueStr to closure to avoid closure capture issues
			}
		}
	}()

	// --- GUI update loop ---
	go func() {
		for job := range guiChan {
			// Run each GUI update safely on Fyne main thread
			job()
		}
	}()

	// --- Run GUI ---
	w.ShowAndRun()
}
