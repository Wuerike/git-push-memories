package proposal

import (
	"strings"
	"testing"
)

func TestUnifiedDiffPreservesLineEndingsAndMarksMissingFinalNewline(t *testing.T) {
	diff := unifiedDiff("memory.md", []byte("one\r\ntwo"), []byte("one\r\nthree"))
	for _, expected := range []string{
		"--- a/memory.md\n+++ b/memory.md\n",
		" one\r\n",
		"-two\n\\ No newline at end of file\n",
		"+three\n\\ No newline at end of file\n",
	} {
		if !strings.Contains(diff, expected) {
			t.Fatalf("unifiedDiff() = %q, missing %q", diff, expected)
		}
	}
}
