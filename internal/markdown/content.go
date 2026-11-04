// Adapted from xChuCx/agent-memory internal/markdown/splice.go at
// 51dac9bf6e72c67f6314ee3abb00515986a3fa4a (blob 36c81a926fbd97d89039ffa03d963a1ecc501c12).
// Copyright 2026 Teterichev Anton. Licensed under Apache-2.0.
// Modified by git-push-memories: extracts section bodies without writing.

package markdown

import "fmt"

// SectionContent returns a copy of the section body without its heading or anchor.
func SectionContent(src []byte, section Section) ([]byte, error) {
	bodyStart, err := sectionBodyStart(src, section)
	if err != nil {
		return nil, err
	}
	if section.ByteEnd < bodyStart || section.ByteEnd > len(src) {
		return nil, fmt.Errorf("section has invalid body bounds")
	}
	return append([]byte(nil), src[bodyStart:section.ByteEnd]...), nil
}

func sectionBodyStart(src []byte, section Section) (int, error) {
	if section.ByteStart < 0 || section.HeadingEnd < section.ByteStart || section.HeadingEnd > len(src) {
		return 0, fmt.Errorf("section has invalid heading bounds")
	}
	bodyStart := section.HeadingEnd
	if section.AnchorID != "" {
		bodyStart = headingLineEnd(src, bodyStart)
	}
	return bodyStart, nil
}
