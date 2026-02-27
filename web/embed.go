package web

import "embed"

//go:embed static/*
var FS embed.FS
