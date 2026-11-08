package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"cli2tv/devices"
)

func staticList(devs ...devices.Device) func() []devices.Device {
	return func() []devices.Device { return devs }
}

func testDevice(name, addr, devType string) devices.Device {
	return devices.Device{Name: name, Addr: addr, Type: devType}
}

// TestPickDeviceAuto covers the mode 1 selection loop: confirm the first
// discovered player, walk to the next on decline, honor abort, and time out
// when the network stays silent.
func TestPickDeviceAuto(t *testing.T) {
	t.Parallel()
	tv := testDevice("Living Room TV", "http://192.168.1.50:49152", devices.DeviceTypeDLNA)
	stick := testDevice("Bedroom Stick", "192.168.1.51:8009", devices.DeviceTypeChromecast)

	tests := []struct {
		name      string
		ctx       func() context.Context
		list      func() []devices.Device
		prompt    func(label string) deviceAnswer
		firstWait time.Duration
		idleWait  time.Duration
		want      devices.Device
		wantErr   error
	}{
		{
			name:   "plays first device on confirm",
			list:   staticList(tv, stick),
			prompt: func(string) deviceAnswer { return answerPlay },
			want:   tv,
		},
		{
			name: "decline moves to next device",
			list: staticList(tv, stick),
			prompt: func(label string) deviceAnswer {
				if label == "Living Room TV (DLNA) [http://192.168.1.50:49152]" {
					return answerNext
				}
				return answerPlay
			},
			want: stick,
		},
		{
			name:    "abort stops selection",
			list:    staticList(tv),
			prompt:  func(string) deviceAnswer { return answerAbort },
			wantErr: errAutodetectAborted,
		},
		{
			name:      "silent network times out",
			list:      staticList(),
			prompt:    func(string) deviceAnswer { return answerPlay },
			firstWait: 5 * time.Millisecond,
			idleWait:  5 * time.Millisecond,
			wantErr:   errAutodetectNone,
		},
		{
			name: "late device is discovered",
			list: func() func() []devices.Device {
				var called int
				return func() []devices.Device {
					called++
					if called < 2 {
						return nil
					}
					return []devices.Device{tv}
				}
			}(),
			prompt:    func(string) deviceAnswer { return answerPlay },
			firstWait: 20 * time.Millisecond,
			idleWait:  5 * time.Millisecond,
			want:      tv,
		},
		{
			name:   "declined devices are not re-asked",
			list:   staticList(tv),
			prompt: func(string) deviceAnswer { return answerNext },
			// Two idle windows pass with nothing new; without the asked-set
			// the loop would prompt forever and this test would hang.
			firstWait: 5 * time.Millisecond,
			idleWait:  5 * time.Millisecond,
			wantErr:   errAutodetectNone,
		},
		{
			name:      "cancelled context aborts discovery",
			ctx:       func() context.Context { ctx, cancel := context.WithCancel(context.Background()); cancel(); return ctx },
			list:      staticList(),
			prompt:    func(string) deviceAnswer { t.Error("prompt called after cancellation"); return answerPlay },
			firstWait: 5 * time.Millisecond,
			idleWait:  5 * time.Millisecond,
			wantErr:   context.Canceled,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.ctx != nil {
				ctx = tt.ctx()
			}
			firstWait := tt.firstWait
			if firstWait == 0 {
				firstWait = time.Second
			}
			idleWait := tt.idleWait
			if idleWait == 0 {
				idleWait = time.Second
			}
			got, err := pickDeviceAuto(ctx, tt.prompt, tt.list, firstWait, idleWait)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("pickDeviceAuto() error = %v, want %v", err, tt.wantErr)
			}
			if err == nil && got != tt.want {
				t.Fatalf("pickDeviceAuto() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestPickDeviceCached covers the cache fast path: an alive cached device is
// prompted instantly, a dead or declined one falls through to full discovery,
// and every successful selection is written back to the cache.
func TestPickDeviceCached(t *testing.T) {
	tv := testDevice("Living Room TV", "http://192.168.1.50:49152", devices.DeviceTypeDLNA)
	stick := testDevice("Bedroom Stick", "192.168.1.51:8009", devices.DeviceTypeChromecast)

	type fixture struct {
		cached    devices.Device
		hasCached bool
		alive     bool
		asked     []string
		saved     []devices.Device
	}

	newFixture := func(hasCached, alive bool) *fixture {
		return &fixture{cached: tv, hasCached: hasCached, alive: alive}
	}

	tests := []struct {
		name    string
		fixture *fixture
		list    func() []devices.Device
		answers []deviceAnswer
		want    devices.Device
		wantErr error
	}{
		{
			name:    "alive cached device plays instantly",
			fixture: newFixture(true, true),
			answers: []deviceAnswer{answerPlay},
			want:    tv,
		},
		{
			name:    "declined cached device falls through to discovery",
			fixture: newFixture(true, true),
			list:    staticList(tv, stick),
			answers: []deviceAnswer{answerNext, answerPlay},
			want:    stick,
		},
		{
			name:    "aborted cache prompt stops selection",
			fixture: newFixture(true, true),
			answers: []deviceAnswer{answerAbort},
			wantErr: errAutodetectAborted,
		},
		{
			name:    "dead cached device is skipped silently",
			fixture: newFixture(true, false),
			list:    staticList(stick),
			answers: []deviceAnswer{answerPlay},
			want:    stick,
		},
		{
			name:    "no cache discovers and saves",
			fixture: newFixture(false, false),
			list:    staticList(stick),
			answers: []deviceAnswer{answerPlay},
			want:    stick,
		},
		{
			name:    "declined cached device is not re-asked by discovery",
			fixture: newFixture(true, true),
			list:    staticList(tv),
			answers: []deviceAnswer{answerNext},
			// With the asked-set the loop hits the idle timeout; without it
			// the cached device would be prompted forever.
			wantErr: errAutodetectNone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := tt.fixture
			load := func() (devices.Device, bool) { return f.cached, f.hasCached }
			alive := func(devices.Device) bool { return f.alive }
			save := func(dev devices.Device) { f.saved = append(f.saved, dev) }
			prompt := func(label string) deviceAnswer {
				f.asked = append(f.asked, label)
				answer := tt.answers[0]
				tt.answers = tt.answers[1:]
				return answer
			}

			got, err := pickDeviceCached(context.Background(), prompt, tt.list, alive, load, save, time.Second, time.Second)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("pickDeviceCached() error = %v, want %v", err, tt.wantErr)
			}
			if err == nil && got != tt.want {
				t.Fatalf("pickDeviceCached() = %+v, want %+v", got, tt.want)
			}

			cacheAsks := 0
			for _, label := range f.asked {
				if label == describeDevice(tv) {
					cacheAsks++
				}
			}
			if tt.name == "declined cached device is not re-asked by discovery" && cacheAsks != 1 {
				t.Fatalf("cached device asked %d times, want exactly once", cacheAsks)
			}

			if tt.wantErr == nil {
				if len(f.saved) != 1 || f.saved[0] != tt.want {
					t.Fatalf("save called %d times with %+v, want one save of %+v", len(f.saved), f.saved, tt.want)
				}
			} else if len(f.saved) != 0 {
				t.Fatalf("save called on failure with %+v, want no saves", f.saved)
			}
		})
	}
}

func TestParseAnswer(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		line string
		err  error
		want deviceAnswer
	}{
		{name: "empty line accepts default", line: "\n", want: answerPlay},
		{name: "y accepts", line: "y\n", want: answerPlay},
		{name: "yes accepts", line: "YES\n", want: answerPlay},
		{name: "EOF accepts default", err: io.EOF, want: answerPlay},
		{name: "n moves on", line: "n\n", want: answerNext},
		{name: "no moves on", line: "no\n", want: answerNext},
		{name: "q aborts", line: "q\n", want: answerAbort},
		{name: "quit aborts", line: "quit\n", want: answerAbort},
		{name: "garbage moves on", line: "maybe\n", want: answerNext},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseAnswer(tt.line, tt.err); got != tt.want {
				t.Fatalf("parseAnswer(%q, %v) = %v, want %v", tt.line, tt.err, got, tt.want)
			}
		})
	}
}

func TestApplyPositionalMedia(t *testing.T) {
	originalMedia, originalURL, originalList := *mediaArg, *urlArg, *listPtr
	t.Cleanup(func() { *mediaArg, *urlArg, *listPtr = originalMedia, originalURL, originalList })

	tests := []struct {
		name    string
		pos     string
		media   string
		url     string
		list    bool
		want    string
		wantErr bool
	}{
		{name: "empty positional is a no-op"},
		{name: "positional becomes media", pos: "movie.mkv", want: "movie.mkv"},
		{name: "conflicts with -v", pos: "movie.mkv", media: "other.mkv", wantErr: true},
		{name: "conflicts with -u", pos: "movie.mkv", url: "http://example.com/v.mp4", wantErr: true},
		{name: "conflicts with -l", pos: "movie.mkv", list: true, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			*mediaArg, *urlArg, *listPtr = tt.media, tt.url, tt.list
			err := applyPositionalMedia(tt.pos)
			if (err != nil) != tt.wantErr {
				t.Fatalf("applyPositionalMedia(%q) error = %v, wantErr %v", tt.pos, err, tt.wantErr)
			}
			if !tt.wantErr && *mediaArg != tt.want {
				t.Fatalf("mediaArg = %q, want %q", *mediaArg, tt.want)
			}
		})
	}
}

func TestCheckVflagAutoDevice(t *testing.T) {
	dir := t.TempDir()
	media := filepath.Join(dir, "movie.mkv")
	if err := os.WriteFile(media, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}

	originalMedia, originalURL, originalTarget, originalList := *mediaArg, *urlArg, *targetPtr, *listPtr
	t.Cleanup(func() {
		*mediaArg, *urlArg, *targetPtr, *listPtr = originalMedia, originalURL, originalTarget, originalList
	})

	tests := []struct {
		name       string
		media      string
		autoDevice bool
		wantErr    bool
	}{
		{name: "existing media without target autodetects", media: media, autoDevice: true},
		{name: "existing media without target errors without autodetect", media: media, wantErr: true},
		{name: "missing media still errors with autodetect", media: filepath.Join(dir, "gone.mkv"), autoDevice: true, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			*mediaArg, *urlArg, *targetPtr, *listPtr = tt.media, "", "", false
			err := checkVflag(tt.autoDevice)
			if (err != nil) != tt.wantErr {
				t.Fatalf("checkVflag(auto=%v) error = %v, wantErr %v", tt.autoDevice, err, tt.wantErr)
			}
		})
	}
}

func TestMediaDisplayName(t *testing.T) {
	originalMedia, originalURL := *mediaArg, *urlArg
	t.Cleanup(func() { *mediaArg, *urlArg = originalMedia, originalURL })

	tests := []struct {
		name      string
		media     string
		url       string
		wantLabel string
	}{
		{name: "local file uses base name", media: "movies/movie.mkv", wantLabel: "movie.mkv"},
		{name: "stdin stream", media: "-", wantLabel: "stdin stream"},
		{name: "url wins over media", media: "-", url: "http://example.com/stream.m3u8", wantLabel: "http://example.com/stream.m3u8"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			*mediaArg, *urlArg = tt.media, tt.url
			if got := mediaDisplayName(); got != tt.wantLabel {
				t.Fatalf("mediaDisplayName() = %q, want %q", got, tt.wantLabel)
			}
		})
	}
}
