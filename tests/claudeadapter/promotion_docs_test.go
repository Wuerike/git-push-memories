package claudeadapter_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// This contract checks public workflow instructions, not prose formatting.
func TestPromotionDocumentationDescribesImplementedWorkflow(t *testing.T) {
	documents := []struct {
		path     string
		required []string
	}{
		{"docs/workflow.md", []string{
			"memory_promote_prepare", "memory_promote_check", "memory_promote_submit", "memory_promote_finish",
			"revisão fixa", "fora do", "Aprove o plano global na conversa", "todas as propostas e todas as",
			"Nenhum arquivo canônico é editado antes", "diff atual", "suspendem a escrita",
			"gpm promote --finish", "merge humano", "caminho e blob", "novas ou\n   alteradas são preservadas",
			"commit global apenas", "esta versão não retoma", "diff canônico vazio",
		}},
		{"docs/features.md", []string{
			"memory_promote_prepare", "memory_promote_check", "memory_promote_submit", "memory_promote_finish",
			"gpm promote --finish", "source: main", "source: proposals", "summary", "pr_number", "input_path",
			"commit global", "merge é humano", "Propostas novas ou alteradas", "promotion-published.json",
			"não há avaliação estruturada de fontes", "não executam julgamento por um LLM",
		}},
	}
	for _, doc := range documents {
		t.Run(doc.path, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("..", "..", filepath.FromSlash(doc.path)))
			if err != nil {
				t.Fatal(err)
			}
			text := string(data)
			for _, required := range doc.required {
				if !strings.Contains(text, required) {
					t.Errorf("missing workflow instruction %q", required)
				}
			}
			for _, obsolete := range []string{
				"release.lock.json", "source: release", "memory_promote_preview", "memory_promote_apply",
				"memory_promote_batch", "memory_promote_open", "memory_promote_status", "preview_token",
				"fold_proposal", "--resume", "--abort", "source_support",
			} {
				if strings.Contains(text, obsolete) {
					t.Errorf("retired public contract %q remains", obsolete)
				}
			}
		})
	}
}
