package memory

import (
	"errors"
	"os"
	"strings"
	"testing"
)

const documentPath = "memories/domains/time-camada-de-leitura.md"

const documentBaseline = "# Wuerike é do time da camada de leitura\n<!-- @id: time-camada-de-leitura -->\n\n## Overview\n<!-- @id: time-camada-de-leitura-overview -->\nO usuário se chama Wuerike e faz parte do time da camada de leitura.\n"

func TestPrepareDocumentInjectsIDsAndDropsOldSections(t *testing.T) {
	golden, err := os.ReadFile("testdata/page-rewrite/team.golden.md")
	if err != nil {
		t.Fatal(err)
	}
	got, err := PrepareDocument(documentPath, []byte(documentBaseline), []byte("# Time da camada de leitura\n\n## Membros\n- Wuerike\n- Guilherme\n"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(golden) {
		t.Fatalf("PrepareDocument() =\n%q\nwant\n%q", got, golden)
	}
}

func TestPrepareDocumentKeepsExistingMarkers(t *testing.T) {
	replacement := "# Novo título\n<!-- @id: time-camada-de-leitura -->\nIntro.\n\n## Resumo\n<!-- @id: time-camada-de-leitura-overview -->\nTexto novo.\n"
	got, err := PrepareDocument(documentPath, []byte(documentBaseline), []byte(replacement))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != replacement {
		t.Fatalf("PrepareDocument() = %q, want %q", got, replacement)
	}
}

func TestPrepareDocumentRejectsInvalidInput(t *testing.T) {
	long := "# T\n\n## " + strings.Repeat("palavra ", 10) + "\nx\n"
	cases := []struct {
		name        string
		replacement string
		want        error
	}{
		{"h1 id mismatch", "# T\n<!-- @id: outra-pagina -->\nx\n", ErrInvalidID},
		{"two h1", "# T\n\n# U\nx\n", ErrInvalidSchema},
		{"malformed marker", "# T\n\n## A\n<!-- @id: Bad ID -->\nx\n", ErrInvalidSchema},
		{"duplicate generated id", "# T\n\n## A b\nx\n\n## A-b\ny\n", ErrSectionExists},
		{"id too long", long, ErrGeneratedIDTooLong},
		{"merge conflict", "# T\n\n## A\n<<<<<<< HEAD\nx\n=======\ny\n>>>>>>> branch\n", ErrInvalidSchema},
		{"over 128 KiB", "# T\n\n## A\n" + strings.Repeat("x", 128*1024) + "\n", ErrInvalidSchema},
		{"empty replacement", "", ErrInvalidSchema},
		{"no heading", "só texto\n", ErrInvalidSchema},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := PrepareDocument(documentPath, []byte(documentBaseline), []byte(tc.replacement)); !errors.Is(err, tc.want) {
				t.Fatalf("PrepareDocument() error = %v, want %v", err, tc.want)
			}
		})
	}
}

const documentFrontmatter = "---\ntags: [a, b]\n---\n"

func TestPrepareDocumentPreservesBaselineFrontmatter(t *testing.T) {
	baseline := documentFrontmatter + documentBaseline
	got, err := PrepareDocument(documentPath, []byte(baseline), []byte("# Time\n\n## Membros\n- A\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := documentFrontmatter + "# Time\n<!-- @id: time-camada-de-leitura -->\n\n## Membros\n<!-- @id: time-camada-de-leitura-membros -->\n- A\n"
	if string(got) != want {
		t.Fatalf("PrepareDocument() = %q, want %q", got, want)
	}
}

func TestPrepareDocumentAcceptsIdenticalFrontmatterInReplacement(t *testing.T) {
	baseline := documentFrontmatter + documentBaseline
	got, err := PrepareDocument(documentPath, []byte(baseline), []byte(documentFrontmatter+"# Time\n\n## Membros\n- A\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := documentFrontmatter + "# Time\n<!-- @id: time-camada-de-leitura -->\n\n## Membros\n<!-- @id: time-camada-de-leitura-membros -->\n- A\n"
	if string(got) != want {
		t.Fatalf("PrepareDocument() = %q, want %q", got, want)
	}
}

func TestPrepareDocumentRejectsDifferentFrontmatter(t *testing.T) {
	body := "# Time\n\n## Membros\n- A\n"
	if _, err := PrepareDocument(documentPath, []byte(documentFrontmatter+documentBaseline), []byte("---\ntags: [x]\n---\n"+body)); !errors.Is(err, ErrInvalidSchema) {
		t.Fatalf("different frontmatter error = %v, want ErrInvalidSchema", err)
	}
	if _, err := PrepareDocument(documentPath, []byte(documentBaseline), []byte(documentFrontmatter+body)); !errors.Is(err, ErrInvalidSchema) {
		t.Fatalf("frontmatter without baseline frontmatter error = %v, want ErrInvalidSchema", err)
	}
}
