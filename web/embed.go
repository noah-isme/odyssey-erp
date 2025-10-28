package web

import "embed"

// Templates embeds HTML templates.
//
//go:embed templates/**/*.html
var Templates embed.FS

// Static embeds static assets.
//
//go:embed static/**/*
var Static embed.FS
