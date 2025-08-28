package main

import (
	"encoding/json"
	"os"

	"tinygo.org/x/bluetooth"
)

var filename = "devices.json"

// deviceData is used only for JSON persistence
type deviceData struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
	Event   string `json:"event"`
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
			Conn:    nil,
		}
	}

	return devices
}
