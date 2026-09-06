package proposal

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/sys/unix"
)

var validID = regexp.MustCompile(`^[0-9a-f]{32}$`)
var validTemporary = regexp.MustCompile(`^\.git-push-memories-[0-9a-f]{16}\.tmp$`)

type store struct {
	root  string
	fault func(string) error
}

func newStore(root string, cleanup bool) (*store, error) {
	store := &store{root: root}
	if cleanup {
		if err := store.cleanup(); err != nil {
			return nil, err
		}
	}
	return store, nil
}

func (store *store) writeProposal(view View) error {
	encoded, err := json.Marshal(view)
	if err != nil {
		return err
	}
	return store.durableWrite("proposals", view.ID+".json", encoded)
}

func (store *store) readProposal(id string) (View, error) {
	var view View
	if !validID.MatchString(id) {
		return view, ErrNotFound
	}
	if err := store.readJSON("proposals", id+".json", &view); err != nil {
		return view, err
	}
	return view, nil
}

func (store *store) readDecision(id string) (DecisionReceipt, error) {
	var receipt DecisionReceipt
	if !validID.MatchString(id) {
		return receipt, ErrNotFound
	}
	if err := store.readJSON("decisions", id+".json", &receipt); err != nil {
		return receipt, err
	}
	return receipt, nil
}

func (store *store) readJSON(directory, name string, target any) error {
	directoryPath := filepath.Join(store.root, directory)
	if err := validatePrivateDirectory(directoryPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrNotFound
		}
		return fmt.Errorf("read proposal state")
	}
	filePath := filepath.Join(directoryPath, name)
	info, err := os.Lstat(filePath)
	if errors.Is(err, os.ErrNotExist) {
		return ErrNotFound
	}
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		return fmt.Errorf("read proposal state")
	}
	encoded, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read proposal state")
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("read proposal state")
	}
	if _, err := decoder.Token(); err != io.EOF {
		return fmt.Errorf("read proposal state")
	}
	if err := validateStoredState(name, target); err != nil {
		return fmt.Errorf("read proposal state")
	}
	return nil
}

func validateStoredState(name string, target any) error {
	id := strings.TrimSuffix(name, ".json")
	switch value := target.(type) {
	case *View:
		if value.SchemaVersion != 1 || value.ID != id || value.Revision != 1 || value.Status != "pending" || value.CreatedAt.IsZero() ||
			value.Operation.Type != "replace_section_content" || value.Operation.Target.Path == "" || value.Operation.Target.SectionID == "" ||
			!validHash(value.Baseline.FileHash) || !validHash(value.PostState.FileHash) || value.Baseline.Head == "" || value.Baseline.Blob == "" || value.Diff == "" || value.SourcePaths == nil {
			return errors.New("invalid proposal")
		}
	case *DecisionReceipt:
		if value.SchemaVersion != 1 || value.ProposalID != id || value.Revision != 1 || value.Status != "rejected" || value.DecidedAt.IsZero() {
			return errors.New("invalid decision")
		}
		if _, valid := rejectionReasons[value.Reason]; !valid {
			return errors.New("invalid decision")
		}
	default:
		return errors.New("unsupported proposal state")
	}
	return nil
}

func validHash(value string) bool {
	if len(value) != len("sha256:")+64 || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil
}

func (store *store) reject(receipt DecisionReceipt) error {
	encoded, err := json.Marshal(receipt)
	if err != nil {
		return err
	}
	if err := store.durableWrite("decisions", receipt.ProposalID+".json", encoded); err != nil {
		return err
	}
	return store.finishRejection(receipt.ProposalID)
}

func (store *store) finishRejection(id string) error {
	proposalPath := filepath.Join(store.root, "proposals", id+".json")
	if err := os.Remove(proposalPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove rejected proposal")
	}
	return syncDirectory(filepath.Dir(proposalPath))
}

func (store *store) durableWrite(directory, name string, contents []byte) error {
	directoryPath := filepath.Join(store.root, directory)
	if err := privateDirectory(store.root); err != nil {
		return err
	}
	if err := privateDirectory(directoryPath); err != nil {
		return err
	}
	temporary, err := temporaryPath(directoryPath)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create proposal temporary")
	}
	committed := false
	defer func() {
		_ = file.Close()
		if !committed {
			_ = os.Remove(temporary)
		}
	}()
	if _, err := file.Write(contents); err != nil {
		return fmt.Errorf("write proposal temporary")
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync proposal temporary")
	}
	if store.fault != nil {
		if err := store.fault("before_rename"); err != nil {
			return err
		}
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close proposal temporary")
	}
	if err := unix.Renameat2(unix.AT_FDCWD, temporary, unix.AT_FDCWD, filepath.Join(directoryPath, name), unix.RENAME_NOREPLACE); err != nil {
		return fmt.Errorf("publish proposal state: %w", err)
	}
	committed = true
	return syncDirectory(directoryPath)
}

func privateDirectory(directory string) error {
	info, err := os.Lstat(directory)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			return fmt.Errorf("create private state directory")
		}
		return os.Chmod(directory, 0o700)
	}
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0o700 {
		return fmt.Errorf("invalid private state directory")
	}
	return nil
}

func validatePrivateDirectory(directory string) error {
	info, err := os.Lstat(directory)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0o700 {
		return errors.New("invalid private state directory")
	}
	return nil
}

func temporaryPath(directory string) (string, error) {
	random := make([]byte, 8)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	return filepath.Join(directory, ".git-push-memories-"+hex.EncodeToString(random)+".tmp"), nil
}

func syncDirectory(directory string) error {
	handle, err := os.Open(directory)
	if err != nil {
		return err
	}
	defer handle.Close()
	return handle.Sync()
}

func (store *store) cleanup() error {
	info, err := os.Lstat(store.root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
		return fmt.Errorf("invalid private state directory")
	}
	return filepath.WalkDir(store.root, func(filePath string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type().IsRegular() && validTemporaryName(entry.Name()) {
			return os.Remove(filePath)
		}
		return nil
	})
}

func validTemporaryName(name string) bool {
	return validTemporary.MatchString(name)
}
