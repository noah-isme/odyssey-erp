package web

import "embed"

// Templates embeds HTML templates.
//
//go:embed templates/layouts/*.html templates/partials/*.html templates/partials/*/*.html templates/pages/*.html templates/pages/*/*.html templates/reports/*.html
var Templates embed.FS

// Static embeds static assets.
//
//go:embed static/**/*
var Static embed.FS
