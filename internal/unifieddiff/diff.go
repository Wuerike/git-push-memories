package unifieddiff

import (
	"fmt"
	"strings"
)

type line struct {
	text       string
	terminated bool
}

func Between(filePath string, before, after []byte) string {
	oldLines := splitLines(string(before))
	newLines := splitLines(string(after))
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

	var output strings.Builder
	fmt.Fprintf(&output, "--- a/%s\n+++ b/%s\n@@ -%d,%d +%d,%d @@\n", filePath, filePath, contextStart+1, oldContextEnd-contextStart, contextStart+1, newContextEnd-contextStart)
	for _, current := range oldLines[contextStart:prefix] {
		writeLine(&output, ' ', current)
	}
	for _, current := range oldLines[prefix:oldChangeEnd] {
		writeLine(&output, '-', current)
	}
	for _, current := range newLines[prefix:newChangeEnd] {
		writeLine(&output, '+', current)
	}
	for _, current := range oldLines[oldChangeEnd:oldContextEnd] {
		writeLine(&output, ' ', current)
	}
	return output.String()
}

func Create(filePath string, contents []byte) string {
	lines := splitLines(string(contents))
	var output strings.Builder
	fmt.Fprintf(&output, "--- /dev/null\n+++ b/%s\n@@ -0,0 +1,%d @@\n", filePath, len(lines))
	for _, current := range lines {
		writeLine(&output, '+', current)
	}
	return output.String()
}

func splitLines(value string) []line {
	if value == "" {
		return nil
	}
	parts := strings.SplitAfter(value, "\n")
	if parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	lines := make([]line, len(parts))
	for i, part := range parts {
		lines[i] = line{text: strings.TrimSuffix(part, "\n"), terminated: strings.HasSuffix(part, "\n")}
	}
	return lines
}

func writeLine(output *strings.Builder, prefix byte, current line) {
	output.WriteByte(prefix)
	output.WriteString(current.text)
	output.WriteByte('\n')
	if !current.terminated {
		output.WriteString("\\ No newline at end of file\n")
	}
}
