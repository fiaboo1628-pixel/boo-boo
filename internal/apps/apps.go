// Package apps manages app-store apps as docker compose projects.
package apps

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type App struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Icon        string `json:"icon"`
	Port        int    `json:"port"`
	Path        string `json:"path,omitempty"`
	compose     []byte
}

type Status struct {
	App
	Installed bool   `json:"installed"`
	State     string `json:"state"` // not_installed, running, stopped, unknown
}

type Manager struct {
	dataDir string
	apps    map[string]App
	mu      sync.Mutex // serialises compose operations
}

// NewManager loads the catalog from fsys (one folder per app).
func NewManager(fsys fs.FS, dataDir string) (*Manager, error) {
	m := &Manager{dataDir: filepath.Join(dataDir, "apps"), apps: map[string]App{}}
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		meta, err := fs.ReadFile(fsys, path.Join(e.Name(), "app.json"))
		if err != nil {
			return nil, err
		}
		var a App
		if err := json.Unmarshal(meta, &a); err != nil {
			return nil, fmt.Errorf("%s/app.json: %w", e.Name(), err)
		}
		a.ID = e.Name()
		if a.compose, err = fs.ReadFile(fsys, path.Join(e.Name(), "docker-compose.yml")); err != nil {
			return nil, err
		}
		m.apps[a.ID] = a
	}
	return m, os.MkdirAll(m.dataDir, 0o755)
}

func (m *Manager) appDir(id string) string { return filepath.Join(m.dataDir, id) }
func (m *Manager) composeFile(id string) string {
	return filepath.Join(m.appDir(id), "docker-compose.yml")
}
func project(id string) string { return "booboo-" + id }

func (m *Manager) installed(id string) bool {
	_, err := os.Stat(m.composeFile(id))
	return err == nil
}

// List returns every catalog app with its install/run state.
func (m *Manager) List(ctx context.Context) []Status {
	out := make([]Status, 0, len(m.apps))
	for _, a := range m.apps {
		s := Status{App: a, State: "not_installed"}
		if m.installed(a.ID) {
			s.Installed = true
			s.State = m.state(ctx, a.ID)
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (m *Manager) get(id string) (App, error) {
	a, ok := m.apps[id]
	if !ok {
		return App{}, fmt.Errorf("unknown app %q", id)
	}
	return a, nil
}

func (m *Manager) Install(ctx context.Context, id string) error {
	a, err := m.get(id)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := os.MkdirAll(filepath.Join(m.appDir(id), "data"), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(m.composeFile(id), a.compose, 0o644); err != nil {
		return err
	}
	if _, err := m.compose(ctx, id, "up", "-d"); err != nil {
		// Roll back so a failed install doesn't show up as installed.
		m.compose(context.WithoutCancel(ctx), id, "down")
		os.Remove(m.composeFile(id))
		return err
	}
	return nil
}

func (m *Manager) Start(ctx context.Context, id string) error { return m.simple(ctx, id, "start") }
func (m *Manager) Stop(ctx context.Context, id string) error  { return m.simple(ctx, id, "stop") }

// Remove stops and deletes the app's containers. App data under
// <data>/apps/<id>/data is kept so a reinstall picks it back up.
func (m *Manager) Remove(ctx context.Context, id string) error {
	if err := m.simple(ctx, id, "down"); err != nil {
		return err
	}
	return os.Remove(m.composeFile(id))
}

func (m *Manager) simple(ctx context.Context, id, verb string) error {
	if _, err := m.get(id); err != nil {
		return err
	}
	if !m.installed(id) {
		return fmt.Errorf("%s is not installed", id)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	_, err := m.compose(ctx, id, verb)
	return err
}

func (m *Manager) state(ctx context.Context, id string) string {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := m.compose(ctx, id, "ps", "-a", "--format", "json")
	if err != nil {
		return "unknown"
	}
	return parseState(out)
}

// parseState reads `docker compose ps --format json`, which prints either
// one JSON object per line or (older versions) a single JSON array.
func parseState(out []byte) string {
	type ctr struct{ State string }
	var ctrs []ctr
	trimmed := bytes.TrimSpace(out)
	if bytes.HasPrefix(trimmed, []byte("[")) {
		json.Unmarshal(trimmed, &ctrs)
	} else {
		sc := bufio.NewScanner(bytes.NewReader(trimmed))
		for sc.Scan() {
			var c ctr
			if json.Unmarshal(sc.Bytes(), &c) == nil {
				ctrs = append(ctrs, c)
			}
		}
	}
	if len(ctrs) == 0 {
		return "stopped"
	}
	for _, c := range ctrs {
		if c.State == "running" {
			return "running"
		}
	}
	return "stopped"
}

func (m *Manager) compose(ctx context.Context, id string, args ...string) ([]byte, error) {
	full := append([]string{"compose", "-p", project(id), "-f", m.composeFile(id)}, args...)
	cmd := exec.CommandContext(ctx, "docker", full...)
	cmd.Env = append(os.Environ(), "APP_DATA="+filepath.Join(m.appDir(id), "data"))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return out, fmt.Errorf("docker %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}
