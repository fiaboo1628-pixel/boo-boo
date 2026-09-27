// Package auth handles the single admin account and cookie sessions.
package auth

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	CookieName   = "booboo_session"
	sessionTTL   = 7 * 24 * time.Hour
	pbkdf2Rounds = 210_000
)

var ErrNotSetup = errors.New("admin account not set up")

type credentials struct {
	Username string `json:"username"`
	Salt     string `json:"salt"`
	Hash     string `json:"hash"`
}

// Store keeps the admin credentials on disk and sessions in memory.
type Store struct {
	path     string
	mu       sync.Mutex
	creds    *credentials
	sessions map[string]time.Time
}

func NewStore(dataDir string) (*Store, error) {
	s := &Store{path: filepath.Join(dataDir, "auth.json"), sessions: map[string]time.Time{}}
	b, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	var c credentials
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	s.creds = &c
	return s, nil
}

func (s *Store) IsSetup() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.creds != nil
}

// Setup creates the admin account. It only works once.
func (s *Store) Setup(username, password string) error {
	if len(username) == 0 || len(password) < 8 {
		return errors.New("username required and password must be at least 8 characters")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.creds != nil {
		return errors.New("already set up")
	}
	salt := make([]byte, 16)
	rand.Read(salt)
	hash, err := pbkdf2.Key(sha256.New, password, salt, pbkdf2Rounds, 32)
	if err != nil {
		return err
	}
	c := &credentials{
		Username: username,
		Salt:     base64.StdEncoding.EncodeToString(salt),
		Hash:     base64.StdEncoding.EncodeToString(hash),
	}
	b, _ := json.MarshalIndent(c, "", "  ")
	if err := os.WriteFile(s.path, b, 0o600); err != nil {
		return err
	}
	s.creds = c
	return nil
}

// Login checks credentials and returns a new session token.
func (s *Store) Login(username, password string) (string, error) {
	s.mu.Lock()
	c := s.creds
	s.mu.Unlock()
	if c == nil {
		return "", ErrNotSetup
	}
	salt, _ := base64.StdEncoding.DecodeString(c.Salt)
	want, _ := base64.StdEncoding.DecodeString(c.Hash)
	got, err := pbkdf2.Key(sha256.New, password, salt, pbkdf2Rounds, 32)
	if err != nil {
		return "", err
	}
	userOK := subtle.ConstantTimeCompare([]byte(username), []byte(c.Username)) == 1
	passOK := subtle.ConstantTimeCompare(got, want) == 1
	if !userOK || !passOK {
		return "", errors.New("invalid username or password")
	}
	tok := make([]byte, 32)
	rand.Read(tok)
	token := base64.RawURLEncoding.EncodeToString(tok)
	s.mu.Lock()
	s.sessions[token] = time.Now().Add(sessionTTL)
	s.mu.Unlock()
	return token, nil
}

func (s *Store) Logout(token string) {
	s.mu.Lock()
	delete(s.sessions, token)
	s.mu.Unlock()
}

func (s *Store) Valid(token string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	exp, ok := s.sessions[token]
	if !ok {
		return false
	}
	if time.Now().After(exp) {
		delete(s.sessions, token)
		return false
	}
	return true
}

// Require wraps a handler so it only runs for logged-in requests.
func (s *Store) Require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(CookieName)
		if err != nil || !s.Valid(c.Value) {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func SetCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name: CookieName, Value: token, Path: "/", HttpOnly: true,
		SameSite: http.SameSiteStrictMode, MaxAge: int(sessionTTL.Seconds()),
	})
}

func ClearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: CookieName, Value: "", Path: "/", MaxAge: -1})
}
