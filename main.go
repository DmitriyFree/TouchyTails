package main

import (
	"fmt"
	"time"
	"touchytails/blemanager"
	"touchytails/devicestore"
	"touchytails/oscmanager"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"tinygo.org/x/bluetooth"
)

var guiChan = make(chan func(), 50)               // buffered
var oscChan = make(chan oscmanager.OSCMessage, 1) // OSC values
var store = devicestore.New("devices.json")

func nextDeviceID(store *devicestore.DeviceStore) string {
	base := ""
	for i := 0; i < 26; i++ { // A-Z
		id := fmt.Sprintf("%s%c", base, 'A'+i)
		if store.FindByName("Device "+id) == nil { // check if name already exists
			return id
		}
	}
	// fallback if all letters used
	return fmt.Sprintf("%s%d", base, store.Count()+1)
}

// Background task that keeps BLE connected
// RunBLEManagers starts BLE loops for all enabled devices
func RunBLEManagers(store *devicestore.DeviceStore, console *Console) {
	go func() {
		for {
			for _, dev := range store.All() {
				// Skip disabled or already managed devices
				if !dev.Enabled || dev.BLEPtr != nil {
					continue
				}

				// Create a BLE manager per device
				ble := blemanager.New()
				dev.BLEPtr = ble

				go func(d *devicestore.Device, ble *blemanager.BLEManager) {
					for {
						// Stop goroutine if device disabled or removed
						if !d.Enabled {
							if d.BLEPtr != nil {
								d.BLEPtr.Disconnect()
								d.BLEPtr = nil
							}
							d.Online = false
							guiChan <- func() { applyStatus(d.Status, "Disabled") }
							return
						}

						guiChan <- func() {
							console.append(fmt.Sprintf("Scanning/connecting to %s (%s)...", d.Name, d.ID))
						}

						err := ble.ConnectDevice(d.ID)
						if err != nil {
							guiChan <- func() {
								console.append(fmt.Sprintf("Failed to connect %s: %v", d.Name, err))
							}
							time.Sleep(5 * time.Second)
							continue
						}

						guiChan <- func() { console.append(fmt.Sprintf("%s connected!", d.Name)) }
						d.Online = true
						guiChan <- func() { applyStatus(d.Status, "Online") }

						// Heartbeat loop
						for {
							if !d.Enabled || !ble.Ready() {
								guiChan <- func() {
									console.append(fmt.Sprintf("%s disconnected, reconnecting...", d.Name))
								}
								ble.Disconnect()
								d.Online = false
								d.BLEPtr = nil
								guiChan <- func() { applyStatus(d.Status, "Offline") }
								break
							}

							ble.Send("ping")
							time.Sleep(2 * time.Second)
						}
					}
				}(dev, ble)
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

	// --- Load devices using DeviceStore ---
	store := devicestore.New("devices.json")
	if err := store.Load(); err != nil {
		guiChan <- func() { console.append("Failed to load devices: " + err.Error()) }
	}

	// Initialize runtime fields
	for _, dev := range store.All() {
		dev.Status = newStatus("Pending")
		dev.Online = false
		dev.BLEPtr = nil
	}

	guiChan <- func() { console.append(fmt.Sprintf("Loaded %d devices", len(store.All()))) }
	refreshDevices(deviceListVBox, console, store)

	// --- Discover button ---
	discoverBtn := widget.NewButton("Discover Devices", func() {
		guiChan <- func() { console.append("Discovery triggered") }

		ble := blemanager.New()
		ble.ScanDevice(
			"TouchyTails",
			5*time.Second,
			func(msg string) { // onmessage
				guiChan <- func() { console.append(msg) }
			},
			func(addrStr string) { // onfound
				guiChan <- func() { console.append("Found device: " + addrStr) }

				var addr bluetooth.Address
				addr.Set(addrStr) // ignore error

				// Check if device already exists
				if store.Exists(addr.String()) {
					guiChan <- func() {
						console.append("Device already exists, skipping: " + addrStr)
					}
					return
				}

				// Add new device
				id := nextDeviceID(store)
				dev := &devicestore.Device{
					ID:      addr.String(),
					Name:    "Device " + id,
					Enabled: true,
					Event:   "",
					Online:  false,
					Status:  newStatus("Pending"),
					BLEPtr:  nil,
				}
				store.Add(dev)
				store.Save()
				refreshDevices(deviceListVBox, console, store)
			},
		)
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

	// --- Start BLE manager loop ---
	go RunBLEManagers(store, console)

	// --- Start OSC manager ---
	oscMgr := oscmanager.New("127.0.0.1:9001", oscChan)
	go oscMgr.Run()

	// --- Process OSC messages ---
	go func() {
		for msg := range oscChan {
			if msg.Value <= 0 {
				continue
			}

			// Map [0..1] → [0.4..1]
			mapped := 0.4 + msg.Value*0.6
			if mapped < 0.4 {
				mapped = 0.4
			}
			valueStr := fmt.Sprintf("%.2f", mapped)

			for _, dev := range store.All() {
				if !dev.Enabled || !dev.Online || dev.Event != msg.Name || dev.BLEPtr == nil {
					continue
				}

				dev.BLEPtr.Send(valueStr)

				// Update console safely
				guiChan <- func(d *devicestore.Device, val string) func() {
					return func() {
						console.append(fmt.Sprintf("%s: %s -> %s", d.Name, d.Event, val))
					}
				}(dev, valueStr)
			}
		}
	}()

	// --- GUI update loop ---
	go func() {
		for job := range guiChan {
			job()
		}
	}()

	// --- Run GUI ---
	w.ShowAndRun()
}
