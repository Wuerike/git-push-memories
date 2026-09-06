package proposal

import (
	"fmt"
	"strings"
)

type diffLine struct {
	text       string
	terminated bool
}

func unifiedDiff(filePath string, before, after []byte) string {
	oldLines := splitDiffLines(string(before))
	newLines := splitDiffLines(string(after))
	prefix := 0
	for prefix < len(oldLines) && prefix < len(newLines) && oldLines[prefix] == newLines[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < len(oldLines)-prefix && suffix < len(newLines)-prefix && oldLines[len(oldLines)-1-suffix] == newLines[len(newLines)-1-suffix] {
		suffix++
	}
	oldChangeEnd := len(oldLines) - suffix
	newChangeEnd := len(newLines) - suffix
	contextStart := max(0, prefix-3)
	oldContextEnd := min(len(oldLines), oldChangeEnd+3)
	newContextEnd := min(len(newLines), newChangeEnd+3)
	oldCount := oldContextEnd - contextStart
	newCount := newContextEnd - contextStart

	var output strings.Builder
	fmt.Fprintf(&output, "--- a/%s\n+++ b/%s\n@@ -%d,%d +%d,%d @@\n", filePath, filePath, contextStart+1, oldCount, contextStart+1, newCount)
	for _, line := range oldLines[contextStart:prefix] {
		writeDiffLine(&output, ' ', line)
	}
	for _, line := range oldLines[prefix:oldChangeEnd] {
		writeDiffLine(&output, '-', line)
	}
	for _, line := range newLines[prefix:newChangeEnd] {
		writeDiffLine(&output, '+', line)
	}
	for _, line := range oldLines[oldChangeEnd:oldContextEnd] {
		writeDiffLine(&output, ' ', line)
	}
	return output.String()
}

func splitDiffLines(value string) []diffLine {
	if value == "" {
		return nil
	}
	parts := strings.SplitAfter(value, "\n")
	if parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	lines := make([]diffLine, len(parts))
	for i, part := range parts {
		lines[i] = diffLine{text: strings.TrimSuffix(part, "\n"), terminated: strings.HasSuffix(part, "\n")}
	}
	return lines
}

func writeDiffLine(output *strings.Builder, prefix byte, line diffLine) {
	output.WriteByte(prefix)
	output.WriteString(line.text)
	output.WriteByte('\n')
	if !line.terminated {
		output.WriteString("\\ No newline at end of file\n")
	}
}
