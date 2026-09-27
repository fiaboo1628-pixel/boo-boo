package apps

import (
	"testing"
	"testing/fstest"
)

func TestParseState(t *testing.T) {
	cases := map[string]string{
		"": "stopped",
		`{"State":"running"}` + "\n" + `{"State":"exited"}`: "running",
		`{"State":"exited"}`:    "stopped",
		`[{"State":"running"}]`: "running",
	}
	for in, want := range cases {
		if got := parseState([]byte(in)); got != want {
			t.Errorf("parseState(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestManagerRejectsUnknownApp(t *testing.T) {
	fsys := fstest.MapFS{
		"demo/app.json":           {Data: []byte(`{"name":"Demo","port":1234}`)},
		"demo/docker-compose.yml": {Data: []byte("services: {}\n")},
	}
	m, err := NewManager(fsys, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Install(t.Context(), "../etc"); err == nil {
		t.Fatal("expected error for unknown app id")
	}
	list := m.List(t.Context())
	if len(list) != 1 || list[0].ID != "demo" || list[0].Installed {
		t.Fatalf("unexpected list: %+v", list)
	}
}
