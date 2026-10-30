// Package github is a thin wrapper over the gh CLI. Every command carries
// -R owner/repo and is non-interactive.
package github

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"
)

var (
	ErrNotGitHub     = errors.New("the repository URL is not a GitHub repository")
	ErrUnavailable   = errors.New("gh is not installed or not authenticated: run `gh auth login`")
	ErrCommandFailed = errors.New("gh command failed")
	ErrPermission    = errors.New("the GitHub account cannot write to the repository")
	ErrConflict      = errors.New("the branch moved while writing")
)

var Timeout = 60 * time.Second

var remotePattern = regexp.MustCompile(`^(?:git@github\.com:|ssh://git@github\.com/|https?://(?:[^@/]+@)?github\.com/|git://github\.com/)([^/\s]+)/([^/\s]+?)(?:\.git)?/?$`)

// Slug extracts owner/repo from a GitHub remote URL.
func Slug(url string) (string, error) {
	match := remotePattern.FindStringSubmatch(strings.TrimSpace(url))
	if match == nil {
		return "", ErrNotGitHub
	}
	return match[1] + "/" + match[2], nil
}

type Client struct {
	Repo string // owner/repo
}

func New(url string) (Client, error) {
	slug, err := Slug(url)
	if err != nil {
		return Client{}, err
	}
	return Client{Repo: slug}, nil
}

type PR struct {
	Number      int       `json:"number"`
	State       string    `json:"state"` // OPEN, MERGED, CLOSED
	URL         string    `json:"url"`
	CreatedAt   time.Time `json:"createdAt"`
	MergeCommit struct {
		OID string `json:"oid"`
	} `json:"mergeCommit"`
}

func (client Client) run(ctx context.Context, arguments ...string) (string, error) {
	return client.runInput(ctx, nil, arguments...)
}

func (client Client) runInput(ctx context.Context, input []byte, arguments ...string) (string, error) {
	if _, err := exec.LookPath("gh"); err != nil {
		return "", ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	command := exec.CommandContext(ctx, "gh", arguments...)
	command.Env = append(os.Environ(), "GH_PROMPT_DISABLED=1", "GH_NO_UPDATE_NOTIFIER=1")
	if input != nil {
		command.Stdin = bytes.NewReader(input)
	}
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		switch {
		case strings.Contains(message, "HTTP 409"):
			return "", fmt.Errorf("%w: %s", ErrConflict, message)
		case strings.Contains(message, "HTTP 401"), strings.Contains(message, "HTTP 403"), strings.Contains(message, "HTTP 404"):
			return "", fmt.Errorf("%w: %s", ErrPermission, message)
		}
		return "", fmt.Errorf("%w: %s", ErrCommandFailed, message)
	}
	return strings.TrimSpace(stdout.String()), nil
}

// AuthStatus fails with ErrUnavailable when gh is missing or not logged in.
func (client Client) AuthStatus(ctx context.Context) error {
	if _, err := client.run(ctx, "auth", "status"); err != nil {
		return ErrUnavailable
	}
	return nil
}

// PRCreate opens a PR from head into base and returns its URL.
func (client Client) PRCreate(ctx context.Context, base, head, title, body string) (string, error) {
	return client.run(ctx, "pr", "create", "-R", client.Repo, "--base", base, "--head", head, "--title", title, "--body", body)
}

// PRFind returns the most recent PR of head in any state, or nil when there is none.
func (client Client) PRFind(ctx context.Context, head string) (*PR, error) {
	out, err := client.run(ctx, "pr", "list", "-R", client.Repo, "--head", head, "--state", "all", "--limit", "100", "--json", "number,state,url,createdAt,mergeCommit")
	if err != nil {
		return nil, err
	}
	var prs []PR
	if err := json.Unmarshal([]byte(out), &prs); err != nil {
		return nil, fmt.Errorf("%w: unreadable pr list", ErrCommandFailed)
	}
	if len(prs) == 0 {
		return nil, nil
	}
	sort.SliceStable(prs, func(i, j int) bool {
		if prs[i].CreatedAt.Equal(prs[j].CreatedAt) {
			return prs[i].Number > prs[j].Number
		}
		return prs[i].CreatedAt.After(prs[j].CreatedAt)
	})
	return &prs[0], nil
}

// PRView returns the state and merge commit of one PR.
func (client Client) PRView(ctx context.Context, number int) (*PR, error) {
	out, err := client.run(ctx, "pr", "view", fmt.Sprint(number), "-R", client.Repo, "--json", "number,state,url,createdAt,mergeCommit")
	if err != nil {
		return nil, err
	}
	var pr PR
	if err := json.Unmarshal([]byte(out), &pr); err != nil {
		return nil, fmt.Errorf("%w: unreadable pr view", ErrCommandFailed)
	}
	return &pr, nil
}

// CreateFile commits a new file to branch through the contents API and returns the commit SHA. The
// branch moving under a concurrent writer (HTTP 409) is retried, since every proposal is a distinct path.
func (client Client) CreateFile(ctx context.Context, branch, path, message string, contents []byte) (string, error) {
	body, err := json.Marshal(map[string]string{"message": message, "branch": branch, "content": base64.StdEncoding.EncodeToString(contents)})
	if err != nil {
		return "", err
	}
	for attempt := 0; ; attempt++ {
		out, err := client.runInput(ctx, body, "api", "-X", "PUT", "repos/"+client.Repo+"/contents/"+path, "--input", "-")
		if errors.Is(err, ErrConflict) && attempt < 4 {
			continue
		}
		if err != nil {
			return "", err
		}
		var created struct {
			Commit struct {
				SHA string `json:"sha"`
			} `json:"commit"`
		}
		if json.Unmarshal([]byte(out), &created) != nil || created.Commit.SHA == "" {
			return "", fmt.Errorf("%w: unreadable contents response", ErrCommandFailed)
		}
		return created.Commit.SHA, nil
	}
}
