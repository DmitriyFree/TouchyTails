package main

import (
	"encoding/json"
	"fmt"
	"image/color"
	"os"
	"strings"
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
}

// deviceData is used only for JSON persistence
type deviceData struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
	Event   string `json:"event"`
}

var filename = "devices.json"

// --- Status handling ---
var statusColors = map[string]color.RGBA{
	"Online":      {0, 200, 0, 255},
	"Offline":     {200, 0, 0, 255},
	"Malfunction": {200, 100, 0, 255},
	"Disabled":    {150, 150, 150, 255},
	"Pending":     {200, 200, 200, 255},
}

func newStatus(text string) *canvas.Text {
	col := statusColors[text]
	txt := canvas.NewText(text, col)
	txt.TextSize = 14
	return txt
}

func applyStatus(label *canvas.Text, text string) {
	col, ok := statusColors[text]
	if !ok {
		col = statusColors["Pending"]
	}
	label.Text = text
	label.Color = col
	label.Refresh()
}

// --- Console handling ---
type Console struct {
	widget *widget.Entry
	limit  int
}

func newConsole(limit int) *Console {
	c := &Console{
		widget: widget.NewMultiLineEntry(),
		limit:  limit,
	}
	c.widget.SetPlaceHolder("Console output...")
	c.widget.Wrapping = fyne.TextWrapWord
	c.widget.Disable()
	return c
}

func (c *Console) append(line string) {
	lines := strings.Split(c.widget.Text, "\n")
	lines = append(lines, line)

	// enforce max lines
	if len(lines) > c.limit {
		lines = lines[len(lines)-c.limit:]
	}
	c.widget.SetText(strings.Join(lines, "\n"))
	c.widget.CursorRow = len(lines)
}

var devices = []*Device{}

// --- UI building ---
func buildDeviceUI(d *Device, console *Console, refreshDevices func()) *fyne.Container {
	// --- Labels & Entries ---
	idLabel := widget.NewLabel(d.ID.String())

	nameEntry := widget.NewEntry()
	nameEntry.SetText(d.Name)

	eventEntry := widget.NewEntry()
	eventEntry.SetText(d.Event)

	statusLabel := d.Status

	// --- Handlers ---
	onBeep := func() {
		console.append("Beep sent to " + d.ID.String())
	}

	onToggleEnabled := func(enabled bool) {
		d.Enabled = enabled
		switch {
		case !d.Enabled:
			applyStatus(d.Status, "Disabled")
		case d.Online:
			applyStatus(d.Status, "Online")
		default:
			applyStatus(d.Status, "Pending")
		}
		SaveDevices(devices)
		console.append(d.Status.Text + " for " + d.ID.String())
	}

	onRemove := func() {
		newDevices := []*Device{}
		for _, dev := range devices {
			if dev.ID != d.ID {
				newDevices = append(newDevices, dev)
			}
		}
		devices = newDevices
		SaveDevices(devices)
		refreshDevices()
	}

	onNameChanged := func(newName string) {
		d.Name = newName
		SaveDevices(devices)
		console.append("Name updated for " + d.ID.String())
	}

	onEventChanged := func(newEvent string) {
		d.Event = newEvent
		SaveDevices(devices)
		console.append("Event updated for " + d.ID.String())
	}

	// --- Widgets ---
	beepBtn := widget.NewButton("Beep", onBeep)

	enabledCheck := widget.NewCheck("Enabled", onToggleEnabled)
	enabledCheck.SetChecked(d.Enabled)

	nameEntry.OnChanged = onNameChanged
	eventEntry.OnChanged = onEventChanged

	removeBtn := widget.NewButton("Remove", onRemove)

	// --- Layout ---
	row := container.NewGridWithColumns(7,
		idLabel, nameEntry, statusLabel, beepBtn, enabledCheck, eventEntry, removeBtn,
	)

	// Border ensures full width in VBox
	return container.NewBorder(nil, nil, nil, nil, row)
}

func refreshDevices(deviceList *fyne.Container, console *Console) {
	deviceList.Objects = nil

	// Add legend/header row
	header := container.NewGridWithColumns(7,
		widget.NewLabelWithStyle("ID", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewLabelWithStyle("Name", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewLabelWithStyle("Status", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewLabelWithStyle("Beep", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewLabelWithStyle("Enabled", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewLabelWithStyle("Event", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewLabelWithStyle("Remove", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
	)

	deviceList.Add(header)

	// Add device rows
	for _, d := range devices {
		deviceList.Add(buildDeviceUI(d, console, func() { refreshDevices(deviceList, console) }))
	}

	deviceList.Refresh()
}

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

// SaveDevices saves only the fields we care about to JSON
func SaveDevices(devices []*Device) error {
	dataToSave := make([]deviceData, len(devices))
	for i, d := range devices {
		dataToSave[i] = deviceData{
			ID:      d.ID.String(), // convert Address -> string
			Name:    d.Name,
			Enabled: d.Enabled,
			Event:   d.Event,
		}
	}

	data, err := json.MarshalIndent(dataToSave, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filename, data, 0644)
}

// LoadDevices loads JSON into Device structs and initializes Status
func LoadDevices() []*Device {
	raw, err := os.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return []*Device{}
		}
		return nil
	}

	var data []deviceData
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil
	}

	devices := make([]*Device, len(data))
	for i, d := range data {

		var deviceAddr bluetooth.Address
		deviceAddr.Set(d.ID)

		devices[i] = &Device{
			ID:      deviceAddr,
			Name:    d.Name,
			Enabled: d.Enabled,
			Event:   d.Event,
			Status:  newStatus("Pending"),
			Online:  false,
			blePtr:  nil,
		}
	}

	return devices
}

// Background task that keeps BLE connected
// RunBLEManagers starts BLE loops for all enabled devices
func RunBLEManagers(console *Console) {
	for _, d := range devices {
		if !d.Enabled || d.blePtr != nil {
			continue // skip disabled or already managed devices
		}

		// Create a BLE manager per device
		ble := blemanager.New()
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

				// Heartbeat loop
				for {
					if !dev.Enabled || !ble.Ready() {
						console.append(fmt.Sprintf("%s disconnected, reconnecting...", dev.Name))
						d.blePtr.Disconnect()
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
		err := ble.ScanDevice("TouchyTails", 10*time.Second, func(addrStr string) {
			console.append("Found device: " + addrStr)

			var addr bluetooth.Address
			if err := addr.Set(addrStr); err != nil {
				console.append("Invalid address: " + err.Error())
				return
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
		if err != nil {
			console.append("Scan error: " + err.Error())
		}
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
