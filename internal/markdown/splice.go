// Adapted from xChuCx/agent-memory internal/markdown/splice.go at
// 51dac9bf6e72c67f6314ee3abb00515986a3fa4a (blob 36c81a926fbd97d89039ffa03d963a1ecc501c12).
// Copyright 2026 Teterichev Anton. Licensed under Apache-2.0.
// Modified by git-push-memories: replacement is limited to the section body.

package markdown

import (
	"fmt"
	"sort"
)

// SpliceOp describes a byte-range substitution against the original source.
type SpliceOp struct {
	ByteStart   int
	ByteEnd     int
	Replacement []byte
}

// Splice applies non-overlapping byte-range substitutions to src.
func Splice(src []byte, ops []SpliceOp) ([]byte, error) {
	if len(ops) == 0 {
		return append([]byte(nil), src...), nil
	}

	sorted := append([]SpliceOp(nil), ops...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].ByteStart < sorted[j].ByteStart
	})
	if err := validateOps(src, sorted); err != nil {
		return nil, err
	}

	resultLength := len(src)
	for _, op := range sorted {
		resultLength += len(op.Replacement) - (op.ByteEnd - op.ByteStart)
	}
	out := make([]byte, 0, max(resultLength, 0))
	cursor := 0
	for _, op := range sorted {
		out = append(out, src[cursor:op.ByteStart]...)
		out = append(out, op.Replacement...)
		cursor = op.ByteEnd
	}
	out = append(out, src[cursor:]...)
	return out, nil
}

func validateOps(src []byte, ops []SpliceOp) error {
	for i, op := range ops {
		if op.ByteStart < 0 {
			return fmt.Errorf("splice op %d has negative start", i)
		}
		if op.ByteEnd > len(src) {
			return fmt.Errorf("splice op %d exceeds source", i)
		}
		if op.ByteStart > op.ByteEnd {
			return fmt.Errorf("splice op %d has inverted bounds", i)
		}
		if i > 0 && ops[i-1].ByteEnd > op.ByteStart {
			return fmt.Errorf("splice op %d overlaps previous op", i)
		}
	}
	return nil
}

// ReplaceSectionContent replaces only the body after the heading and anchor.
func ReplaceSectionContent(src []byte, section Section, replacement []byte) ([]byte, error) {
	bodyStart, err := sectionBodyStart(src, section)
	if err != nil {
		return nil, err
	}
	return Splice(src, []SpliceOp{{
		ByteStart:   bodyStart,
		ByteEnd:     section.ByteEnd,
		Replacement: replacement,
	}})
}

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
