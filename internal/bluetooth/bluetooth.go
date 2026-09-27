// Package bluetooth controls the host's Bluetooth adapter through bluetoothctl.
package bluetooth

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"
)

type Device struct {
	MAC       string `json:"mac"`
	Name      string `json:"name"`
	Icon      string `json:"icon"` // e.g. audio-headset, audio-card, phone
	Paired    bool   `json:"paired"`
	Trusted   bool   `json:"trusted"`
	Connected bool   `json:"connected"`
}

type Status struct {
	Available bool     `json:"available"`
	Powered   bool     `json:"powered"`
	Devices   []Device `json:"devices"`
}

// Run executes bluetoothctl with args. Replaced in tests.
var Run = func(ctx context.Context, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "bluetoothctl", args...).CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("bluetoothctl %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return out, nil
}

func Available() bool {
	_, err := exec.LookPath("bluetoothctl")
	return err == nil
}

var macRe = regexp.MustCompile(`^([0-9A-F]{2}:){5}[0-9A-F]{2}$`)

func checkMAC(mac string) error {
	if !macRe.MatchString(mac) {
		return fmt.Errorf("invalid device address %q", mac)
	}
	return nil
}

func Get(ctx context.Context) (Status, error) {
	if !Available() {
		return Status{}, nil
	}
	st := Status{Available: true}
	show, err := Run(ctx, "show")
	if err != nil {
		return st, err
	}
	st.Powered = parseInfo(show)["Powered"] == "yes"

	out, err := Run(ctx, "devices")
	if err != nil {
		return st, err
	}
	for _, d := range parseDevices(out) {
		if info, err := Run(ctx, "info", d.MAC); err == nil {
			kv := parseInfo(info)
			d.Icon = kv["Icon"]
			d.Paired = kv["Paired"] == "yes"
			d.Trusted = kv["Trusted"] == "yes"
			d.Connected = kv["Connected"] == "yes"
		}
		st.Devices = append(st.Devices, d)
	}
	sort.SliceStable(st.Devices, func(i, j int) bool {
		a, b := st.Devices[i], st.Devices[j]
		if a.Connected != b.Connected {
			return a.Connected
		}
		if a.Paired != b.Paired {
			return a.Paired
		}
		return a.Name < b.Name
	})
	return st, nil
}

// parseDevices reads lines like "Device AA:BB:CC:DD:EE:FF JBL Flip 5".
func parseDevices(out []byte) []Device {
	var ds []Device
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		f := strings.SplitN(strings.TrimSpace(sc.Text()), " ", 3)
		if len(f) < 2 || f[0] != "Device" || !macRe.MatchString(f[1]) {
			continue
		}
		d := Device{MAC: f[1], Name: f[1]}
		if len(f) == 3 {
			d.Name = f[2]
		}
		ds = append(ds, d)
	}
	return ds
}

// parseInfo reads "\tKey: value" lines from `show` / `info`.
func parseInfo(out []byte) map[string]string {
	kv := map[string]string{}
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		k, v, ok := strings.Cut(strings.TrimSpace(sc.Text()), ":")
		if !ok {
			continue
		}
		if _, seen := kv[k]; !seen {
			kv[k] = strings.TrimSpace(v)
		}
	}
	return kv
}

func PowerOn(ctx context.Context) error {
	_, err := Run(ctx, "power", "on")
	return err
}

// Scan looks for nearby devices for the given duration.
func Scan(ctx context.Context, d time.Duration) error {
	_, err := Run(ctx, "--timeout", fmt.Sprint(int(d.Seconds())), "scan", "on")
	return err
}

// Connect pairs (if needed), trusts and connects a device, so it
// reconnects on its own after a reboot.
func Connect(ctx context.Context, mac string) error {
	if err := checkMAC(mac); err != nil {
		return err
	}
	info, err := Run(ctx, "info", mac)
	if err != nil {
		return err
	}
	if parseInfo(info)["Paired"] != "yes" {
		if _, err := Run(ctx, "pair", mac); err != nil {
			return err
		}
	}
	if _, err := Run(ctx, "trust", mac); err != nil {
		return err
	}
	_, err = Run(ctx, "connect", mac)
	return err
}

func Disconnect(ctx context.Context, mac string) error {
	if err := checkMAC(mac); err != nil {
		return err
	}
	_, err := Run(ctx, "disconnect", mac)
	return err
}

func Forget(ctx context.Context, mac string) error {
	if err := checkMAC(mac); err != nil {
		return err
	}
	_, err := Run(ctx, "remove", mac)
	return err
}
