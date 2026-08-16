package webassets

import "embed"

// FS contains the complete local UI served by both independently started modes.
//
//go:embed index.html style.css app.js
var FS embed.FS
