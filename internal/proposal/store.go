package proposal

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
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
	"time"

	"golang.org/x/sys/unix"
)

var validID = regexp.MustCompile(`^[0-9a-f]{32}$`)
var validTemporary = regexp.MustCompile(`^\.git-push-memories-[0-9a-f]{16}\.tmp$`)

type store struct {
	root         string
	fault        func(string) error
	cleanupFault func(string) error
}

func (store *store) withProposalLock(ctx context.Context, id string, action func() error) error {
	if !validID.MatchString(id) {
		return ErrNotFound
	}
	if err := privateDirectory(store.root); err != nil {
		return err
	}
	lockDirectory := filepath.Join(store.root, "locks")
	if err := privateDirectory(lockDirectory); err != nil {
		return err
	}
	lockFD, err := unix.Open(filepath.Join(lockDirectory, id+".lock"), unix.O_RDWR|unix.O_CREAT|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0o600)
	if err != nil {
		return fmt.Errorf("open proposal lock")
	}
	lock := os.NewFile(uintptr(lockFD), id+".lock")
	if lock == nil {
		_ = unix.Close(lockFD)
		return fmt.Errorf("open proposal lock")
	}
	defer lock.Close()
	info, err := lock.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("invalid proposal lock")
	}
	if err := lock.Chmod(0o600); err != nil {
		return fmt.Errorf("secure proposal lock")
	}
	for {
		err := unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			break
		}
		if !errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EAGAIN) {
			return fmt.Errorf("lock proposal")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Millisecond):
		}
	}
	defer unix.Flock(int(lock.Fd()), unix.LOCK_UN)
	return action()
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

func (store *store) readApply(id string) (ApplyReceipt, error) {
	var receipt ApplyReceipt
	if !validID.MatchString(id) {
		return receipt, ErrNotFound
	}
	if err := store.readJSON("applies", id+".json", &receipt); err != nil {
		return receipt, err
	}
	return receipt, nil
}

func (store *store) readPublication(id string) (PublicationReceipt, error) {
	var receipt PublicationReceipt
	if !validID.MatchString(id) {
		return receipt, ErrNotFound
	}
	if err := store.readJSON("publications", id+".json", &receipt); err != nil {
		return receipt, err
	}
	return receipt, nil
}

func (store *store) proposalIDs() ([]string, error) {
	directory := filepath.Join(store.root, "proposals")
	if err := validatePrivateDirectory(directory); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("list proposal state")
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("list proposal state")
	}
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.Type().IsRegular() && strings.HasSuffix(entry.Name(), ".json") {
			id := strings.TrimSuffix(entry.Name(), ".json")
			if validID.MatchString(id) {
				ids = append(ids, id)
			}
		}
	}
	return ids, nil
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
		return fmt.Errorf("%w: read proposal state", ErrIntegrity)
	}
	if _, err := decoder.Token(); err != io.EOF {
		return fmt.Errorf("%w: read proposal state", ErrIntegrity)
	}
	if err := validateStoredState(name, target); err != nil {
		return fmt.Errorf("%w: read proposal state", ErrIntegrity)
	}
	return nil
}

func validateStoredState(name string, target any) error {
	id := strings.TrimSuffix(name, ".json")
	switch value := target.(type) {
	case *View:
		if !validView(*value, id) {
			return errors.New("invalid proposal")
		}
		expectedIntegrity, err := proposalIntegrity(*value)
		if err != nil || expectedIntegrity != value.IntegrityHash || value.ID != strings.TrimPrefix(expectedIntegrity, "sha256:")[:32] {
			return errors.New("invalid proposal")
		}
	case *DecisionReceipt:
		if (value.SchemaVersion != 1 && value.SchemaVersion != 2) || value.ProposalID != id || value.Revision != 1 || value.Status != "rejected" || value.DecidedAt.IsZero() {
			return errors.New("invalid decision")
		}
		if _, valid := rejectionReasons[value.Reason]; !valid {
			return errors.New("invalid decision")
		}
	case *ApplyReceipt:
		if !validApplyReceipt(*value, id) {
			return errors.New("invalid apply receipt")
		}
	case *PublicationReceipt:
		if value.SchemaVersion != 1 || value.ProposalID != id || value.Revision != 1 || value.Status != "published" || value.PublishedAt.IsZero() ||
			value.MemoryRoot == "" || value.Target.Path == "" || value.Target.SectionID == "" || value.Branch == "" || value.Parent == "" || value.Commit == "" || value.Blob == "" {
			return errors.New("invalid publication receipt")
		}
	default:
		return errors.New("unsupported proposal state")
	}
	return nil
}

func validView(value View, id string) bool {
	if value.ID != id || !validID.MatchString(value.Nonce) || value.Revision != 1 || value.Status != "pending" || value.CreatedAt.IsZero() ||
		value.Operation.Target.Path == "" || value.Operation.Target.SectionID == "" || !validHash(value.PostState.FileHash) || value.Baseline.Head == "" || value.Diff == "" || value.SourcePaths == nil || !validHash(value.IntegrityHash) {
		return false
	}
	switch value.SchemaVersion {
	case 1:
		return value.Operation.Type == "replace_section_content" && value.Operation.Title == "" && value.Operation.Body == "" &&
			value.Baseline.Exists == nil && validHash(value.Baseline.FileHash) && value.Baseline.Blob != ""
	case 2:
		if value.Baseline.Exists == nil {
			return false
		}
		if value.Operation.Type == "create_page" {
			return !*value.Baseline.Exists && value.Operation.Replacement == "" && value.Operation.Title != "" && value.Operation.Body != "" && value.Baseline.FileHash == "" && value.Baseline.Blob == ""
		}
		return value.Operation.Type == "replace_section_content" && *value.Baseline.Exists && value.Operation.Title == "" && value.Operation.Body == "" && validHash(value.Baseline.FileHash) && value.Baseline.Blob != ""
	default:
		return false
	}
}

func validApplyReceipt(value ApplyReceipt, id string) bool {
	if value.ProposalID != id || value.Revision != 1 || value.Status != "applied_unpublished" || value.AppliedAt.IsZero() || value.MemoryRoot == "" ||
		value.Target.Path == "" || value.Target.SectionID == "" || value.Baseline.Head == "" || !validHash(value.PostState.FileHash) {
		return false
	}
	if value.SchemaVersion == 1 {
		return value.Baseline.Exists == nil && validHash(value.Baseline.FileHash) && value.Baseline.Blob != ""
	}
	if value.SchemaVersion != 2 || value.Baseline.Exists == nil {
		return false
	}
	if *value.Baseline.Exists {
		return validHash(value.Baseline.FileHash) && value.Baseline.Blob != ""
	}
	return value.Baseline.FileHash == "" && value.Baseline.Blob == ""
}

func sealProposal(view *View) error {
	integrity, err := proposalIntegrity(*view)
	if err != nil {
		return err
	}
	view.IntegrityHash = integrity
	view.ID = strings.TrimPrefix(integrity, "sha256:")[:32]
	return nil
}

func proposalIntegrity(view View) (string, error) {
	view.IntegrityHash = ""
	view.ID = ""
	encoded, err := json.Marshal(view)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func (store *store) writeApply(receipt ApplyReceipt) error {
	encoded, err := json.Marshal(receipt)
	if err != nil {
		return err
	}
	return store.durableWrite("applies", receipt.ProposalID+".json", encoded)
}

func (store *store) writePublication(receipt PublicationReceipt) error {
	encoded, err := json.Marshal(receipt)
	if err != nil {
		return err
	}
	return store.durableWrite("publications", receipt.ProposalID+".json", encoded)
}

func (store *store) finishApply(id string) error {
	if store.cleanupFault != nil {
		if err := store.cleanupFault("before_apply_cleanup"); err != nil {
			return err
		}
	}
	proposalPath := filepath.Join(store.root, "proposals", id+".json")
	if err := os.Remove(proposalPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove applied proposal")
	}
	return syncDirectory(filepath.Dir(proposalPath))
}

func (store *store) confirmDurable(directory string) error {
	return syncDirectory(filepath.Join(store.root, directory))
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
	if err := privateDirectory(store.root); err != nil {
		return err
	}
	return store.withStateLock(func() error {
		return store.durableWriteLocked(directory, name, contents)
	})
}

func (store *store) durableWriteLocked(directory, name string, contents []byte) error {
	directoryPath := filepath.Join(store.root, directory)
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
	if store.fault != nil {
		if err := store.fault("after_rename"); err != nil {
			return err
		}
	}
	return syncDirectory(directoryPath)
}

func (store *store) withStateLock(action func() error) error {
	rootFD, err := unix.Open(store.root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return fmt.Errorf("open state lock")
	}
	root := os.NewFile(uintptr(rootFD), store.root)
	if root == nil {
		_ = unix.Close(rootFD)
		return fmt.Errorf("open state lock")
	}
	defer root.Close()
	if err := unix.Flock(rootFD, unix.LOCK_EX); err != nil {
		return fmt.Errorf("lock state")
	}
	defer unix.Flock(rootFD, unix.LOCK_UN)
	return action()
}

func privateDirectory(directory string) error {
	info, err := os.Lstat(directory)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			return fmt.Errorf("create private state directory")
		}
		if err := os.Chmod(directory, 0o700); err != nil {
			return err
		}
		return syncDirectory(filepath.Dir(directory))
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
	return store.withStateLock(store.cleanupLocked)
}

func (store *store) cleanupLocked() error {
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
