//go:build !noweb

package main

import "embed"

//go:embed all:dist
var uiFS embed.FS

const uiEnabled = true
