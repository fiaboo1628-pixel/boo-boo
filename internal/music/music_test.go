package music

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestTracksAndResolve(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"b.mp3", "a/x.FLAC", "notes.txt", ".hidden/y.mp3"} {
		os.MkdirAll(filepath.Join(dir, filepath.Dir(f)), 0o755)
		os.WriteFile(filepath.Join(dir, f), nil, 0o644)
	}
	p := &Player{Dir: dir}
	got, err := p.Tracks()
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"a/x.FLAC", "b.mp3"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Tracks() = %v, want %v", got, want)
	}
	if _, err := p.resolve("../../etc/passwd"); err == nil {
		t.Fatal("expected error for path outside library")
	}
	if abs, err := p.resolve("../b.mp3"); err != nil || abs != filepath.Join(dir, "b.mp3") {
		t.Fatalf("resolve kept inside dir: %v %v", abs, err)
	}
}

func TestMissingLibrary(t *testing.T) {
	p := &Player{Dir: filepath.Join(t.TempDir(), "nope")}
	got, err := p.Tracks()
	if err != nil || len(got) != 0 {
		t.Fatalf("got %v, %v", got, err)
	}
}
