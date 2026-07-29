//go:build prod
// +build prod

package main

import (
	"embed"
	"io/fs"
)

//go:embed all:webs/dist
var embeddedFiles embed.FS

var StaticFiles fs.FS = embeddedFiles
