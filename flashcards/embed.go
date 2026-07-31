// Package flashcards embeds the card decks so the binary is self-contained.
//
// The embed directive has to live at the module root because go:embed cannot
// reach into parent directories, which is why this file exists outside
// internal/.
package flashcards

import "embed"

// Decks holds the YAML decks baked into the binary at build time. A running
// instance can be pointed at a directory instead via DECKS_DIR — that override
// is what makes the M2 ConfigMap exercise possible.
//
//go:embed decks/*.yaml
var Decks embed.FS
