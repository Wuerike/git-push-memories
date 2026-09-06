package markdown_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/Wuerike/git-push-memories/internal/markdown"
)

func TestParseSectionsRequiresAnchorImmediatelyAfterHeading(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{name: "immediate", src: "# Service\n<!-- @id: service -->\n", want: "service"},
		{name: "missing", src: "# Service\nbody\n", want: ""},
		{name: "blank line", src: "# Service\n\n<!-- @id: service -->\n", want: ""},
		{name: "noncanonical", src: "# Service\n<!-- @id:service-->\n", want: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sections, err := markdown.ParseSections([]byte(test.src))
			if err != nil {
				t.Fatal(err)
			}
			if len(sections) != 1 {
				t.Fatalf("ParseSections() returned %d sections, want 1", len(sections))
			}
			if got := sections[0].AnchorID; got != test.want {
				t.Fatalf("AnchorID = %q, want %q", got, test.want)
			}
		})
	}
}

func TestParseSectionsRejectsDuplicateIDs(t *testing.T) {
	_, err := markdown.ParseSections([]byte("# One\n<!-- @id: duplicate -->\n## Two\n<!-- @id: duplicate -->\n"))
	if !errors.Is(err, markdown.ErrDuplicateSectionID) {
		t.Fatalf("ParseSections() error = %v, want ErrDuplicateSectionID", err)
	}
}

func TestParseSectionsFindsAnchorAfterSetextHeading(t *testing.T) {
	sections, err := markdown.ParseSections([]byte("Service\r\n=======\r\n<!-- @id: service -->"))
	if err != nil {
		t.Fatal(err)
	}
	if len(sections) != 1 || sections[0].AnchorID != "service" {
		t.Fatalf("ParseSections() = %+v, want one anchored section", sections)
	}
}

func TestParseSectionsDoesNotConfuseSetextTextStartingWithHashForATX(t *testing.T) {
	sections, err := markdown.ParseSections([]byte("#5 bolt\n=======\n<!-- @id: bolt -->\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(sections) != 1 || sections[0].AnchorID != "bolt" {
		t.Fatalf("ParseSections() = %+v, want one anchored Setext section", sections)
	}
}

func TestParseSectionsHandlesUTF8CRLFFencesAndAdjacentSections(t *testing.T) {
	src := []byte("# Serviço\r\n<!-- @id: service -->\r\n```markdown\r\n## Not a section\r\n<!-- @id: not-a-section -->\r\n```\r\n## Operação\r\n<!-- @id: operation -->\r\n## Próxima\r\n<!-- @id: next -->")
	sections, err := markdown.ParseSections(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(sections) != 3 {
		t.Fatalf("ParseSections() returned %d sections, want 3", len(sections))
	}
	if sections[0].HeadingText != "Serviço" || sections[1].HeadingText != "Operação" || sections[2].HeadingText != "Próxima" {
		t.Fatalf("heading texts = [%q, %q, %q]", sections[0].HeadingText, sections[1].HeadingText, sections[2].HeadingText)
	}
	operationStart := bytes.Index(src, []byte("## Operação"))
	nextStart := bytes.Index(src, []byte("## Próxima"))
	if sections[0].ByteEnd != len(src) || sections[1].ByteStart != operationStart || sections[1].ByteEnd != nextStart || sections[2].ByteEnd != len(src) {
		t.Fatalf("section boundaries = [%d,%d) [%d,%d) [%d,%d)", sections[0].ByteStart, sections[0].ByteEnd,
			sections[1].ByteStart, sections[1].ByteEnd, sections[2].ByteStart, sections[2].ByteEnd)
	}
}
