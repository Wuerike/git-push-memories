package experiment

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type privacyScanSpec struct {
	FileRoots []string
	GitRoots  []string
	Markers   [][]byte
}

type PrivacyScanResult struct {
	Clean         bool `json:"clean"`
	FileHits      int  `json:"file_hits"`
	GitObjectHits int  `json:"git_object_hits"`
}

type objectInventory struct {
	IDs   []string
	Files map[string]string
}

func scanPrivacy(ctx context.Context, spec privacyScanSpec) (PrivacyScanResult, error) {
	result := PrivacyScanResult{Clean: true}
	for _, root := range spec.FileRoots {
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() && entry.Name() == ".git" {
				return filepath.SkipDir
			}
			if entry.IsDir() {
				return nil
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return errors.New("privacy scan encountered a symlink")
			}
			contents, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			result.FileHits += markerHits(contents, spec.Markers)
			return nil
		})
		if err != nil {
			return PrivacyScanResult{}, err
		}
	}
	for _, root := range spec.GitRoots {
		inventory, err := gitObjectInventory(ctx, root)
		if err != nil {
			return PrivacyScanResult{}, err
		}
		for _, id := range inventory.IDs {
			contents, err := gitOutputBytes(ctx, root, "cat-file", "-p", id)
			if err != nil {
				return PrivacyScanResult{}, err
			}
			result.GitObjectHits += markerHits(contents, spec.Markers)
		}
	}
	result.Clean = result.FileHits == 0 && result.GitObjectHits == 0
	return result, nil
}

func gitObjectInventory(ctx context.Context, root string) (objectInventory, error) {
	output, err := gitOutputBytes(ctx, root, "cat-file", "--batch-all-objects", "--batch-check=%(objectname)")
	if err != nil {
		return objectInventory{}, err
	}
	ids := strings.Fields(string(output))
	sort.Strings(ids)
	files := make(map[string]string)
	objectsRoot := filepath.Join(root, ".git", "objects")
	err = filepath.WalkDir(objectsRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("Git object store contains a symlink")
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(objectsRoot, path)
		if err != nil {
			return err
		}
		digest := sha256.Sum256(contents)
		files[filepath.ToSlash(relative)] = hex.EncodeToString(digest[:])
		return nil
	})
	if err != nil {
		return objectInventory{}, err
	}
	return objectInventory{IDs: ids, Files: files}, nil
}

func inventoriesEqual(left, right objectInventory) bool {
	if !slicesEqual(left.IDs, right.IDs) || len(left.Files) != len(right.Files) {
		return false
	}
	for path, digest := range left.Files {
		if right.Files[path] != digest {
			return false
		}
	}
	return true
}

func gitOutputBytes(ctx context.Context, root string, arguments ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, "git", arguments...)
	command.Dir = root
	command.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=/dev/null", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "LC_ALL=C.UTF-8", "TZ=UTC"}
	return command.Output()
}

func markerHits(contents []byte, markers [][]byte) int {
	hits := 0
	for _, marker := range markers {
		if len(marker) > 0 {
			hits += bytes.Count(contents, marker)
		}
	}
	return hits
}
