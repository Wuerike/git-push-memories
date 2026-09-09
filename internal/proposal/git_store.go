package proposal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"sort"
	"strings"
)

const proposalRefPrefix = "refs/heads/gpm/proposal/"

func Branch(id string) string {
	if !validID.MatchString(id) {
		return ""
	}
	return strings.TrimPrefix(proposalRefPrefix, "refs/heads/") + id
}

func proposalPath(id string) string {
	return path.Join("proposals", id+".json")
}

func (store *store) writeGitProposal(view View) error {
	if !validView(view, view.ID) {
		return ErrIntegrity
	}
	encoded, err := json.Marshal(view)
	if err != nil {
		return err
	}
	if err := privateDirectory(store.root); err != nil {
		return err
	}
	return store.withStateLock(func() error {
		indexPath, err := temporaryPath(store.root)
		if err != nil {
			return err
		}
		defer os.Remove(indexPath)
		environment := map[string]string{"GIT_INDEX_FILE": indexPath}
		if _, err := store.gitInput(context.Background(), environment, nil, "read-tree", view.Baseline.Head); err != nil {
			return fmt.Errorf("initialize proposal tree")
		}
		blob, err := store.gitText(context.Background(), nil, encoded, "hash-object", "-w", "--stdin")
		if err != nil || blob == "" {
			return fmt.Errorf("write proposal blob")
		}
		cacheInfo := "100644," + blob + "," + proposalPath(view.ID)
		if _, err := store.gitInput(context.Background(), environment, nil, "update-index", "--add", "--cacheinfo", cacheInfo); err != nil {
			return fmt.Errorf("update proposal tree")
		}
		tree, err := store.gitText(context.Background(), environment, nil, "write-tree")
		if err != nil || tree == "" {
			return fmt.Errorf("write proposal tree")
		}
		commitEnvironment := map[string]string{
			"GIT_AUTHOR_NAME": "git-push-memories", "GIT_AUTHOR_EMAIL": "git-push-memories@local", "GIT_AUTHOR_DATE": view.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			"GIT_COMMITTER_NAME": "git-push-memories", "GIT_COMMITTER_EMAIL": "git-push-memories@local", "GIT_COMMITTER_DATE": view.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		}
		message := []byte(fmt.Sprintf("Store memory proposal\n\nGit-Push-Memories-Proposal: %s/%d\n", view.ID, view.Revision))
		commit, err := store.gitText(context.Background(), commitEnvironment, message, "commit-tree", tree, "-p", view.Baseline.Head)
		if err != nil || commit == "" {
			return fmt.Errorf("commit proposal")
		}
		if store.fault != nil {
			if err := store.fault("before_ref_update"); err != nil {
				return err
			}
		}
		zero := strings.Repeat("0", len(view.Baseline.Head))
		if _, err := store.gitInput(context.Background(), nil, nil, "update-ref", "--no-deref", proposalRefPrefix+view.ID, commit, zero); err != nil {
			return fmt.Errorf("publish proposal branch")
		}
		if store.fault != nil {
			if err := store.fault("after_ref_update"); err != nil {
				return err
			}
		}
		return nil
	})
}

func (store *store) readGitProposal(id string) (View, error) {
	var view View
	if !validID.MatchString(id) {
		return view, ErrNotFound
	}
	ref := proposalRefPrefix + id
	commit, err := store.gitText(context.Background(), nil, nil, "rev-parse", "--verify", ref+"^{commit}")
	if err != nil {
		return view, ErrNotFound
	}
	encoded, err := store.gitInput(context.Background(), nil, nil, "show", commit+":"+proposalPath(id))
	if err != nil {
		return view, ErrIntegrity
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&view); err != nil {
		return View{}, ErrIntegrity
	}
	if _, err := decoder.Token(); err != io.EOF {
		return View{}, ErrIntegrity
	}
	if err := validateStoredState(id+".json", &view); err != nil {
		return View{}, ErrIntegrity
	}
	parents, err := store.gitText(context.Background(), nil, nil, "rev-list", "--parents", "-n", "1", commit)
	if err != nil {
		return View{}, ErrIntegrity
	}
	parentFields := strings.Fields(parents)
	if len(parentFields) != 2 || parentFields[0] != commit || parentFields[1] != view.Baseline.Head {
		return View{}, ErrIntegrity
	}
	changed, err := store.gitInput(context.Background(), nil, nil, "diff-tree", "--no-commit-id", "--name-only", "-r", "-z", commit)
	if err != nil || !bytes.Equal(changed, []byte(proposalPath(id)+"\x00")) {
		return View{}, ErrIntegrity
	}
	return view, nil
}

func (store *store) gitProposalIDs() ([]string, error) {
	output, err := store.gitText(context.Background(), nil, nil, "for-each-ref", "--format=%(refname)", proposalRefPrefix)
	if err != nil {
		return nil, fmt.Errorf("list proposal branches")
	}
	if output == "" {
		return nil, nil
	}
	ids := make([]string, 0)
	for _, ref := range strings.Fields(output) {
		id := strings.TrimPrefix(ref, proposalRefPrefix)
		if validID.MatchString(id) && ref == proposalRefPrefix+id {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids, nil
}

func (store *store) confirmGitProposal(id string) error {
	_, err := store.readGitProposal(id)
	return err
}

func (store *store) gitText(ctx context.Context, environment map[string]string, input []byte, arguments ...string) (string, error) {
	output, err := store.gitInput(ctx, environment, input, arguments...)
	return strings.TrimSpace(string(output)), err
}

func (store *store) gitInput(ctx context.Context, extraEnvironment map[string]string, input []byte, arguments ...string) ([]byte, error) {
	commandArguments := append([]string{"--no-optional-locks", "--no-replace-objects", "-C", "/proc/self/fd/3"}, arguments...)
	command := exec.CommandContext(ctx, "git", commandArguments...)
	command.Env = proposalGitEnvironment(extraEnvironment)
	command.ExtraFiles = []*os.File{store.memoryRoot}
	command.Stdin = bytes.NewReader(input)
	return command.Output()
}

func proposalGitEnvironment(extra map[string]string) []string {
	environment := make([]string, 0, len(os.Environ())+len(extra))
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "GIT_") {
			environment = append(environment, entry)
		}
	}
	for key, value := range extra {
		environment = append(environment, key+"="+value)
	}
	return environment
}
