package main

import (
	"os"
	"path/filepath"
	"testing"

	"cli2tv/devices"
)

func TestLoadSaveCachedDevice(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "last-device.json")
	deviceCachePath = path

	dev := devices.Device{Name: "Living Room TV", Addr: "http://192.168.1.75:1152/", Type: devices.DeviceTypeDLNA}

	if _, ok := loadCachedDevice(); ok {
		t.Fatal("loadCachedDevice() found a device before anything was saved")
	}

	if err := saveCachedDevice(dev); err != nil {
		t.Fatalf("saveCachedDevice() error = %v", err)
	}

	got, ok := loadCachedDevice()
	if !ok {
		t.Fatal("loadCachedDevice() did not find the saved device")
	}
	if got != dev {
		t.Fatalf("loadCachedDevice() = %+v, want %+v", got, dev)
	}
}

func TestLoadCachedDeviceMissing(t *testing.T) {
	deviceCachePath = filepath.Join(t.TempDir(), "last-device.json")

	if _, ok := loadCachedDevice(); ok {
		t.Fatal("loadCachedDevice() found a device in a missing file")
	}
}

func TestLoadCachedDeviceCorrupt(t *testing.T) {
	deviceCachePath = filepath.Join(t.TempDir(), "last-device.json")
	if err := os.WriteFile(deviceCachePath, []byte("not json"), 0600); err != nil {
		t.Fatal(err)
	}

	if _, ok := loadCachedDevice(); ok {
		t.Fatal("loadCachedDevice() accepted corrupt JSON")
	}
}

func TestLoadCachedDeviceEmptyAddr(t *testing.T) {
	deviceCachePath = filepath.Join(t.TempDir(), "last-device.json")
	if err := os.WriteFile(deviceCachePath, []byte(`{"addr":"","name":"x","type":"dlna"}`), 0600); err != nil {
		t.Fatal(err)
	}

	if _, ok := loadCachedDevice(); ok {
		t.Fatal("loadCachedDevice() accepted an empty address")
	}
}

func TestSaveCachedDeviceOverwrites(t *testing.T) {
	deviceCachePath = filepath.Join(t.TempDir(), "last-device.json")

	first := devices.Device{Name: "Old", Addr: "http://10.0.0.1:1/", Type: devices.DeviceTypeDLNA}
	second := devices.Device{Name: "New", Addr: "192.168.1.51:8009", Type: devices.DeviceTypeChromecast}

	if err := saveCachedDevice(first); err != nil {
		t.Fatalf("saveCachedDevice() error = %v", err)
	}
	if err := saveCachedDevice(second); err != nil {
		t.Fatalf("saveCachedDevice() error = %v", err)
	}

	got, ok := loadCachedDevice()
	if !ok {
		t.Fatal("loadCachedDevice() did not find the saved device")
	}
	if got != second {
		t.Fatalf("loadCachedDevice() = %+v, want the most recent save %+v", got, second)
	}
}
