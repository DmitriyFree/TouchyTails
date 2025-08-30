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

func newStatus(text string) *canvas.Text {
	col := statusColors[text]
	txt := canvas.NewText(text, col)
	txt.TextSize = 14
	return txt
}

func applyStatus(label *canvas.Text, text string) {
	if label == nil {
		return
	}
	col, ok := statusColors[text]
	if !ok {
		col = statusColors["Pending"]
	}
	label.Text = text
	label.Alignment = fyne.TextAlignCenter
	label.Color = col
	fyne.DoAndWait(func() {
		label.Refresh()
	})
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
	fyne.DoAndWait(func() {
		c.widget.SetText(strings.Join(lines, "\n"))
	})
	c.widget.CursorRow = len(lines)
}

// --- UI building ---
func buildDeviceUI(d *devicestore.Device, console *Console, store *devicestore.DeviceStore, refreshDevices func()) *fyne.Container {
	// --- Labels & Entries ---
	idLabel := canvas.NewText(d.ID, color.White)
	idLabel.TextSize = 6
	idLabel.Alignment = fyne.TextAlignCenter
	idLabel.Resize(fyne.NewSize(50, 20))

	nameEntry := widget.NewEntry()
	nameEntry.SetText(d.Name)

	eventEntry := widget.NewEntry()
	eventEntry.SetText(d.Event)

	// Ensure runtime status
	if d.Status == nil {
		d.Status = canvas.NewText("Pending", color.White)
		d.Status.Alignment = fyne.TextAlignCenter
	}
	statusLabel := d.Status

	// --- Handlers ---
	onBeep := func() {
		if d.BLEPtr == nil || !d.Online {
			guiChan <- func() {
				console.append("Device offline, cannot beep: " + d.ID)
			}
			return
		}

		val := 0.4 + rand.Float64()*0.6 // random float in [0.4, 1.0]
		d.BLEPtr.Send(fmt.Sprintf("%.2f", val))
		guiChan <- func() {
			console.append(fmt.Sprintf("Beep: %.2f for %s", val, d.ID))
		}
	}

	onToggleEnabled := func(enabled bool) {
		d.Enabled = enabled
		if !d.Enabled {
			guiChan <- func() { applyStatus(d.Status, "Disabled") }
			if d.BLEPtr != nil {
				d.BLEPtr.Disconnect()
				d.BLEPtr = nil
			}
		} else if d.Online {
			guiChan <- func() { applyStatus(d.Status, "Online") }
		} else {
			guiChan <- func() { applyStatus(d.Status, "Pending") }
		}

		store.Save()
		guiChan <- func() {
			console.append(d.Status.Text + " for " + d.ID)
		}
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
		guiChan <- func() { console.append("Name updated for " + d.ID) }
	}

	onEventChanged := func(newEvent string) {
		d.Event = newEvent
		store.Save()
		guiChan <- func() { console.append("Event updated for " + d.ID) }
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

func refreshDevices(deviceList *fyne.Container, console *Console, store *devicestore.DeviceStore) {
	deviceList.Objects = nil

	// Header row
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
		// Ensure runtime fields are initialized
		if d.Status == nil {
			d.Status = newStatus("Pending")
		}

		deviceList.Add(buildDeviceUI(d, console, store, func() { refreshDevices(deviceList, console, store) }))
	}

	// Refresh UI safely on main thread
	fyne.Do(func() {
		deviceList.Refresh()
	})
}
