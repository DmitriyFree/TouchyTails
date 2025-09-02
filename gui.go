// gui.go
package main

import (
	"fmt"
	"image/color"
	"math/rand/v2"
	"strings"
	"touchytails/devicestore"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// --- Status handling ---
var statusColors = map[string]color.RGBA{
	"Online":      {0, 200, 0, 255},
	"Offline":     {200, 0, 0, 255},
	"Malfunction": {200, 100, 0, 255},
	"Disabled":    {150, 150, 150, 255},
	"Pending":     {200, 200, 200, 255},
}

// Creates a new status label
func newStatus(text string) *canvas.Text {
	col := statusColors[text]
	txt := canvas.NewText(text, col)
	txt.TextSize = 14
	txt.Alignment = fyne.TextAlignCenter
	return txt
}

// Updates status label safely
func applyStatus(label *canvas.Text, text string) {
	if label == nil {
		return
	}
	col, ok := statusColors[text]
	if !ok {
		col = statusColors["Pending"]
	}
	label.Text = text
	label.Color = col
	label.Alignment = fyne.TextAlignCenter
	label.Refresh()
}

// --- GUI posting helper ---
func postGUI(job func()) {
	guiChan <- func() { fyne.Do(job) }
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

// Append text safely to console
func (c *Console) append(line string) {
	lines := strings.Split(c.widget.Text, "\n")
	lines = append(lines, line)
	if len(lines) > c.limit {
		lines = lines[len(lines)-c.limit:]
	}
	c.widget.SetText(strings.Join(lines, "\n"))
	c.widget.CursorRow = len(lines)
}

// Public method to append via GUI safely
func (c *Console) Append(line string) {
	postGUI(func() { c.append(line) })
}

// Apply status change to device via GUI
func (c *Console) ApplyStatus(dev *devicestore.Device, status string) {
	postGUI(func() { applyStatus(dev.Status, status) })
}

// --- Device UI ---
func buildDeviceUI(d *devicestore.Device, console *Console, store *devicestore.DeviceStore, refreshDevices func()) *fyne.Container {
	// --- Labels & Entries ---
	if d.Status == nil {
		d.Status = newStatus("Pending")
	}
	idLabel := canvas.NewText(d.ID, color.White)
	idLabel.TextSize = 6
	idLabel.Alignment = fyne.TextAlignCenter
	idLabel.Resize(fyne.NewSize(50, 20))

	nameEntry := widget.NewEntry()
	nameEntry.SetText(d.Name)

	eventEntry := widget.NewEntry()
	eventEntry.SetText(d.Event)

	statusLabel := d.Status

	// --- Handlers ---
	onBeep := func() {
		if d.BLEPtr == nil || !d.Online {
			console.Append("Device offline, cannot beep: " + d.ID)
			return
		}
		val := 0.4 + rand.Float64()*0.6
		d.BLEPtr.Send(fmt.Sprintf("%.2f", val))
		console.Append(fmt.Sprintf("Beep: %.2f for %s", val, d.ID))
	}

	onToggleEnabled := func(enabled bool) {
		d.Enabled = enabled
		store.Save()
		postGUI(func() {
			if !d.Enabled {
				if d.BLEPtr != nil {
					d.BLEPtr.Disconnect()
					d.BLEPtr = nil
				}
				applyStatus(statusLabel, "Disabled")
			} else if d.Online {
				applyStatus(statusLabel, "Online")
			} else {
				applyStatus(statusLabel, "Pending")
			}
			console.Append(fmt.Sprintf("%s for %s", statusLabel.Text, d.ID))
		})
	}

	onRemove := func() {
		d.Enabled = false
		if d.BLEPtr != nil {
			d.BLEPtr.Disconnect()
			d.BLEPtr = nil
		}
		store.Remove(d.ID)
		refreshDevices()
	}

	onNameChanged := func(newName string) {
		d.Name = newName
		store.Save()
		console.Append("Name updated for " + d.ID)
	}

	onEventChanged := func(newEvent string) {
		d.Event = newEvent
		store.Save()
		console.Append("Event updated for " + d.ID)
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

	return container.NewBorder(nil, nil, nil, nil, row)
}

// Refresh the device list
func refreshDevices(deviceList *fyne.Container, console *Console, store *devicestore.DeviceStore) {
	postGUI(func() {
		deviceList.Objects = nil

		// Header
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

		// Device rows
		for _, d := range store.All() {
			if d.Status == nil {
				d.Status = newStatus("Pending")
			}
			deviceList.Add(buildDeviceUI(d, console, store, func() { refreshDevices(deviceList, console, store) }))
		}

		deviceList.Refresh()
	})
}
