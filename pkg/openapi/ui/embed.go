package ui

import "embed"

//go:embed index.html app.css app.js
var assets embed.FS
