package experiment

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/Wuerike/git-push-memories/internal/markdown"
)

type controlOmission struct {
	Path      string `json:"path"`
	SectionID string `json:"section_id"`
}

func buildTaskArms(source, treatment, control string, omission controlOmission) error {
	if omission.Path == "" || filepath.IsAbs(omission.Path) || filepath.Clean(omission.Path) != filepath.FromSlash(omission.Path) || filepath.Clean(omission.Path) == ".." {
		return errors.New("invalid control omission path")
	}
	if err := copyDirectory(source, treatment); err != nil {
		return err
	}
	if err := copyDirectory(source, control); err != nil {
		return err
	}
	target := filepath.Join(control, filepath.FromSlash(omission.Path))
	if omission.SectionID == "" {
		return os.Remove(target)
	}
	contents, err := os.ReadFile(target)
	if err != nil {
		return err
	}
	sections, err := markdown.ParseSections(contents)
	if err != nil {
		return err
	}
	section, exists := markdown.FindByID(sections, omission.SectionID)
	if !exists || section.ByteStart < 0 || section.ByteEnd > len(contents) {
		return errors.New("control omission section is unavailable")
	}
	poststate, err := markdown.Splice(contents, []markdown.SpliceOp{{ByteStart: section.ByteStart, ByteEnd: section.ByteEnd}})
	if err != nil {
		return err
	}
	return os.WriteFile(target, poststate, 0o600)
}

func copyDirectory(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("fixture tree contains a symlink")
		}
		if entry.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, contents, 0o600)
	})
}
