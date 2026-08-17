package main

import "embed"

//go:embed static/css static/js static/img
var staticFS embed.FS
