// Package asset embeds the brand SVG so the binary has no runtime
// file dependency.
package asset

import _ "embed"

// LogoSVG is the Automergent wordmark, embedded verbatim from the
// original Fabric.js export.
//
//go:embed logo.svg
var LogoSVG []byte
