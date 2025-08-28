// gui.go
package main

import (
	"image/color"
	"strings"

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

// --- UI building ---
func buildDeviceUI(d *Device, console *Console, refreshDevices func()) *fyne.Container {
	// Labels & Entries
	idLabel := widget.NewLabel(d.ID.String())
	nameEntry := widget.NewEntry()
	nameEntry.SetText(d.Name)
	eventEntry := widget.NewEntry()
	eventEntry.SetText(d.Event)
	statusLabel := d.Status

	// Handlers
	onBeep := func() {
		d.blePtr.Send("1")
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

	// Widgets
	beepBtn := widget.NewButton("Beep", onBeep)
	enabledCheck := widget.NewCheck("Enabled", onToggleEnabled)
	enabledCheck.SetChecked(d.Enabled)
	nameEntry.OnChanged = onNameChanged
	eventEntry.OnChanged = onEventChanged
	removeBtn := widget.NewButton("Remove", onRemove)

	// Layout
	row := container.NewGridWithColumns(7,
		idLabel, nameEntry, statusLabel, beepBtn, enabledCheck, eventEntry, removeBtn,
	)

	return container.NewBorder(nil, nil, nil, nil, row)
}

func refreshDevices(deviceList *fyne.Container, console *Console) {
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
	for _, d := range devices {
		deviceList.Add(buildDeviceUI(d, console, func() { refreshDevices(deviceList, console) }))
	}

	deviceList.Refresh()
}
