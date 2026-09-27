// Package music plays audio files from a folder on the host with mpv, so
// sound goes to the host's default output (e.g. a connected Bluetooth
// speaker through PipeWire).
package music

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

var audioExt = map[string]bool{".mp3": true, ".flac": true, ".ogg": true, ".opus": true, ".m4a": true, ".wav": true, ".aac": true}

const maxTracks = 5000

type Player struct {
	Dir  string // music library folder
	User string // run mpv as this user so it reaches their PipeWire session; empty = current user

	mu     sync.Mutex
	cmd    *exec.Cmd
	socket string
}

type Status struct {
	Available bool    `json:"available"`
	Dir       string  `json:"dir"`
	Playing   bool    `json:"playing"`
	Paused    bool    `json:"paused"`
	Title     string  `json:"title"`
	Volume    float64 `json:"volume"`
}

func Available() bool {
	_, err := exec.LookPath("mpv")
	return err == nil
}

// Tracks lists audio files under Dir, as paths relative to it.
func (p *Player) Tracks() ([]string, error) {
	var out []string
	err := filepath.WalkDir(p.Dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if path == p.Dir {
				return err
			}
			return nil
		}
		if d.IsDir() && strings.HasPrefix(d.Name(), ".") && path != p.Dir {
			return filepath.SkipDir
		}
		if !d.IsDir() && audioExt[strings.ToLower(filepath.Ext(path))] {
			rel, _ := filepath.Rel(p.Dir, path)
			out = append(out, rel)
			if len(out) >= maxTracks {
				return fs.SkipAll
			}
		}
		return nil
	})
	if errors.Is(err, fs.ErrNotExist) {
		return []string{}, nil
	}
	sort.Strings(out)
	return out, err
}

// resolve turns a relative track path into an absolute one inside Dir.
func (p *Player) resolve(rel string) (string, error) {
	abs := filepath.Join(p.Dir, filepath.Clean("/"+rel))
	if !audioExt[strings.ToLower(filepath.Ext(abs))] {
		return "", fmt.Errorf("not an audio file: %q", rel)
	}
	if _, err := os.Stat(abs); err != nil {
		return "", err
	}
	return abs, nil
}

// Play starts playing from track start through the rest of the library
// (in order), or the whole library shuffled when start is empty.
func (p *Player) Play(start string) error {
	tracks, err := p.Tracks()
	if err != nil {
		return err
	}
	if len(tracks) == 0 {
		return fmt.Errorf("no audio files in %s", p.Dir)
	}
	args := []string{"--no-video", "--no-terminal", "--idle=no"}
	var files []string
	if start == "" {
		args = append(args, "--shuffle")
		files = tracks
	} else {
		if _, err := p.resolve(start); err != nil {
			return err
		}
		i := sort.SearchStrings(tracks, start)
		if i == len(tracks) || tracks[i] != start {
			return fmt.Errorf("track not in library: %q", start)
		}
		files = tracks[i:]
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	p.stopLocked()

	sockDir, err := os.MkdirTemp("", "booboo-mpv-")
	if err != nil {
		return err
	}
	p.socket = filepath.Join(sockDir, "mpv.sock")
	args = append(args, "--input-ipc-server="+p.socket, "--")
	for _, f := range files {
		args = append(args, filepath.Join(p.Dir, f))
	}
	cmd := exec.Command("mpv", args...)
	if err := p.asUser(cmd, sockDir); err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	p.cmd = cmd
	go func() {
		cmd.Wait()
		os.RemoveAll(sockDir)
		p.mu.Lock()
		if p.cmd == cmd {
			p.cmd = nil
		}
		p.mu.Unlock()
	}()
	return nil
}

// asUser makes mpv run as p.User with that user's XDG_RUNTIME_DIR, which is
// where the PipeWire/PulseAudio socket lives.
func (p *Player) asUser(cmd *exec.Cmd, sockDir string) error {
	if p.User == "" {
		return nil
	}
	u, err := user.Lookup(p.User)
	if err != nil {
		return err
	}
	uid, _ := strconv.Atoi(u.Uid)
	gid, _ := strconv.Atoi(u.Gid)
	if err := os.Chown(sockDir, uid, gid); err != nil {
		return err
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid)}}
	cmd.Env = append(os.Environ(), "HOME="+u.HomeDir, "USER="+u.Username,
		"XDG_RUNTIME_DIR=/run/user/"+u.Uid)
	return nil
}

func (p *Player) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stopLocked()
}

func (p *Player) stopLocked() {
	if p.cmd != nil && p.cmd.Process != nil {
		p.cmd.Process.Kill()
	}
	p.cmd = nil
}

func (p *Player) running() (string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.socket, p.cmd != nil
}

// ipc sends one command to mpv's JSON IPC socket and returns its data field.
func (p *Player) ipc(command ...any) (json.RawMessage, error) {
	sock, ok := p.running()
	if !ok {
		return nil, errors.New("nothing is playing")
	}
	conn, err := net.DialTimeout("unix", sock, 2*time.Second)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(2 * time.Second))
	req, _ := json.Marshal(map[string]any{"command": command})
	if _, err := conn.Write(append(req, '\n')); err != nil {
		return nil, err
	}
	sc := bufio.NewScanner(conn)
	for sc.Scan() {
		var resp struct {
			Event string          `json:"event"`
			Error string          `json:"error"`
			Data  json.RawMessage `json:"data"`
		}
		if json.Unmarshal(sc.Bytes(), &resp) != nil || resp.Event != "" {
			continue // skip async events
		}
		if resp.Error != "success" {
			return nil, errors.New(resp.Error)
		}
		return resp.Data, nil
	}
	return nil, errors.New("no reply from mpv")
}

func (p *Player) TogglePause() error {
	_, err := p.ipc("cycle", "pause")
	return err
}

func (p *Player) Next() error {
	_, err := p.ipc("playlist-next", "force")
	return err
}

func (p *Player) Prev() error {
	_, err := p.ipc("playlist-prev", "weak")
	return err
}

func (p *Player) SetVolume(v float64) error {
	if v < 0 || v > 100 {
		return errors.New("volume must be 0-100")
	}
	_, err := p.ipc("set_property", "volume", v)
	return err
}

func (p *Player) Status() Status {
	st := Status{Available: Available(), Dir: p.Dir}
	if _, ok := p.running(); !ok {
		return st
	}
	st.Playing = true
	if d, err := p.ipc("get_property", "media-title"); err == nil {
		json.Unmarshal(d, &st.Title)
	}
	if d, err := p.ipc("get_property", "pause"); err == nil {
		json.Unmarshal(d, &st.Paused)
	}
	if d, err := p.ipc("get_property", "volume"); err == nil {
		json.Unmarshal(d, &st.Volume)
	}
	return st
}
