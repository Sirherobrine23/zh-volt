//go:build static

package page

import "embed"

// go:embed web_src/**/*
var source embed.FS
