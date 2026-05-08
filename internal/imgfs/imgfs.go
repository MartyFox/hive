// Package imgfs exposes the embedded Containerfile build contexts.
package imgfs

import "embed"

// FS holds the embedded images/ directory tree.
// Access files with FS.ReadFile("images/base/Containerfile") etc.
//
//go:embed images
var FS embed.FS
