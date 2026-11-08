package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cli2tv/devices"
	"cli2tv/utils"
)

var (
	errAutodetectAborted = errors.New("no device selected")
	errAutodetectNone    = errors.New("no DLNA/Chromecast devices found on the network")
)

const (
	firstDeviceTimeout = 15 * time.Second
	idleDeviceTimeout  = 10 * time.Second
)

type deviceAnswer int

const (
	answerPlay deviceAnswer = iota
	answerNext
	answerAbort
)

type promptFunc func(label string) deviceAnswer

// pickDeviceAuto implements mode 1: walk the discovered players in order,
// ask about each, and stop on the first acceptance. Declined devices are
// never re-asked; the search ends once no new device shows up within
// firstWait (before the first device) or idleWait (afterward).
func pickDeviceAuto(ctx context.Context, prompt promptFunc, list func() []devices.Device, firstWait, idleWait time.Duration) (devices.Device, error) {
	asked := make(map[string]bool)

	firstDeadline := time.Now().Add(firstWait)
	var idleDeadline time.Time

	for {
		var fresh []devices.Device
		for _, dev := range list() {
			if !asked[dev.Addr] {
				fresh = append(fresh, dev)
			}
		}

		if len(fresh) > 0 {
			idleDeadline = time.Now().Add(idleWait)
			for _, dev := range fresh {
				asked[dev.Addr] = true
				switch prompt(describeDevice(dev)) {
				case answerPlay:
					return dev, nil
				case answerAbort:
					return devices.Device{}, errAutodetectAborted
				}
			}
			continue
		}

		deadline := firstDeadline
		if !idleDeadline.IsZero() {
			deadline = idleDeadline
		}
		if time.Now().After(deadline) {
			return devices.Device{}, errAutodetectNone
		}

		select {
		case <-ctx.Done():
			return devices.Device{}, ctx.Err()
		case <-time.After(time.Until(deadline)):
		}
	}
}

// pickDeviceCached layers the last-selected-device cache on top of
// pickDeviceAuto: an alive cached device is prompted instantly, skipping the
// discovery wait entirely. A dead or declined cached device falls through to
// the normal discovery loop without being re-asked. Every successful
// selection is written back through save.
func pickDeviceCached(ctx context.Context, prompt promptFunc, list func() []devices.Device, alive func(devices.Device) bool, load func() (devices.Device, bool), save func(devices.Device), firstWait, idleWait time.Duration) (devices.Device, error) {
	asked := make(map[string]bool)

	if cached, ok := load(); ok {
		if alive(cached) {
			asked[cached.Addr] = true
			switch prompt(describeDevice(cached)) {
			case answerPlay:
				save(cached)
				return cached, nil
			case answerAbort:
				return devices.Device{}, errAutodetectAborted
			}
		}
	}

	dev, err := pickDeviceAuto(ctx, prompt, func() []devices.Device {
		var fresh []devices.Device
		for _, dev := range list() {
			if !asked[dev.Addr] {
				fresh = append(fresh, dev)
			}
		}
		return fresh
	}, firstWait, idleWait)
	if err != nil {
		return devices.Device{}, err
	}
	save(dev)
	return dev, nil
}

func describeDevice(dev devices.Device) string {
	return dev.Name + " (" + typeTag(dev.Type) + ") [" + dev.Addr + "]"
}

// parseAnswer maps a confirmation prompt reply onto the selection loop:
// empty input and EOF take the default (play), n walks to the next player,
// q aborts the search entirely.
func parseAnswer(line string, err error) deviceAnswer {
	if err != nil && strings.TrimSpace(line) == "" {
		return answerPlay
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "", "y", "yes":
		return answerPlay
	case "q", "quit":
		return answerAbort
	default:
		return answerNext
	}
}

// confirmDevicePrompt asks the user about each candidate on the terminal.
// A cancelled context aborts the pending read.
func confirmDevicePrompt(ctx context.Context, mediaLabel string) promptFunc {
	reader := bufio.NewReader(os.Stdin)
	return func(label string) deviceAnswer {
		fmt.Printf("Play %q on %s? [Y/n/q] ", mediaLabel, label)

		type reply struct {
			line string
			err  error
		}
		ch := make(chan reply, 1)
		go func() {
			line, err := reader.ReadString('\n')
			ch <- reply{line, err}
		}()

		select {
		case <-ctx.Done():
			fmt.Println()
			return answerAbort
		case r := <-ch:
			return parseAnswer(r.line, r.err)
		}
	}
}

// autoPlayPrompt skips confirmation when stdin is not a terminal.
func autoPlayPrompt(mediaLabel string) promptFunc {
	return func(label string) deviceAnswer {
		fmt.Printf("Auto-selecting %s for %q (non-interactive)\n", label, mediaLabel)
		return answerPlay
	}
}

// applyPositionalMedia adopts a positional media file argument. It rejects
// combinations that would leave the media source ambiguous.
func applyPositionalMedia(pos string) error {
	if pos == "" {
		return nil
	}
	switch {
	case *mediaArg != "":
		return errors.New("media file specified both positionally and with -v")
	case *urlArg != "":
		return errors.New("media file specified both positionally and with -u")
	case *listPtr:
		return errNoCombi
	}
	*mediaArg = pos
	return nil
}

// mediaDisplayName renders the media being cast for confirmation prompts.
func mediaDisplayName() string {
	switch {
	case *urlArg != "":
		return *urlArg
	case *mediaArg == "-":
		return "stdin stream"
	default:
		return filepath.Base(*mediaArg)
	}
}

// cachedDeviceAlive probes whether the remembered device still responds.
// DLNA addresses are URLs; Chromecast addresses are bare host:port values
// that url.Parse rejects, so parse failures keep the address as-is.
func cachedDeviceAlive(dev devices.Device) bool {
	addr := dev.Addr
	if u, err := url.Parse(addr); err == nil && u.Host != "" {
		addr = u.Host
	}
	return utils.HostPortIsAlive(addr)
}

// autodetectDevice discovers renderers and returns the URL of the first one
// the user confirms (or the first found, when running non-interactively).
// The last confirmed device is cached and probed first to skip discovery.
func autodetectDevice(ctx context.Context) (string, error) {
	devices.StartDiscovery(context.Background())

	mediaLabel := mediaDisplayName()
	prompt := confirmDevicePrompt(ctx, mediaLabel)
	if checkStdin() {
		prompt = autoPlayPrompt(mediaLabel)
	}

	dev, err := pickDeviceCached(ctx, prompt, func() []devices.Device {
		list, _ := devices.LoadAllDevices()
		return list
	}, cachedDeviceAlive, loadCachedDevice, func(d devices.Device) {
		_ = saveCachedDevice(d)
	}, firstDeviceTimeout, idleDeviceTimeout)
	if err != nil {
		return "", err
	}

	fmt.Printf("\nSelected %s\n\n", describeDevice(dev))
	return dev.Addr, nil
}
