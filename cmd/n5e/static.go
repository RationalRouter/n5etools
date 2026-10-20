package main

import "embed"

//go:embed static/css static/js static/img static/vendor
var staticFS embed.FS
