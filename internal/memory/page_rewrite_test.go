package memory

import (
	"errors"
	"os"
	"strings"
	"testing"
)

const rewritePath = "memories/domains/time-camada-de-leitura.md"

const rewriteBaseline = "# Wuerike é do time da camada de leitura\n<!-- @id: time-camada-de-leitura -->\n\n## Overview\n<!-- @id: time-camada-de-leitura-overview -->\nO usuário se chama Wuerike e faz parte do time da camada de leitura.\n"

func TestRewritePageInjectsIDsAndDropsOldSections(t *testing.T) {
	golden, err := os.ReadFile("testdata/page-rewrite/team.golden.md")
	if err != nil {
		t.Fatal(err)
	}
	got, err := rewritePage([]byte(rewriteBaseline), rewritePath, "# Time da camada de leitura\n\n## Membros\n- Wuerike\n- Guilherme\n")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(golden) {
		t.Fatalf("rewritePage() =\n%q\nwant\n%q", got, golden)
	}
}

func TestRewritePageKeepsExistingMarkers(t *testing.T) {
	replacement := "# Novo título\n<!-- @id: time-camada-de-leitura -->\nIntro.\n\n## Resumo\n<!-- @id: time-camada-de-leitura-overview -->\nTexto novo.\n"
	got, err := rewritePage([]byte(rewriteBaseline), rewritePath, replacement)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != replacement {
		t.Fatalf("rewritePage() = %q, want %q", got, replacement)
	}
}

func TestRewritePageRejectsInvalidInput(t *testing.T) {
	long := "# T\n\n## " + strings.Repeat("palavra ", 10) + "\nx\n"
	cases := []struct {
		name        string
		replacement string
		want        error
	}{
		{"unknown marker", "# T\n\n## A\n<!-- @id: time-camada-de-leitura-novo -->\nx\n", ErrInvalidID},
		{"h1 id mismatch", "# T\n<!-- @id: outra-pagina -->\nx\n", ErrInvalidID},
		{"two h1", "# T\n\n# U\nx\n", ErrInvalidSchema},
		{"malformed marker", "# T\n\n## A\n<!-- @id: Bad ID -->\nx\n", ErrInvalidSchema},
		{"duplicate generated id", "# T\n\n## A b\nx\n\n## A-b\ny\n", ErrSectionExists},
		{"id too long", long, ErrGeneratedIDTooLong},
		{"no change", rewriteBaseline, ErrInvalidSchema},
		{"merge conflict", "# T\n\n## A\n<<<<<<< HEAD\nx\n=======\ny\n>>>>>>> branch\n", ErrInvalidSchema},
		{"over 128 KiB", "# T\n\n## A\n" + strings.Repeat("x", 128*1024) + "\n", ErrInvalidSchema},
		{"empty replacement", "", ErrInvalidSchema},
		{"no heading", "só texto\n", ErrInvalidSchema},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := rewritePage([]byte(rewriteBaseline), rewritePath, tc.replacement); !errors.Is(err, tc.want) {
				t.Fatalf("rewritePage() error = %v, want %v", err, tc.want)
			}
		})
	}
}

const rewriteFrontmatter = "---\ntags: [a, b]\n---\n"

func TestRewritePagePreservesBaselineFrontmatter(t *testing.T) {
	baseline := rewriteFrontmatter + rewriteBaseline
	got, err := rewritePage([]byte(baseline), rewritePath, "# Time\n\n## Membros\n- A\n")
	if err != nil {
		t.Fatal(err)
	}
	want := rewriteFrontmatter + "# Time\n<!-- @id: time-camada-de-leitura -->\n\n## Membros\n<!-- @id: time-camada-de-leitura-membros -->\n- A\n"
	if string(got) != want {
		t.Fatalf("rewritePage() = %q, want %q", got, want)
	}
}

func TestRewritePageAcceptsIdenticalFrontmatterInReplacement(t *testing.T) {
	baseline := rewriteFrontmatter + rewriteBaseline
	got, err := rewritePage([]byte(baseline), rewritePath, rewriteFrontmatter+"# Time\n\n## Membros\n- A\n")
	if err != nil {
		t.Fatal(err)
	}
	want := rewriteFrontmatter + "# Time\n<!-- @id: time-camada-de-leitura -->\n\n## Membros\n<!-- @id: time-camada-de-leitura-membros -->\n- A\n"
	if string(got) != want {
		t.Fatalf("rewritePage() = %q, want %q", got, want)
	}
}

func TestRewritePageRejectsDifferentFrontmatter(t *testing.T) {
	body := "# Time\n\n## Membros\n- A\n"
	if _, err := rewritePage([]byte(rewriteFrontmatter+rewriteBaseline), rewritePath, "---\ntags: [x]\n---\n"+body); !errors.Is(err, ErrInvalidSchema) {
		t.Fatalf("different frontmatter error = %v, want ErrInvalidSchema", err)
	}
	if _, err := rewritePage([]byte(rewriteBaseline), rewritePath, rewriteFrontmatter+body); !errors.Is(err, ErrInvalidSchema) {
		t.Fatalf("frontmatter without baseline frontmatter error = %v, want ErrInvalidSchema", err)
	}
}
