// Adapted from xChuCx/agent-memory internal/markdown/section_id.go at
// 51dac9bf6e72c67f6314ee3abb00515986a3fa4a (blob aa801f90ce4ab852f7771000d9e0f81b648fbff6).
// Copyright 2026 Teterichev Anton. Licensed under Apache-2.0.
// Modified by git-push-memories: automatic ID insertion was removed and anchors are parsed strictly.

package markdown

import (
	"bytes"
	"strings"
)

const (
	anchorPrefix       = "<!-- @id: "
	anchorSuffix       = " -->"
	maxSectionIDLength = 64
)

func parseAnchorLine(line []byte) (string, bool) {
	line = bytes.TrimSuffix(line, []byte("\r"))
	if !bytes.HasPrefix(line, []byte(anchorPrefix)) || !bytes.HasSuffix(line, []byte(anchorSuffix)) {
		return "", false
	}
	id := string(line[len(anchorPrefix) : len(line)-len(anchorSuffix)])
	if !validSectionID(id) {
		return "", false
	}
	return id, true
}

func validSectionID(id string) bool {
	if id == "" || len(id) > maxSectionIDLength || strings.HasPrefix(id, "-") || strings.HasSuffix(id, "-") {
		return false
	}
	previousDash := false
	for _, character := range id {
		if character == '-' {
			if previousDash {
				return false
			}
			previousDash = true
			continue
		}
		if !((character >= 'a' && character <= 'z') || (character >= '0' && character <= '9')) {
			return false
		}
		previousDash = false
	}
	return true
}
