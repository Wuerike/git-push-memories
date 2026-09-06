package markdown_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/Wuerike/git-push-memories/internal/markdown"
)

func TestReplaceSectionContentGolden(t *testing.T) {
	input := readFixture(t, "replace.input.md")
	want := readFixture(t, "replace.golden.md")
	sections, err := markdown.ParseSections(input)
	if err != nil {
		t.Fatal(err)
	}
	target, ok := markdown.FindByID(sections, "checkout-api-operation")
	if !ok {
		t.Fatal("target section not found")
	}

	got, err := markdown.ReplaceSectionContent(input, *target, []byte("Timeout: 3 segundos.\nOwner: team-b.\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("ReplaceSectionContent() mismatch\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestReplaceSectionContentPreservesCRLFAndMissingFinalNewline(t *testing.T) {
	input := []byte("# Service\r\n<!-- @id: service -->\r\nold value")
	sections, err := markdown.ParseSections(input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := markdown.ReplaceSectionContent(input, sections[0], []byte("new value"))
	if err != nil {
		t.Fatal(err)
	}
	want := []byte("# Service\r\n<!-- @id: service -->\r\nnew value")
	if !bytes.Equal(got, want) {
		t.Fatalf("ReplaceSectionContent() = %q, want %q", got, want)
	}
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return content
}
