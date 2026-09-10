package repolock

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/unix"
)

func With(ctx context.Context, stateRoot string, action func() error) error {
	if err := os.MkdirAll(stateRoot, 0o700); err != nil {
		return fmt.Errorf("create repository state")
	}
	if err := os.Chmod(stateRoot, 0o700); err != nil {
		return fmt.Errorf("secure repository state")
	}
	lockFD, err := unix.Open(filepath.Join(stateRoot, "inbox.lock"), unix.O_RDWR|unix.O_CREAT|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0o600)
	if err != nil {
		return fmt.Errorf("open repository lock")
	}
	lock := os.NewFile(uintptr(lockFD), "inbox.lock")
	if lock == nil {
		_ = unix.Close(lockFD)
		return fmt.Errorf("open repository lock")
	}
	defer lock.Close()
	for {
		err := unix.Flock(lockFD, unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			break
		}
		if !errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EAGAIN) {
			return fmt.Errorf("lock repository")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Millisecond):
		}
	}
	defer unix.Flock(lockFD, unix.LOCK_UN)
	return action()
}
