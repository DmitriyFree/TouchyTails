package main

import (
	_ "embed"
	"fmt"
	"image/color"
	"log"
	"os"
	"time"
	"touchytails/blemanager"
	"touchytails/devicestore"
	"touchytails/oscmanager"

	"gioui.org/app"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

type GUIState struct {
	window      *app.Window
	console     *Console
	deviceUIs   []*DeviceUI
	discoverBtn widget.Clickable
	consoleList widget.List
	store       *devicestore.DeviceStore
}

func main() {
	go func() {
		window := new(app.Window)
		window.Option(app.Title("Touchy Tails"))

		gui := &GUIState{
			window:  window,
			console: newConsole(200),
			store:   devicestore.New("devices.json"),
		}

		if err := run(window, gui); err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}()
	app.Main()
}

func run(w *app.Window, gui *GUIState) error {
	th := material.NewTheme()

	gui.console.append("Running initial scan")
	gui.window.Invalidate()

	// Start background processes safely
	go gui.bleScan()
	go gui.loadDevices()
	go startRuntimeManagers(gui)

	var ops op.Ops
	gui.consoleList.Axis = layout.Vertical

	for {
		switch e := w.Event().(type) {
		case app.DestroyEvent:
			return e.Err
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			// fill background first
			paint.Fill(gtx.Ops, color.NRGBA{R: 20, G: 20, B: 20, A: 255}) // dark gray
			drawUI(gtx, th, gui)
			e.Frame(gtx.Ops)
		}
	}
}

func drawUI(gtx layout.Context, th *material.Theme, gui *GUIState) layout.Dimensions {
	inset := layout.UniformInset(unit.Dp(8))

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		// Table header
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
				layout.Flexed(0.2, centeredCaption(th, "ID", color.NRGBA{200, 220, 255, 255})),
				layout.Flexed(0.15, centeredCaption(th, "Name", color.NRGBA{200, 220, 255, 255})),
				layout.Flexed(0.15, centeredCaption(th, "Status", color.NRGBA{200, 220, 255, 255})),
				layout.Flexed(0.2, centeredCaption(th, "Enabled", color.NRGBA{200, 220, 255, 255})),
				layout.Flexed(0.15, centeredCaption(th, "Beep", color.NRGBA{200, 220, 255, 255})),
				layout.Flexed(0.15, centeredCaption(th, "Remove", color.NRGBA{200, 220, 255, 255})),
			)
		}),

		// Device rows
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, gui.deviceList(gtx, th)...)
		}),

		// Spacer (pushes next widgets to bottom)
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: gtx.Constraints.Min}
		}),

		// Discover button
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			btn := material.Button(th, &gui.discoverBtn, "Discover Devices")
			if gui.discoverBtn.Clicked(gtx) {
				gui.console.append("Discovery triggered")
				go gui.bleScan()
			}
			return inset.Layout(gtx, btn.Layout)
		}),

		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			const consoleHeight = 200 // height in dp
			margin := unit.Dp(8)      // margin around console

			// Force fixed height
			gtx.Constraints.Min.Y = gtx.Dp(consoleHeight)
			gtx.Constraints.Max.Y = gtx.Dp(consoleHeight)

			// Add margin
			inset := layout.UniformInset(margin)
			return inset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				// Draw scrollable console lines
				return material.List(th, &gui.consoleList).Layout(gtx, len(gui.console.lines), func(gtx layout.Context, i int) layout.Dimensions {
					lbl := material.Body2(th, gui.console.lines[i])
					lbl.Color = color.NRGBA{200, 200, 200, 255}
					lbl.TextSize = unit.Sp(12)
					return lbl.Layout(gtx)
				})
			})
		}),
	)
}

func centeredCaption(th *material.Theme, text string, col color.NRGBA) func(gtx layout.Context) layout.Dimensions {
	return func(gtx layout.Context) layout.Dimensions {
		lbl := material.Caption(th, text)
		lbl.Color = col
		lbl.TextSize = unit.Sp(16)
		// center horizontally
		return layout.Center.Layout(gtx, lbl.Layout)
	}
}

func (gui *GUIState) deviceList(gtx layout.Context, th *material.Theme) []layout.FlexChild {
	children := make([]layout.FlexChild, 0, len(gui.deviceUIs))
	for _, du := range gui.deviceUIs {
		du := du // capture a new variable for this closure
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layoutDevice(gtx, th, du, gui)
		}))
	}
	return children
}

//go:embed icon.png
var iconData []byte
var oscChan = make(chan oscmanager.OSCMessage, 1)
var store = devicestore.New("devices.json")

// ------------------- Load Devices -------------------

func (gui *GUIState) loadDevices() {
	if err := gui.store.Load(); err != nil {
		gui.console.append("Failed to load devices: " + err.Error())
	}

	gui.deviceUIs = nil
	for _, d := range gui.store.All() {
		du := newDeviceUI(d)
		gui.deviceUIs = append(gui.deviceUIs, du)
	}
	gui.console.append(fmt.Sprintf("Loaded %d devices", len(gui.deviceUIs)))
}

// ------------------- BLE Discovery -------------------
func (gui *GUIState) bleScan() {
	ble := blemanager.New()
	ble.ScanDevice("TouchyTails", 5*time.Second,
		func(msg string) {
			gui.console.append(msg)
			gui.window.Invalidate()
		},
		func(addr string) {
			gui.console.append("Found " + addr)
			dev := &devicestore.Device{
				ID:      addr,
				Name:    "Device " + devicestore.NextDeviceLetter(gui.store),
				Enabled: true,
				Status:  "Pending",
			}
			gui.store.Add(dev)
			gui.store.Save()
			gui.loadDevices()
			gui.window.Invalidate()
		},
	)
}

/*func addDeviceFromBLE(gui *GUIState, addrStr string) {
	gui.console.append("Found device: " + addrStr)

	var addr bluetooth.Address
	addr.Set(addrStr)

	if store.Exists(addrStr) {
		gui.console.append("Device already exists, skipping: " + addrStr)
		return
	}

	letter := devicestore.NextDeviceLetter(store)
	dev := &devicestore.Device{
		ID:      addrStr,
		Name:    "Device " + letter,
		Enabled: true,
		Status:  "Pending",
	}
	store.Add(dev)
	store.Save()
}*/

// ------------------- Runtime Managers -------------------

func startRuntimeManagers(gui *GUIState) {
	// BLE runtime manager
	proxy := &GUIConsoleProxy{gui: gui}
	runtimeMgr := devicestore.NewRuntimeManager(proxy)
	runtimeMgr.Run(store)

	// OSC manager
	oscMgr := oscmanager.New("127.0.0.1:9001", oscChan)
	go oscMgr.Run(func(msg string) {
		gui.console.append(msg)
	})

	// OSC processor
	go processOSC(gui)
}

// ------------------- OSC Handling -------------------

func processOSC(gui *GUIState) {
	for msg := range oscChan {
		for _, dev := range store.All() {
			if !dev.Enabled || !dev.Online || dev.Event != msg.Name || dev.BLEPtr == nil {
				continue
			}

			// Skip if both old and new values are 0
			if msg.Value == 0 && dev.LastOSCVal == 0 {
				continue
			}

			// Remember last value
			dev.LastOSCVal = msg.Value

			// Map and format
			valueStr := fmt.Sprintf("%.2f", mapOSCValue(msg.Value))

			// Send
			dev.BLEPtr.Send(valueStr)
			gui.console.append(fmt.Sprintf("%s: %s -> %s", dev.Name, dev.Event, valueStr))
		}
	}
}

func mapOSCValue(val float32) float32 {
	mapped := 0.8 + val*0.2
	if mapped < 0.8 {
		mapped = 0.8
	}
	return mapped
}

type GUIConsoleProxy struct {
	gui *GUIState
}

func (p *GUIConsoleProxy) Log(msg string) {
	p.gui.console.append(msg)
	p.gui.window.Invalidate()
}

func (p *GUIConsoleProxy) SetStatus(dev *devicestore.Device, status string) {
	msg := fmt.Sprintf("%s → %s", dev.Name, status)
	p.gui.console.append(msg)
	p.gui.window.Invalidate()
}
