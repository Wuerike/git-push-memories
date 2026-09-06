package memory

import (
	"context"
	"errors"
	"time"

	"golang.org/x/sys/unix"
)

func (repository *Repository) withLock(ctx context.Context, exclusive bool, action func() error) error {
	repository.mutex.Lock()
	defer repository.mutex.Unlock()

	operation := unix.LOCK_SH
	if exclusive {
		operation = unix.LOCK_EX
	}
	for {
		err := unix.Flock(int(repository.root.Fd()), operation|unix.LOCK_NB)
		if err == nil {
			break
		}
		if !errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EAGAIN) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Millisecond):
		}
	}
	defer unix.Flock(int(repository.root.Fd()), unix.LOCK_UN)
	return action()
}
