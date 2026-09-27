// Command booboo runs the Boo Boo home-server dashboard.
package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/fiaboo1628-pixel/boo-boo/catalog"
	"github.com/fiaboo1628-pixel/boo-boo/internal/apps"
	"github.com/fiaboo1628-pixel/boo-boo/internal/auth"
	"github.com/fiaboo1628-pixel/boo-boo/internal/server"
	"github.com/fiaboo1628-pixel/boo-boo/web"
)

func main() {
	addr := flag.String("addr", ":8080", "address to listen on")
	dataDir := flag.String("data", "/var/lib/booboo", "folder for settings and app data")
	flag.Parse()

	if err := os.MkdirAll(*dataDir, 0o755); err != nil {
		log.Fatal(err)
	}
	authStore, err := auth.NewStore(*dataDir)
	if err != nil {
		log.Fatal(err)
	}
	mgr, err := apps.NewManager(catalog.FS, *dataDir)
	if err != nil {
		log.Fatal(err)
	}
	srv := &server.Server{Auth: authStore, Apps: mgr, DataDir: *dataDir, Web: web.FS}

	httpSrv := &http.Server{
		Addr:              *addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("Boo Boo listening on %s (data: %s)", *addr, *dataDir)
	log.Fatal(httpSrv.ListenAndServe())
}
