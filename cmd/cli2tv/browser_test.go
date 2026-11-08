package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"cli2tv/devices"
)

func TestMediaExt(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		want bool
	}{
		{name: "movie.mkv", want: true},
		{name: "movie.MKV", want: true},
		{name: "song.mp3", want: true},
		{name: "clip.mp4", want: true},
		{name: "audio.flac", want: true},
		{name: "notes.txt"},
		{name: "archive.zip"},
		{name: "noext"},
		{name: ".hidden"},
		{name: "movie.mkv.bak"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mediaExt(tt.name); got != tt.want {
				t.Fatalf("mediaExt(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestListDir(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"b.mkv": "x", "a.mp3": "x", "notes.txt": "x", "z.mkv": "x",
		".hidden.mp4": "x",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, sub := range []string{"zdir", "adir", ".hiddendir"} {
		if err := os.Mkdir(filepath.Join(dir, sub), 0700); err != nil {
			t.Fatal(err)
		}
	}

	entries, err := listDir(dir)
	if err != nil {
		t.Fatalf("listDir() error = %v", err)
	}

	wantNames := []string{"adir", "zdir", "a.mp3", "b.mkv", "z.mkv"}
	if len(entries) != len(wantNames) {
		t.Fatalf("listDir() returned %d entries %+v, want %v", len(entries), entries, wantNames)
	}
	for i, want := range wantNames {
		if entries[i].name != want {
			t.Fatalf("listDir()[%d] = %q, want %q (full: %+v)", i, entries[i].name, want, entries)
		}
	}
	for _, e := range entries {
		wantDir := !strings.HasSuffix(e.name, ".mp3") && !strings.HasSuffix(e.name, ".mkv")
		if e.dir != wantDir {
			t.Fatalf("listDir() entry %q dir = %v, want %v", e.name, e.dir, wantDir)
		}
	}
}

func TestListDirMissing(t *testing.T) {
	if _, err := listDir(filepath.Join(t.TempDir(), "gone")); err == nil {
		t.Fatal("listDir() on a missing directory should fail")
	}
}

func TestVisibleWindow(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		count          int
		cursor         int
		height         int
		wantLo, wantHi int
	}{
		{name: "everything fits", count: 5, cursor: 2, height: 10, wantLo: 0, wantHi: 5},
		{name: "zero height shows nothing", count: 5, cursor: 2, height: 0, wantLo: 0, wantHi: 0},
		{name: "top of long list", count: 10, cursor: 0, height: 3, wantLo: 0, wantHi: 3},
		{name: "bottom of long list", count: 10, cursor: 9, height: 3, wantLo: 7, wantHi: 10},
		{name: "middle of long list", count: 10, cursor: 4, height: 3, wantLo: 3, wantHi: 6},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lo, hi := visibleWindow(tt.count, tt.cursor, tt.height)
			if lo != tt.wantLo || hi != tt.wantHi {
				t.Fatalf("visibleWindow(%d, %d, %d) = (%d, %d), want (%d, %d)",
					tt.count, tt.cursor, tt.height, lo, hi, tt.wantLo, tt.wantHi)
			}
		})
	}
}

// newTestBrowser builds a browser model whose listing and device polling are
// faked in memory.
func newTestBrowser(t *testing.T, root string, listings map[string][]browserEntry, devList []devices.Device) browserModel {
	t.Helper()
	return newBrowserModel(root,
		func(dir string) ([]browserEntry, error) {
			if entries, ok := listings[dir]; ok {
				return entries, nil
			}
			return nil, os.ErrNotExist
		},
		func() []devices.Device { return devList },
	)
}

func sendKey(t *testing.T, m browserModel, key tea.KeyType) browserModel {
	t.Helper()
	updated, _ := m.Update(tea.KeyMsg{Type: key})
	next, ok := updated.(browserModel)
	if !ok {
		t.Fatalf("Update returned %T, want browserModel", updated)
	}
	return next
}

func TestBrowserNavigation(t *testing.T) {
	listings := map[string][]browserEntry{
		"/media": {
			{name: "shows", dir: true},
			{name: "a.mkv"},
			{name: "b.mkv"},
		},
		"/media/shows": {
			{name: "s1.mkv"},
		},
	}
	tv := testDevice("LG TV", "http://192.168.1.75:1152/", devices.DeviceTypeDLNA)

	t.Run("initial listing loads with cursor at top", func(t *testing.T) {
		m := newTestBrowser(t, "/media", listings, []devices.Device{tv})
		if len(m.entries) != 3 || m.entries[0].name != "shows" {
			t.Fatalf("entries = %+v, want the /media listing", m.entries)
		}
		if m.cursor != 0 {
			t.Fatalf("cursor = %d, want 0", m.cursor)
		}
	})

	t.Run("cursor moves and clamps", func(t *testing.T) {
		m := newTestBrowser(t, "/media", listings, nil)
		m = sendKey(t, m, tea.KeyDown)
		m = sendKey(t, m, tea.KeyDown)
		if m.cursor != 2 {
			t.Fatalf("cursor = %d, want 2", m.cursor)
		}
		m = sendKey(t, m, tea.KeyDown)
		if m.cursor != 2 {
			t.Fatalf("cursor = %d, want clamped at 2", m.cursor)
		}
		m = sendKey(t, m, tea.KeyUp)
		if m.cursor != 1 {
			t.Fatalf("cursor = %d, want 1", m.cursor)
		}
		m = sendKey(t, m, tea.KeyUp)
		m = sendKey(t, m, tea.KeyUp)
		if m.cursor != 0 {
			t.Fatalf("cursor = %d, want clamped at 0", m.cursor)
		}
	})

	t.Run("enter on directory descends", func(t *testing.T) {
		m := newTestBrowser(t, "/media", listings, nil)
		m = sendKey(t, m, tea.KeyEnter)
		if m.root != "/media/shows" {
			t.Fatalf("root = %q, want /media/shows", m.root)
		}
		if len(m.entries) != 1 || m.entries[0].name != "s1.mkv" {
			t.Fatalf("entries = %+v, want the /media/shows listing", m.entries)
		}
		if m.cursor != 0 {
			t.Fatalf("cursor = %d, want reset to 0", m.cursor)
		}
	})

	t.Run("esc in a subdirectory goes back up", func(t *testing.T) {
		m := newTestBrowser(t, "/media", listings, nil)
		m = sendKey(t, m, tea.KeyEnter)
		m = sendKey(t, m, tea.KeyEsc)
		if m.root != "/media" {
			t.Fatalf("root = %q, want /media", m.root)
		}
		if len(m.entries) != 3 {
			t.Fatalf("entries = %+v, want the /media listing", m.entries)
		}
	})

	t.Run("esc at the start root quits without selection", func(t *testing.T) {
		m := newTestBrowser(t, "/media", listings, nil)
		m = sendKey(t, m, tea.KeyEsc)
		if !m.quit {
			t.Fatal("esc at root should quit")
		}
		if m.selection != nil {
			t.Fatalf("selection = %+v, want none", m.selection)
		}
	})

	t.Run("q quits from anywhere", func(t *testing.T) {
		m := newTestBrowser(t, "/media", listings, nil)
		m = sendKey(t, m, tea.KeyEnter)
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
		m = updated.(browserModel)
		if !m.quit {
			t.Fatal("q should quit")
		}
	})
}

func TestBrowserPlay(t *testing.T) {
	listings := map[string][]browserEntry{
		"/media": {{name: "a.mkv"}},
	}
	tv := testDevice("LG TV", "http://192.168.1.75:1152/", devices.DeviceTypeDLNA)
	stick := testDevice("Stick", "192.168.1.51:8009", devices.DeviceTypeChromecast)

	t.Run("single player is selected automatically", func(t *testing.T) {
		m := newTestBrowser(t, "/media", listings, []devices.Device{tv})
		m = sendKey(t, m, tea.KeyEnter)
		if m.selection == nil {
			t.Fatal("selection = nil, want the single discovered player")
		}
		if !m.quit {
			t.Fatal("playing on the only discovered player should finish the browser")
		}
		want := "/media/a.mkv"
		if m.selection.mediaPath != want {
			t.Fatalf("mediaPath = %q, want %q", m.selection.mediaPath, want)
		}
		if m.selection.device != tv {
			t.Fatalf("device = %+v, want %+v", m.selection.device, tv)
		}
	})

	t.Run("no players yet keeps browsing with a status hint", func(t *testing.T) {
		m := newTestBrowser(t, "/media", listings, nil)
		m = sendKey(t, m, tea.KeyEnter)
		if m.selection != nil || m.quit {
			t.Fatal("nothing should be selected while no player is known")
		}
		if m.status == "" {
			t.Fatal("status should explain that discovery is still running")
		}
	})

	t.Run("multiple players open the chooser", func(t *testing.T) {
		m := newTestBrowser(t, "/media", listings, []devices.Device{tv, stick})
		m = sendKey(t, m, tea.KeyEnter)
		if m.selection != nil || m.quit {
			t.Fatal("the chooser should stay open until a player is picked")
		}
		if !m.chooser {
			t.Fatal("chooser mode not entered")
		}

		m = sendKey(t, m, tea.KeyDown)
		m = sendKey(t, m, tea.KeyEnter)
		if m.selection == nil || m.selection.device != stick {
			t.Fatalf("selection = %+v, want the second player %+v", m.selection, stick)
		}
		if !m.quit {
			t.Fatal("picking a player should finish the browser")
		}
	})

	t.Run("esc in the chooser returns to browsing", func(t *testing.T) {
		m := newTestBrowser(t, "/media", listings, []devices.Device{tv, stick})
		m = sendKey(t, m, tea.KeyEnter)
		m = sendKey(t, m, tea.KeyEsc)
		if m.chooser {
			t.Fatal("esc should leave the chooser")
		}
		if m.selection != nil || m.quit {
			t.Fatal("esc in the chooser should not select or quit")
		}
		if len(m.entries) != 1 || m.entries[0].name != "a.mkv" {
			t.Fatalf("entries = %+v, want the /media listing", m.entries)
		}
	})
}

func TestBrowserDiscoveryUpdate(t *testing.T) {
	m := newTestBrowser(t, "/media", map[string][]browserEntry{"/media": {{name: "a.mkv"}}}, nil)

	tv := testDevice("LG TV", "http://192.168.1.75:1152/", devices.DeviceTypeDLNA)
	updated, _ := m.Update(devicesMsg([]devices.Device{tv}))
	m = updated.(browserModel)

	if len(m.devices) != 1 || m.devices[0] != tv {
		t.Fatalf("devices = %+v, want the discovered player", m.devices)
	}

	// A late-arriving player list must not clobber a finished selection.
	m.selection = &browserSelection{mediaPath: "/media/a.mkv", device: tv}
	updated, _ = m.Update(devicesMsg(nil))
	m = updated.(browserModel)
	if m.selection == nil {
		t.Fatal("devicesMsg wiped an existing selection")
	}
}

func TestBrowserView(t *testing.T) {
	listings := map[string][]browserEntry{
		"/media": {{name: "shows", dir: true}, {name: "a.mkv"}},
	}
	m := newTestBrowser(t, "/media", listings, nil)
	m.height = 24

	view := m.View()
	for _, want := range []string{"/media", "shows", "a.mkv"} {
		if !strings.Contains(view, want) {
			t.Fatalf("View() missing %q:\n%s", want, view)
		}
	}
}

// TestBrowserRequest covers when the CLI switches from direct casting to the
// media browser: a directory argument (positional or -v) opens the browser
// rooted there, a bare terminal invocation browses the current folder, and
// explicit media sources, -l, -version, and piped stdin are untouched.
func TestBrowserRequest(t *testing.T) {
	dir := t.TempDir()
	media := filepath.Join(dir, "movie.mkv")
	if err := os.WriteFile(media, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}

	originalMedia, originalURL, originalList, originalVersion := *mediaArg, *urlArg, *listPtr, *versionPtr
	t.Cleanup(func() {
		*mediaArg, *urlArg, *listPtr, *versionPtr = originalMedia, originalURL, originalList, originalVersion
	})

	tests := []struct {
		name       string
		media      string
		url        string
		list       bool
		version    bool
		pipedIn    bool
		wantRoot   string
		wantBrowse bool
	}{
		{name: "bare terminal invocation browses cwd", wantRoot: ".", wantBrowse: true},
		{name: "piped stdin keeps stdin streaming", pipedIn: true},
		{name: "media file casts directly", media: media},
		{name: "directory argument opens browser rooted there", media: dir, wantRoot: dir, wantBrowse: true},
		{name: "url keeps url casting", url: "http://example.com/v.mp4"},
		{name: "list flag skips browser", list: true},
		{name: "version flag skips browser", version: true},
		{name: "missing path still errors via checkVflag", media: filepath.Join(dir, "gone.mkv")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			*mediaArg, *urlArg, *listPtr, *versionPtr = tt.media, tt.url, tt.list, tt.version
			root, ok := browserRequest(tt.pipedIn)
			if ok != tt.wantBrowse {
				t.Fatalf("browserRequest() browse = %v, want %v", ok, tt.wantBrowse)
			}
			if root != tt.wantRoot {
				t.Fatalf("browserRequest() root = %q, want %q", root, tt.wantRoot)
			}
		})
	}
}
