package codex

import "embed"

// Skills contains the Codex skill instructions and invocation policies.
//
//go:embed skills
var Skills embed.FS
