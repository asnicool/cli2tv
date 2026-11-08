package main

import (
	"encoding/json"
	"os"
	"path/filepath"

	"cli2tv/devices"
)

// deviceCachePath points at the persisted last-selected device. Tests
// override it; the default resolves to the user cache directory.
var deviceCachePath = defaultDeviceCachePath()

func defaultDeviceCachePath() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "cli2tv", "last-device.json")
}

// loadCachedDevice returns the device remembered from the last successful
// selection, or false when no usable cache exists.
func loadCachedDevice() (devices.Device, bool) {
	if deviceCachePath == "" {
		return devices.Device{}, false
	}
	data, err := os.ReadFile(deviceCachePath)
	if err != nil {
		return devices.Device{}, false
	}
	var dev devices.Device
	if err := json.Unmarshal(data, &dev); err != nil || dev.Addr == "" {
		return devices.Device{}, false
	}
	return dev, true
}

// saveCachedDevice persists the selected device for the next run. Failures
// are best-effort: caching is an optimization, not a requirement.
func saveCachedDevice(dev devices.Device) error {
	if deviceCachePath == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(deviceCachePath), 0700); err != nil {
		return err
	}
	data, err := json.Marshal(dev)
	if err != nil {
		return err
	}
	return os.WriteFile(deviceCachePath, data, 0600)
}
