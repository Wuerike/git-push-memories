// Adapted from xChuCx/agent-memory internal/markdown/parse.go at
// 51dac9bf6e72c67f6314ee3abb00515986a3fa4a (blob e4cde2d405d112a67b607f0e0f8a3747b81a76e3).
// Copyright 2026 Teterichev Anton. Licensed under Apache-2.0.
// Modified by git-push-memories: anchors are strict, duplicate IDs fail parsing,
// and frontmatter is excluded from section identity.

// Package markdown implements byte-preserving Markdown parsing and editing.
package markdown

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// ErrDuplicateSectionID reports an anchor ID used by more than one section.
var ErrDuplicateSectionID = errors.New("duplicate section ID")

// Section represents a Markdown section located in source bytes.
type Section struct {
	HeadingText  string
	HeadingLevel int
	AnchorID     string
	ByteStart    int
	HeadingEnd   int
	ByteEnd      int
	ContentHash  string
}

// ParseSections returns sections with byte offsets in document order.
func ParseSections(src []byte) ([]Section, error) {
	md := goldmark.New()
	parseSource := maskFrontmatter(src)
	doc := md.Parser().Parse(text.NewReader(parseSource))

	type headingInfo struct {
		text       string
		level      int
		byteStart  int
		headingEnd int
		anchorID   string
	}
	var headings []headingInfo

	if err := ast.Walk(doc, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		heading, ok := node.(*ast.Heading)
		if !ok {
			return ast.WalkContinue, nil
		}
		lines := heading.Lines()
		if lines == nil || lines.Len() == 0 {
			return ast.WalkContinue, nil
		}
		title := extractText(heading, src)
		byteStart := lineStart(src, lines.At(0).Start)
		headingEnd := headingSyntaxEnd(src, heading, byteStart)
		headings = append(headings, headingInfo{
			text:       title,
			level:      heading.Level,
			byteStart:  byteStart,
			headingEnd: headingEnd,
		})
		return ast.WalkContinue, nil
	}); err != nil {
		return nil, fmt.Errorf("walk Markdown AST: %w", err)
	}

	seenIDs := make(map[string]struct{}, len(headings))
	for i := range headings {
		headings[i].anchorID = findAnchorID(src, headings[i].headingEnd)
		if headings[i].anchorID == "" {
			continue
		}
		if _, duplicate := seenIDs[headings[i].anchorID]; duplicate {
			return nil, fmt.Errorf("%w %q", ErrDuplicateSectionID, headings[i].anchorID)
		}
		seenIDs[headings[i].anchorID] = struct{}{}
	}

	sections := make([]Section, len(headings))
	for i, heading := range headings {
		end := len(src)
		for j := i + 1; j < len(headings); j++ {
			if headings[j].level <= heading.level {
				end = headings[j].byteStart
				break
			}
		}
		sections[i] = Section{
			HeadingText:  heading.text,
			HeadingLevel: heading.level,
			AnchorID:     heading.anchorID,
			ByteStart:    heading.byteStart,
			HeadingEnd:   heading.headingEnd,
			ByteEnd:      end,
			ContentHash:  hashBytes(src[heading.byteStart:end]),
		}
	}
	return sections, nil
}

func maskFrontmatter(src []byte) []byte {
	firstEnd := headingLineEnd(src, 0)
	if !isFrontmatterDelimiter(src[:firstEnd]) {
		return src
	}
	for lineStart := firstEnd; lineStart < len(src); {
		lineEnd := headingLineEnd(src, lineStart)
		if isFrontmatterDelimiter(src[lineStart:lineEnd]) || isFrontmatterEnd(src[lineStart:lineEnd]) {
			masked := append([]byte(nil), src...)
			for i := 0; i < lineEnd; i++ {
				if masked[i] != '\n' && masked[i] != '\r' {
					masked[i] = ' '
				}
			}
			return masked
		}
		lineStart = lineEnd
	}
	return src
}

func isFrontmatterDelimiter(line []byte) bool {
	return string(bytes.TrimSuffix(bytes.TrimSuffix(line, []byte("\n")), []byte("\r"))) == "---"
}

func isFrontmatterEnd(line []byte) bool {
	return string(bytes.TrimSuffix(bytes.TrimSuffix(line, []byte("\n")), []byte("\r"))) == "..."
}

// FindByID returns the section with id, if one exists.
func FindByID(sections []Section, id string) (*Section, bool) {
	for i := range sections {
		if sections[i].AnchorID == id {
			return &sections[i], true
		}
	}
	return nil, false
}

func hashBytes(value []byte) string {
	sum := sha256.Sum256(value)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func extractText(heading *ast.Heading, src []byte) string {
	var result bytes.Buffer
	_ = ast.Walk(heading, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch value := node.(type) {
		case *ast.Text:
			result.Write(value.Segment.Value(src))
		case *ast.String:
			result.Write(value.Value)
		}
		return ast.WalkContinue, nil
	})
	return result.String()
}

func lineStart(src []byte, offset int) int {
	if offset > len(src) {
		offset = len(src)
	}
	for offset > 0 && src[offset-1] != '\n' {
		offset--
	}
	return offset
}

// findAnchorID accepts an anchor only on the line immediately after a heading.
func findAnchorID(src []byte, headingEnd int) string {
	if headingEnd >= len(src) {
		return ""
	}
	lineEnd := bytes.IndexByte(src[headingEnd:], '\n')
	if lineEnd < 0 {
		lineEnd = len(src) - headingEnd
	}
	id, _ := parseAnchorLine(src[headingEnd : headingEnd+lineEnd])
	return id
}

func headingSyntaxEnd(src []byte, heading *ast.Heading, byteStart int) int {
	firstLineEnd := headingLineEnd(src, byteStart)
	if isATXHeadingLine(src[byteStart:firstLineEnd]) {
		return firstLineEnd
	}

	lastLine := heading.Lines().At(heading.Lines().Len() - 1)
	underlineStart := headingLineEnd(src, lineStart(src, lastLine.Start))
	return headingLineEnd(src, underlineStart)
}

func isATXHeadingLine(line []byte) bool {
	line = bytes.TrimSuffix(bytes.TrimSuffix(line, []byte("\n")), []byte("\r"))
	indent := 0
	for indent < len(line) && indent < 4 && line[indent] == ' ' {
		indent++
	}
	if indent > 3 {
		return false
	}
	hashes := 0
	for indent+hashes < len(line) && line[indent+hashes] == '#' {
		hashes++
	}
	if hashes == 0 || hashes > 6 {
		return false
	}
	next := indent + hashes
	return next == len(line) || line[next] == ' ' || line[next] == '\t'
}

func headingLineEnd(src []byte, byteStart int) int {
	index := byteStart
	for index < len(src) && src[index] != '\n' {
		index++
	}
	if index < len(src) {
		index++
	}
	return index
}
