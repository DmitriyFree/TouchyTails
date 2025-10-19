package main

import (
	"fmt"
	"image/color"
	"math/rand"
	"time"
	"touchytails/devicestore"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// --- Status handling ---
var statusColors = map[string]color.NRGBA{
	"Online":      {0, 200, 0, 255},
	"Offline":     {200, 0, 0, 255},
	"Malfunction": {200, 100, 0, 255},
	"Disabled":    {150, 150, 150, 255},
	"Pending":     {200, 200, 200, 255},
}

// Get status color
func getStatusColor(status string) color.NRGBA {
	col, ok := statusColors[status]
	if !ok {
		col = statusColors["Pending"]
	}
	return col
}

// --- Console handling ---
type Console struct {
	lines []string
	limit int
	List  widget.List // for scrolling
}

func newConsole(limit int) *Console {
	return &Console{
		lines: nil,
		limit: limit,
		List:  widget.List{List: layout.List{Axis: layout.Vertical}},
	}
}

func (c *Console) append(line string) {
	// Prepend timestamp
	timestamp := time.Now().Format("15:04:05") // HH:MM:SS
	line = fmt.Sprintf("[%s] %s", timestamp, line)

	// Add to top of slice (newest first)
	c.lines = append([]string{line}, c.lines...)
	if len(c.lines) > c.limit {
		c.lines = c.lines[:c.limit]
	}

	// Auto-scroll to bottom (newest last)
	c.List.Position.First = len(c.lines)
}

// --- Device UI helpers for Gio ---
type DeviceUI struct {
	idLabel     string
	name        string
	nameEditor  widget.Editor
	event       string
	eventEditor widget.Editor
	status      string
	enabled     bool
	beepBtn     widget.Clickable
	enabledBtn  widget.Bool
	removeBtn   widget.Clickable
	device      *devicestore.Device
}

func newDeviceUI(d *devicestore.Device) *DeviceUI {
	if d.Status == "" {
		d.Status = "Pending"
	}

	du := &DeviceUI{
		idLabel: d.ID,
		name:    d.Name,
		event:   d.Event,
		status:  d.Status,
		enabled: d.Enabled,
		device:  d,
	}

	// Initialize editor and checkbox
	du.nameEditor = widget.Editor{SingleLine: true}
	du.nameEditor.SetText(d.Name)

	du.eventEditor = widget.Editor{SingleLine: true}
	du.eventEditor.SetText(d.Event)

	du.enabledBtn = widget.Bool{Value: d.Enabled}

	return du
}

// --- BLE & UI actions ---
func (du *DeviceUI) onBeep(gui *GUIState) {
	if du.device.BLEPtr == nil || !du.device.Online {
		gui.console.append("Device offline, cannot beep: " + du.device.ID)
		return
	}
	val := mapOSCValue(rand.Float32())
	du.device.BLEPtr.Send(fmt.Sprintf("%.2f", val))
	gui.console.append(fmt.Sprintf("Beep: %.2f for %s", val, du.device.ID))
}

func (du *DeviceUI) onToggleEnabled(enabled bool, gui *GUIState) {
	du.device.Enabled = enabled
	gui.store.Save()
	if !enabled {
		if du.device.BLEPtr != nil {
			du.device.BLEPtr.Disconnect()
			du.device.BLEPtr = nil
		}
		du.status = "Disabled"
	} else if du.device.Online {
		du.status = "Online"
	} else {
		du.status = "Pending"
	}
	gui.console.append(fmt.Sprintf("%s for %s", du.status, du.device.ID))
}

func (du *DeviceUI) onRemove(gui *GUIState, store *devicestore.DeviceStore) {
	du.device.Enabled = false
	if du.device.BLEPtr != nil {
		du.device.BLEPtr.Disconnect()
		du.device.BLEPtr = nil
	}
	store.Remove(du.device.ID) // remove from the store
	gui.refreshDevices()       // refresh only from the same store
	gui.console.append(fmt.Sprintf("Removed device %s", du.device.ID))
}
func (gui *GUIState) refreshDevices() {
	gui.deviceUIs = nil
	for _, d := range gui.store.All() { // use gui.store, not some other store
		du := newDeviceUI(d)
		gui.deviceUIs = append(gui.deviceUIs, du)
	}
}

// --- Build device UI frame ---
func layoutDevice(gtx layout.Context, th *material.Theme, du *DeviceUI, gui *GUIState) layout.Dimensions {

	inset := layout.UniformInset(unit.Dp(4))

	// Handle button clicks
	if du.beepBtn.Clicked(gtx) {
		du.onBeep(gui)
	}
	if du.removeBtn.Clicked(gtx) {
		du.onRemove(gui, gui.store)
	}

	return inset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			// ID column
			layout.Flexed(0.2, func(gtx layout.Context) layout.Dimensions {
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					lbl := material.Body1(th, du.idLabel)
					lbl.Color = color.NRGBA{255, 255, 255, 255}
					lbl.TextSize = unit.Sp(8)
					return lbl.Layout(gtx)
				})
			}),
			// Name editor
			layout.Flexed(0.15, func(gtx layout.Context) layout.Dimensions {
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					ed := material.Editor(th, &du.nameEditor, "")
					ed.Color = color.NRGBA{255, 255, 255, 255}
					d := ed.Layout(gtx)

					// Update device name ONLY if changed
					newName := du.nameEditor.Text()
					if newName != du.device.Name {
						du.device.Name = newName
						gui.store.Save()
					}

					return d
				})
			}),

			// Status column
			layout.Flexed(0.15, func(gtx layout.Context) layout.Dimensions {
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					lbl := material.Body1(th, du.device.Status)
					lbl.Color = getStatusColor(du.device.Status)
					lbl.TextSize = unit.Sp(14)
					return lbl.Layout(gtx)
				})
			}),
			// Enabled checkbox
			layout.Flexed(0.10, func(gtx layout.Context) layout.Dimensions {
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					chk := material.CheckBox(th, &du.enabledBtn, "")
					chk.Color = color.NRGBA{255, 255, 255, 255}
					d := chk.Layout(gtx)
					if du.device.Enabled != du.enabledBtn.Value {
						du.onToggleEnabled(du.enabledBtn.Value, gui)
					}
					return d
				})
			}),
			layout.Flexed(0.15, func(gtx layout.Context) layout.Dimensions {
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					ed := material.Editor(th, &du.eventEditor, "Event name...")
					ed.Color = color.NRGBA{255, 255, 255, 255}
					ed.Hint = "-Event-"
					ed.HintColor = color.NRGBA{150, 150, 150, 255}

					// Add min width and optional background
					minW := gtx.Dp(unit.Dp(100))
					if gtx.Constraints.Min.X < minW {
						gtx.Constraints.Min.X = minW
					}
					paint.FillShape(gtx.Ops, color.NRGBA{R: 40, G: 40, B: 40, A: 255},
						clip.Rect{Max: gtx.Constraints.Min}.Op())

					d := ed.Layout(gtx)

					newEvent := du.eventEditor.Text()
					if newEvent != du.device.Event {
						du.device.Event = newEvent
						gui.store.Save()
					}

					return d
				})
			}),
			// Beep button
			layout.Flexed(0.10, func(gtx layout.Context) layout.Dimensions {
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return material.Button(th, &du.beepBtn, "Beep").Layout(gtx)
				})
			}),
			// Remove button
			layout.Flexed(0.15, func(gtx layout.Context) layout.Dimensions {
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return material.Button(th, &du.removeBtn, "Remove").Layout(gtx)
				})

			}),
		)
	})
}
