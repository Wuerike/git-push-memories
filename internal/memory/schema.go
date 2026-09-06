package memory

import (
	"bytes"
	"fmt"
	"unicode/utf8"

	"github.com/Wuerike/git-push-memories/internal/markdown"
)

func validateDocument(source []byte) ([]markdown.Section, error) {
	if !utf8.Valid(source) {
		return nil, fmt.Errorf("%w: invalid UTF-8", ErrInvalidSchema)
	}
	if hasUnresolvedMerge(source) {
		return nil, fmt.Errorf("%w: unresolved merge", ErrInvalidSchema)
	}
	sections, err := markdown.ParseSections(source)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidSchema, err)
	}
	if len(sections) == 0 {
		return nil, fmt.Errorf("%w: missing heading", ErrInvalidSchema)
	}
	h1Count := 0
	for _, section := range sections {
		if section.HeadingLevel == 1 {
			h1Count++
		}
		if section.AnchorID == "" {
			return nil, fmt.Errorf("%w: missing or invalid section ID", ErrInvalidSchema)
		}
	}
	if h1Count != 1 {
		return nil, fmt.Errorf("%w: expected exactly one H1", ErrInvalidSchema)
	}
	return sections, nil
}

func hasUnresolvedMerge(source []byte) bool {
	inFence := false
	var fenceByte byte
	fenceLength := 0
	for _, line := range bytes.Split(source, []byte("\n")) {
		line = bytes.TrimSuffix(line, []byte("\r"))
		trimmed := bytes.TrimLeft(line, " ")
		indent := len(line) - len(trimmed)
		if indent <= 3 && len(trimmed) >= 3 && (trimmed[0] == '`' || trimmed[0] == '~') {
			count := 0
			for count < len(trimmed) && trimmed[count] == trimmed[0] {
				count++
			}
			if count >= 3 {
				if !inFence {
					inFence, fenceByte, fenceLength = true, trimmed[0], count
					continue
				}
				if trimmed[0] == fenceByte && count >= fenceLength {
					inFence = false
					continue
				}
			}
		}
		if inFence {
			continue
		}
		if bytes.HasPrefix(line, []byte("<<<<<<<")) || bytes.HasPrefix(line, []byte(">>>>>>>")) ||
			bytes.HasPrefix(line, []byte("<!-- @merge-conflict")) || bytes.HasPrefix(line, []byte("<!-- /@merge-conflict")) {
			return true
		}
	}
	return false
}
