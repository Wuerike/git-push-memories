package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Wuerike/git-push-memories/internal/workspace"
)

const schemaVersion = 1

type Config struct {
	RepoURL          string
	Paths            workspace.Paths
	MemoryRoot       string // the read worktree: published main
	StateRoot        string // per-repository state, also the lock root
	MemoryCategories []string
}

type fileConfig struct {
	SchemaVersion int    `json:"schema_version"`
	RepoURL       string `json:"repo_url"`
}

type repositoryConfig struct {
	SchemaVersion    int      `json:"schema_version"`
	MemoryCategories []string `json:"memory_categories"`
}

func (config Config) IsMemoryPath(requested string) bool {
	if requested == "" || strings.Contains(requested, "\\") || path.IsAbs(requested) || path.Clean(requested) != requested || path.Ext(requested) != ".md" {
		return false
	}
	relative, found := strings.CutPrefix(requested, "memories/")
	if !found {
		return false
	}
	category, remainder, found := strings.Cut(relative, "/")
	if !found || remainder == "" {
		return false
	}
	for _, configured := range config.MemoryCategories {
		if category == configured {
			return true
		}
	}
	return false
}

var categoryPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// StoredURL returns the configured repository URL, or "" when setup never ran.
func StoredURL() (string, error) {
	path, err := Path()
	if err != nil {
		return "", errors.New("configuration path is unavailable")
	}
	contents, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", errors.New("configuration could not be read")
	}
	var stored fileConfig
	if err := json.Unmarshal(contents, &stored); err != nil || stored.SchemaVersion != schemaVersion || stored.RepoURL == "" {
		return "", errors.New("configuration is invalid")
	}
	return stored.RepoURL, nil
}

func Load() (Config, error) {
	url, err := StoredURL()
	if err != nil {
		return Config{}, err
	}
	if url == "" {
		return Config{}, errors.New("git-push-memories is not configured; run setup first")
	}
	return Resolve(url)
}

// Resolve derives every path from the repository URL and reads the categories from the read worktree.
func Resolve(repoURL string) (Config, error) {
	if repoURL == "" {
		return Config{}, errors.New("repository URL is required")
	}
	paths, err := workspace.PathsFor(repoURL)
	if err != nil {
		return Config{}, errors.New("state root is unavailable")
	}
	categories, err := LoadRepositoryConfig(paths.Read)
	if err != nil {
		if _, statErr := os.Stat(filepath.Join(paths.Read, ".git")); statErr != nil {
			return Config{}, errors.New("memory repository is not prepared; run setup")
		}
		return Config{}, err
	}
	return Config{RepoURL: repoURL, Paths: paths, MemoryRoot: paths.Read, StateRoot: paths.State, MemoryCategories: categories}, nil
}

// Workspace opens the Git layer for this configuration.
func (config Config) Workspace() *workspace.Workspace {
	return &workspace.Workspace{URL: config.RepoURL, Paths: config.Paths}
}

// LoadRepositoryConfig reads and validates gpm.json under root.
func LoadRepositoryConfig(root string) ([]string, error) {
	contents, err := os.ReadFile(filepath.Join(root, "gpm.json"))
	if err != nil {
		return nil, errors.New("repository configuration is invalid")
	}
	return ParseRepositoryConfig(contents)
}

// RepositoryConfigJSON renders a valid gpm.json for the categories.
func RepositoryConfigJSON(categories []string) ([]byte, error) {
	payload, err := json.MarshalIndent(repositoryConfig{SchemaVersion: schemaVersion, MemoryCategories: categories}, "", "  ")
	if err != nil {
		return nil, err
	}
	payload = append(payload, '\n')
	if _, err := ParseRepositoryConfig(payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func ParseRepositoryConfig(contents []byte) ([]string, error) {
	var stored repositoryConfig
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&stored); err != nil || stored.SchemaVersion != schemaVersion || len(stored.MemoryCategories) == 0 {
		return nil, errors.New("repository configuration is invalid")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, errors.New("repository configuration is invalid")
	}
	seen := make(map[string]struct{}, len(stored.MemoryCategories))
	for _, category := range stored.MemoryCategories {
		if !categoryPattern.MatchString(category) {
			return nil, errors.New("repository configuration is invalid")
		}
		if _, exists := seen[category]; exists {
			return nil, errors.New("repository configuration is invalid")
		}
		seen[category] = struct{}{}
	}
	return append([]string(nil), stored.MemoryCategories...), nil
}

func Save(repoURL string) error {
	path, err := Path()
	if err != nil {
		return errors.New("configuration path is unavailable")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return errors.New("configuration directory could not be created")
	}
	payload, err := json.MarshalIndent(fileConfig{SchemaVersion: schemaVersion, RepoURL: repoURL}, "", "  ")
	if err != nil {
		return errors.New("configuration could not be encoded")
	}
	payload = append(payload, '\n')
	temporary, err := os.CreateTemp(filepath.Dir(path), ".config-*.tmp")
	if err != nil {
		return errors.New("configuration could not be written")
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return errors.New("configuration could not be secured")
	}
	if _, err := temporary.Write(payload); err != nil {
		temporary.Close()
		return errors.New("configuration could not be written")
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return errors.New("configuration could not be synchronized")
	}
	if err := temporary.Close(); err != nil {
		return errors.New("configuration could not be written")
	}
	if err := os.Rename(temporaryName, path); err != nil {
		return errors.New("configuration could not be installed")
	}
	return nil
}

func Path() (string, error) {
	root, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "git-push-memories", "config.json"), nil
}
