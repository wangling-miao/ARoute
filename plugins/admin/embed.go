package admin

import "embed"

//go:embed all:dist
var adminDistFS embed.FS
