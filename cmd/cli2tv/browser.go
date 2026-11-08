package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"cli2tv/devices"
)

// browserRequest decides whether the CLI should open the media browser
// instead of casting directly. A directory argument (positional or -v) opens
// the browser rooted there; a bare terminal invocation browses the current
// folder. Explicit media sources, -l, -version, and piped stdin are
// unaffected.
func browserRequest(pipedStdin bool) (string, bool) {
	if *listPtr || *versionPtr {
		return "", false
	}
	if *mediaArg != "" {
		info, err := os.Stat(*mediaArg)
		if err == nil && info.IsDir() {
			return *mediaArg, true
		}
		return "", false
	}
	if *urlArg != "" || pipedStdin {
		return "", false
	}
	return ".", true
}

type browserEntry struct {
	name string
	dir  bool
}

type browserSelection struct {
	mediaPath string
	device    devices.Device
}

type devicesMsg []devices.Device

const defaultBrowserHeight = 24

var mediaExtensions = map[string]bool{
	".3gp": true, ".aac": true, ".avi": true, ".flac": true, ".flv": true,
	".m4a": true, ".m4v": true, ".mkv": true, ".mov": true, ".mp3": true,
	".mp4": true, ".mpeg": true, ".mpg": true, ".mpv": true, ".oga": true,
	".ogg": true, ".opus": true, ".ts": true, ".wav": true, ".webm": true,
	".wmv": true,
}

func mediaExt(name string) bool {
	return mediaExtensions[strings.ToLower(filepath.Ext(name))]
}

// listDir returns the subdirectories followed by the castable media files of
// root, both alphabetically sorted, skipping hidden entries.
func listDir(root string) ([]browserEntry, error) {
	dirEntries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var dirs, files []browserEntry
	for _, de := range dirEntries {
		name := de.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		switch {
		case de.IsDir():
			dirs = append(dirs, browserEntry{name: name, dir: true})
		case mediaExt(name):
			files = append(files, browserEntry{name: name})
		}
	}
	slices.SortFunc(dirs, func(a, b browserEntry) int { return strings.Compare(a.name, b.name) })
	slices.SortFunc(files, func(a, b browserEntry) int { return strings.Compare(a.name, b.name) })
	return append(dirs, files...), nil
}

// visibleWindow returns the [lo, hi) slice of a list that keeps the cursor on
// screen when at most height rows are available.
func visibleWindow(count, cursor, height int) (int, int) {
	if height <= 0 {
		return 0, 0
	}
	if count <= height {
		return 0, count
	}
	lo := cursor - height/2
	lo = max(0, min(lo, count-height))
	return lo, lo + height
}

type browserModel struct {
	start       string
	root        string
	entries     []browserEntry
	cursor      int
	devices     []devices.Device
	chooserList []devices.Device
	pendingPath string
	chooser     bool
	status      string
	quit        bool
	selection   *browserSelection
	height      int
	width       int
	load        func(string) ([]browserEntry, error)
	poll        func() []devices.Device
}

func newBrowserModel(root string, load func(string) ([]browserEntry, error), poll func() []devices.Device) browserModel {
	m := browserModel{
		start:   root,
		root:    root,
		load:    load,
		poll:    poll,
		height:  defaultBrowserHeight,
		devices: poll(),
	}
	m.reload(root)
	if len(m.devices) == 0 {
		m.status = "searching for players..."
	}
	return m
}

func (m *browserModel) reload(root string) {
	m.root = root
	entries, err := m.load(root)
	if err != nil {
		m.entries = nil
		m.status = "cannot read " + root + ": " + err.Error()
		return
	}
	m.entries = entries
	m.cursor = 0
}

func pollDevicesCmd(poll func() []devices.Device) tea.Cmd {
	return func() tea.Msg { return devicesMsg(poll()) }
}

func (m browserModel) Init() tea.Cmd {
	devices.StartDiscovery(context.Background())
	return pollDevicesCmd(m.poll)
}

func (m browserModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.height, m.width = msg.Height, msg.Width

	case devicesMsg:
		m.devices = msg
		if len(m.devices) > 0 {
			m.status = ""
		}
		return m, tea.Tick(time.Second, func(time.Time) tea.Msg { return pollDevicesCmd(m.poll)() })

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.quit = true
			return m, tea.Quit
		case "up":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down":
			limit := len(m.entries)
			if m.chooser {
				limit = len(m.chooserList)
			}
			if m.cursor < limit-1 {
				m.cursor++
			}
		case "enter":
			if m.chooser {
				if m.cursor < len(m.chooserList) {
					m.selection = &browserSelection{
						mediaPath: m.pendingPath,
						device:    m.chooserList[m.cursor],
					}
					m.quit = true
					return m, tea.Quit
				}
			} else if len(m.entries) > 0 {
				entry := m.entries[m.cursor]
				if entry.dir {
					m.reload(filepath.Join(m.root, entry.name))
				} else {
					m.play(filepath.Join(m.root, entry.name))
				}
			}
		case "esc":
			if m.chooser {
				m.chooser = false
				m.cursor = 0
			} else if m.root != m.start {
				m.reload(filepath.Dir(m.root))
			} else {
				m.quit = true
				return m, tea.Quit
			}
		}
	}

	return m, nil
}

// play selects the file at path: the only discovered player is used
// directly, several players open the chooser, and none keeps the browser
// open with a hint.
func (m *browserModel) play(path string) {
	switch {
	case len(m.devices) == 0:
		m.status = "no players discovered yet — give the network a moment"
	case len(m.devices) == 1:
		m.selection = &browserSelection{mediaPath: path, device: m.devices[0]}
		m.quit = true
	default:
		m.chooser = true
		m.chooserList = m.devices
		m.pendingPath = path
		m.cursor = 0
	}
}

func (m browserModel) View() string {
	var b strings.Builder

	if m.chooser {
		b.WriteString("Choose a player for " + filepath.Base(m.pendingPath) + ":\n\n")
		lo, hi := visibleWindow(len(m.chooserList), m.cursor, m.height-4)
		for i := lo; i < hi; i++ {
			dev := m.chooserList[i]
			marker := "  "
			if i == m.cursor {
				marker = "▸ "
			}
			b.WriteString(marker + dev.Name + " " + typeTag(dev.Type) + "\n")
		}
		b.WriteString("\nenter: play · esc: back · q: quit")
		return b.String()
	}

	b.WriteString("Cast from " + m.root + "\n\n")

	switch {
	case len(m.entries) == 0 && m.status == "":
		b.WriteString("no castable media in this folder\n")
	}

	lo, hi := visibleWindow(len(m.entries), m.cursor, m.height-4)
	for i := lo; i < hi; i++ {
		e := m.entries[i]
		marker := "  "
		if i == m.cursor {
			marker = "▸ "
		}
		name := e.name
		if e.dir {
			name += "/"
		}
		b.WriteString(marker + name + "\n")
	}

	footer := m.status
	if footer == "" {
		switch {
		case len(m.devices) == 0:
			footer = "searching for players..."
		case len(m.devices) == 1:
			footer = "player: " + m.devices[0].Name
		default:
			footer = fmt.Sprintf("players: %d discovered", len(m.devices))
		}
	}

	b.WriteString("\n" + footer + "\n")
	b.WriteString("up/down: select · enter: play/open · esc: back/quit · q: quit")
	return b.String()
}

// runBrowser opens the interactive media browser rooted at dir. A nil
// selection means the user left without playing anything.
func runBrowser(root string) (*browserSelection, error) {
	p := tea.NewProgram(newBrowserModel(root, listDir, func() []devices.Device {
		list, _ := devices.LoadAllDevices()
		return list
	}))
	final, err := p.Run()
	if err != nil {
		return nil, err
	}
	m, ok := final.(browserModel)
	if !ok {
		return nil, nil
	}
	return m.selection, nil
}

