package experiment

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildTaskArmsAppliesOnlyFrozenControlOmission(t *testing.T) {
	source := filepath.Join(t.TempDir(), "memory")
	if err := os.MkdirAll(filepath.Join(source, "memories", "domains"), 0o700); err != nil {
		t.Fatal(err)
	}
	page := "# Routing\n<!-- @id: routing -->\n\n## Queue\n<!-- @id: routing-queue -->\npayments-eu-v2\n\n## Owner\n<!-- @id: routing-owner -->\nrouting-ops\n"
	if err := os.WriteFile(filepath.Join(source, "gpm.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "memories", "domains", "routing.md"), []byte(page), 0o600); err != nil {
		t.Fatal(err)
	}
	treatment := filepath.Join(t.TempDir(), "treatment")
	control := filepath.Join(t.TempDir(), "control")
	if err := buildTaskArms(source, treatment, control, controlOmission{Path: "memories/domains/routing.md", SectionID: "routing-queue"}); err != nil {
		t.Fatal(err)
	}
	treatmentPage, _ := os.ReadFile(filepath.Join(treatment, "memories", "domains", "routing.md"))
	controlPage, _ := os.ReadFile(filepath.Join(control, "memories", "domains", "routing.md"))
	if string(treatmentPage) != page || strings.Contains(string(controlPage), "payments-eu-v2") || !strings.Contains(string(controlPage), "routing-ops") {
		t.Fatalf("treatment=%q control=%q", treatmentPage, controlPage)
	}
	if got := differingFiles(t, treatment, control); len(got) != 1 || got[0] != "memories/domains/routing.md" {
		t.Fatalf("arm delta = %v", got)
	}
}

func TestBuildTaskArmsCanOmitWholePage(t *testing.T) {
	source := filepath.Join(t.TempDir(), "memory")
	path := filepath.Join(source, "memories", "domains", "cache.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("# Cache\n<!-- @id: cache -->\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	treatment := filepath.Join(t.TempDir(), "treatment")
	control := filepath.Join(t.TempDir(), "control")
	if err := buildTaskArms(source, treatment, control, controlOmission{Path: "memories/domains/cache.md"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(control, "memories", "domains", "cache.md")); !os.IsNotExist(err) {
		t.Fatal("control retained the omitted page")
	}
	if _, err := os.Stat(filepath.Join(treatment, "memories", "domains", "cache.md")); err != nil {
		t.Fatal("treatment lost the canonical page")
	}
}

func differingFiles(t *testing.T, left, right string) []string {
	t.Helper()
	files := map[string][2]string{}
	for index, root := range []string{left, right} {
		_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil || entry.IsDir() {
				return err
			}
			relative, _ := filepath.Rel(root, path)
			contents, _ := os.ReadFile(path)
			pair := files[filepath.ToSlash(relative)]
			pair[index] = string(contents)
			files[filepath.ToSlash(relative)] = pair
			return nil
		})
	}
	var result []string
	for path, contents := range files {
		if contents[0] != contents[1] {
			result = append(result, path)
		}
	}
	return result
}
