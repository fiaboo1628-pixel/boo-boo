// Package server wires the HTTP API and the embedded web UI together.
package server

import (
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"time"

	"github.com/fiaboo1628-pixel/boo-boo/internal/apps"
	"github.com/fiaboo1628-pixel/boo-boo/internal/auth"
	"github.com/fiaboo1628-pixel/boo-boo/internal/bluetooth"
	"github.com/fiaboo1628-pixel/boo-boo/internal/music"
	"github.com/fiaboo1628-pixel/boo-boo/internal/sysinfo"
)

type Server struct {
	Auth    *auth.Store
	Apps    *apps.Manager
	DataDir string
	Music   *music.Player
	Web     fs.FS
}

func (s *Server) Handler() http.Handler {
	api := http.NewServeMux()
	api.HandleFunc("GET /api/system", s.system)
	api.HandleFunc("GET /api/apps", s.listApps)
	api.HandleFunc("POST /api/apps/{id}/{action}", s.appAction)
	api.HandleFunc("GET /api/bluetooth", s.btStatus)
	api.HandleFunc("POST /api/bluetooth/{action}", s.btAction)
	api.HandleFunc("GET /api/music", s.musicStatus)
	api.HandleFunc("GET /api/music/tracks", s.musicTracks)
	api.HandleFunc("POST /api/music/{action}", s.musicAction)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/auth/status", s.authStatus)
	mux.HandleFunc("POST /api/auth/setup", s.setup)
	mux.HandleFunc("POST /api/auth/login", s.login)
	mux.HandleFunc("POST /api/auth/logout", s.logout)
	mux.Handle("/api/", s.Auth.Require(api))
	mux.Handle("/", http.FileServerFS(s.Web))
	return mux
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, map[string]string{"error": err.Error()})
}

type creds struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func decode(w http.ResponseWriter, r *http.Request, v any) error {
	return json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(v)
}

func (s *Server) authStatus(w http.ResponseWriter, r *http.Request) {
	loggedIn := false
	if c, err := r.Cookie(auth.CookieName); err == nil {
		loggedIn = s.Auth.Valid(c.Value)
	}
	writeJSON(w, 200, map[string]bool{"setup": s.Auth.IsSetup(), "logged_in": loggedIn})
}

func (s *Server) setup(w http.ResponseWriter, r *http.Request) {
	var c creds
	if err := decode(w, r, &c); err != nil {
		writeErr(w, 400, err)
		return
	}
	if err := s.Auth.Setup(c.Username, c.Password); err != nil {
		writeErr(w, 400, err)
		return
	}
	s.startSession(w, c)
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var c creds
	if err := decode(w, r, &c); err != nil {
		writeErr(w, 400, err)
		return
	}
	s.startSession(w, c)
}

func (s *Server) startSession(w http.ResponseWriter, c creds) {
	tok, err := s.Auth.Login(c.Username, c.Password)
	if err != nil {
		writeErr(w, 401, err)
		return
	}
	auth.SetCookie(w, tok)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(auth.CookieName); err == nil {
		s.Auth.Logout(c.Value)
	}
	auth.ClearCookie(w)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) system(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, sysinfo.Collect(s.DataDir))
}

func (s *Server) listApps(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.Apps.List(r.Context()))
}

func (s *Server) appAction(w http.ResponseWriter, r *http.Request) {
	id, action := r.PathValue("id"), r.PathValue("action")
	var err error
	switch action {
	case "install":
		err = s.Apps.Install(r.Context(), id)
	case "start":
		err = s.Apps.Start(r.Context(), id)
	case "stop":
		err = s.Apps.Stop(r.Context(), id)
	case "remove":
		err = s.Apps.Remove(r.Context(), id)
	default:
		writeJSON(w, 404, map[string]string{"error": "unknown action"})
		return
	}
	if err != nil {
		log.Printf("app %s %s: %v", id, action, err)
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) btStatus(w http.ResponseWriter, r *http.Request) {
	st, err := bluetooth.Get(r.Context())
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 200, st)
}

func (s *Server) btAction(w http.ResponseWriter, r *http.Request) {
	var body struct {
		MAC string `json:"mac"`
	}
	if r.ContentLength != 0 {
		if err := decode(w, r, &body); err != nil {
			writeErr(w, 400, err)
			return
		}
	}
	var err error
	switch r.PathValue("action") {
	case "power":
		err = bluetooth.PowerOn(r.Context())
	case "scan":
		err = bluetooth.Scan(r.Context(), 10*time.Second)
	case "connect":
		err = bluetooth.Connect(r.Context(), body.MAC)
	case "disconnect":
		err = bluetooth.Disconnect(r.Context(), body.MAC)
	case "forget":
		err = bluetooth.Forget(r.Context(), body.MAC)
	default:
		writeJSON(w, 404, map[string]string{"error": "unknown action"})
		return
	}
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) musicStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.Music.Status())
}

func (s *Server) musicTracks(w http.ResponseWriter, r *http.Request) {
	tracks, err := s.Music.Tracks()
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 200, tracks)
}

func (s *Server) musicAction(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Track  string  `json:"track"`
		Volume float64 `json:"volume"`
	}
	if r.ContentLength != 0 {
		if err := decode(w, r, &body); err != nil {
			writeErr(w, 400, err)
			return
		}
	}
	var err error
	switch r.PathValue("action") {
	case "play":
		err = s.Music.Play(body.Track)
	case "pause":
		err = s.Music.TogglePause()
	case "next":
		err = s.Music.Next()
	case "prev":
		err = s.Music.Prev()
	case "stop":
		s.Music.Stop()
	case "volume":
		err = s.Music.SetVolume(body.Volume)
	default:
		writeJSON(w, 404, map[string]string{"error": "unknown action"})
		return
	}
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}
