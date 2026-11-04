package markdown_test

import (
	"bytes"
	"testing"

	"github.com/Wuerike/git-push-memories/internal/markdown"
)

func TestSectionContentExcludesHeadingAndAnchor(t *testing.T) {
	input := []byte("# Service\n<!-- @id: service -->\nIntro.\n\n## Operation\n<!-- @id: operation -->\nTimeout: 5 seconds.\n\n## Owner\n<!-- @id: owner -->\nTeam blue.\n")
	sections, err := markdown.ParseSections(input)
	if err != nil {
		t.Fatal(err)
	}
	before := append([]byte(nil), input...)
	got, err := markdown.SectionContent(input, sections[1])
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "Timeout: 5 seconds.\n\n" {
		t.Fatalf("SectionContent() = %q", got)
	}
	if !bytes.Equal(input, before) {
		t.Fatal("SectionContent() modified the source")
	}
}

func TestSectionContentPreservesCRLFAndMissingFinalNewline(t *testing.T) {
	input := []byte("# Service\r\n<!-- @id: service -->\r\nold value\r\nlast line")
	sections, err := markdown.ParseSections(input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := markdown.SectionContent(input, sections[0])
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "old value\r\nlast line" {
		t.Fatalf("SectionContent() = %q", got)
	}
}
