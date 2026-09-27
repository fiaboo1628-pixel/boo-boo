// Package catalog embeds the built-in app store templates.
//
// Each app lives in its own folder with an app.json (metadata) and a
// docker-compose.yml. Compose files may use ${APP_DATA}, which Boo Boo
// sets to a persistent per-app folder on the host.
package catalog

import "embed"

//go:embed */app.json */docker-compose.yml
var FS embed.FS
