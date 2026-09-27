package bluetooth

import (
	"context"
	"strings"
	"testing"
)

func TestParseDevices(t *testing.T) {
	out := "Device AA:BB:CC:DD:EE:FF JBL Flip 5\nDevice 11:22:33:44:55:66\n[CHG] Controller x\nDevice bad Name\n"
	ds := parseDevices([]byte(out))
	if len(ds) != 2 || ds[0].Name != "JBL Flip 5" || ds[1].Name != "11:22:33:44:55:66" {
		t.Fatalf("got %+v", ds)
	}
}

func TestParseInfo(t *testing.T) {
	out := "Device AA:BB:CC:DD:EE:FF (public)\n\tName: JBL\n\tIcon: audio-card\n\tPaired: yes\n\tConnected: no\n"
	kv := parseInfo([]byte(out))
	if kv["Icon"] != "audio-card" || kv["Paired"] != "yes" || kv["Connected"] != "no" {
		t.Fatalf("got %v", kv)
	}
}

func TestConnectPairsTrustsAndConnects(t *testing.T) {
	var calls []string
	Run = func(_ context.Context, args ...string) ([]byte, error) {
		calls = append(calls, strings.Join(args, " "))
		if args[0] == "info" {
			return []byte("\tPaired: no\n"), nil
		}
		return nil, nil
	}
	if err := Connect(context.Background(), "AA:BB:CC:DD:EE:FF"); err != nil {
		t.Fatal(err)
	}
	want := "info AA:BB:CC:DD:EE:FF|pair AA:BB:CC:DD:EE:FF|trust AA:BB:CC:DD:EE:FF|connect AA:BB:CC:DD:EE:FF"
	if got := strings.Join(calls, "|"); got != want {
		t.Fatalf("calls = %s", got)
	}
	if err := Connect(context.Background(), "AA:BB; rm -rf /"); err == nil {
		t.Fatal("expected invalid MAC error")
	}
}
