package main

import (
	_ "embed"
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

//go:embed icon.png
var iconData []byte

var guiChan = make(chan func(), 50)
var oscChan = make(chan oscmanager.OSCMessage, 1)
var store = devicestore.New("devices.json")

func main() {
	a := app.New()
	w := a.NewWindow("Touchy Tails")
	setupIcons(a, w)

	console := newConsole(100)
	deviceListVBox := container.NewVBox()
	discoverBtn := widget.NewButton("Discover Devices", func() {
		postGUI(func() { console.append("Discovery triggered") })
		go bleScan(console, deviceListVBox)
	})
	setupGUI(w, console, deviceListVBox, discoverBtn)

	loadDevices(console, deviceListVBox)
	startRuntimeManagers(console)

	w.ShowAndRun()
}

// ------------------- Initialization Helpers -------------------

func setupIcons(a fyne.App, w fyne.Window) {
	iconRes := fyne.NewStaticResource("icon.png", iconData)
	a.SetIcon(iconRes)
	w.SetIcon(iconRes)
}

func setupGUI(w fyne.Window, console *Console, deviceListVBox *fyne.Container, discoverBtn *widget.Button) {
	consoleScroll := container.NewVScroll(console.widget)
	consoleScroll.SetMinSize(fyne.NewSize(0, 200))

	deviceListScroll := container.NewVScroll(deviceListVBox)
	deviceListScroll.SetMinSize(fyne.NewSize(0, 300))

	buttonBox := container.NewHBox(discoverBtn)

	mainUI := container.NewVBox(
		deviceListScroll,
		buttonBox, // <- add the button here
		consoleScroll,
	)
	w.SetContent(mainUI)
	w.Resize(fyne.NewSize(800, 550))
}

// ------------------- Device Loading -------------------

func loadDevices(console *Console, deviceListVBox *fyne.Container) {
	if err := store.Load(); err != nil {
		postGUI(func() { console.append("Failed to load devices: " + err.Error()) })
	}

	// Normalize IDs (migration)
	for _, dev := range store.All() {
		dev.Status = newStatus("Pending")
		dev.Status.Alignment = fyne.TextAlignCenter
	}
	store.Save()

	postGUI(func() {
		console.append(fmt.Sprintf("Loaded %d devices", len(store.All())))
	})
	refreshDevices(deviceListVBox, console, store)
}

// ------------------- BLE Discovery -------------------

func bleScan(console *Console, deviceListVBox *fyne.Container) {
	ble := blemanager.New()
	ble.ScanDevice("TouchyTails", 5*time.Second,
		func(msg string) { postGUI(func() { console.append(msg) }) },
		func(addrStr string) { addDeviceFromBLE(console, deviceListVBox, addrStr) },
	)
}

func addDeviceFromBLE(console *Console, deviceListVBox *fyne.Container, addrStr string) {
	postGUI(func() { console.append("Found device: " + addrStr) })

	var addr bluetooth.Address
	addr.Set(addrStr)

	if store.Exists(addrStr) {
		postGUI(func() { console.append("Device already exists, skipping: " + addrStr) })
		return
	}

	letter := devicestore.NextDeviceLetter(store)
	dev := &devicestore.Device{
		ID:      addrStr,
		Name:    "Device " + letter,
		Enabled: true,
		Status:  newStatus("Pending"),
	}
	store.Add(dev)
	store.Save()
	refreshDevices(deviceListVBox, console, store)
}

// ------------------- Runtime Managers -------------------

func startRuntimeManagers(console *Console) {
	// BLE runtime manager
	runtimeMgr := devicestore.NewRuntimeManager(console)
	runtimeMgr.Run(store)

	// OSC manager
	oscMgr := oscmanager.New("127.0.0.1:9001", oscChan)
	go oscMgr.Run(func(msg string) {
		console.append(msg)
	})

	// OSC processor
	go processOSC(console)

	// GUI updater
	go func() {
		for job := range guiChan {
			job()
		}
	}()
}

// ------------------- OSC Handling -------------------

func processOSC(console *Console) {
	for msg := range oscChan {
		if msg.Value <= 0 {
			continue
		}
		valueStr := fmt.Sprintf("%.2f", mapOSCValue(msg.Value))

		for _, dev := range store.All() {
			if !dev.Enabled || !dev.Online || dev.Event != msg.Name || dev.BLEPtr == nil {
				continue
			}
			dev.BLEPtr.Send(valueStr)
			postGUI(func() {
				console.append(fmt.Sprintf("%s: %s -> %s", dev.Name, dev.Event, valueStr))
			})
		}
	}
}

func mapOSCValue(val float32) float32 {
	mapped := 0.4 + val*0.6
	if mapped < 0.4 {
		mapped = 0.4
	}
	return mapped
}
