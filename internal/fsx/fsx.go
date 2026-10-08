// Package fsx holds the file operations every writer in cadre shares:
// atomic writes, locks, and copying a folder tree with a check that the
// copy is complete.
package fsx

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"golang.org/x/sys/unix"
)

// WriteFile replaces path with data atomically: a temporary file in the
// same folder is written, synced and given perm, then renamed over path. A
// reader sees the old file or the new one, never a part.
func WriteFile(path string, data []byte, perm fs.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".cadre-")
	if err != nil {
		return err
	}
	name := tmp.Name()
	ok := false
	defer func() {
		if !ok {
			tmp.Close()
			os.Remove(name)
		}
	}()
	if err := tmp.Chmod(perm); err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, path); err != nil {
		return err
	}
	ok = true
	return nil
}

// ErrBusy is returned when another cadre command holds a lock for longer
// than the wait.
var ErrBusy = errors.New("another cadre command is using it")

// Lock is an exclusive lock held by this process.
type Lock struct {
	f   *os.File // flock on this file
	dir string   // or, where flock is not supported, this mkdir lock
}

// staleAfter is how old a mkdir lock must be before it is taken over. It
// only applies on filesystems without flock; a flock is released by the
// kernel when its holder exits.
const staleAfter = 60 * time.Second

// forceDirLock makes Acquire use the mkdir lock, for tests.
var forceDirLock = false

// Acquire takes the lock at path (a file it creates if needed), waiting up
// to wait for another holder. The holder's pid is written into the file
// for messages. On a filesystem without flock (some network filesystems),
// it falls back to a mkdir lock at path + ".d", taken over when older than
// a minute.
func Acquire(path string, wait time.Duration) (*Lock, error) {
	if !forceDirLock {
		f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
		if err != nil {
			return nil, err
		}
		deadline := time.Now().Add(wait)
		for {
			err = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
			if err == nil {
				// The pid is for people reading the file; the lock is the flock.
				if f.Truncate(0) == nil {
					f.WriteAt([]byte(strconv.Itoa(os.Getpid())+"\n"), 0)
				}
				return &Lock{f: f}, nil
			}
			if !errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EINTR) {
				f.Close()
				if unsupported(err) {
					break
				}
				return nil, fmt.Errorf("lock %s: %w", path, err)
			}
			if time.Now().After(deadline) {
				f.Close()
				return nil, ErrBusy
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	return acquireDir(path+".d", wait)
}

func unsupported(err error) bool {
	return errors.Is(err, unix.ENOTSUP) || errors.Is(err, unix.EOPNOTSUPP) || errors.Is(err, unix.ENOLCK)
}

func acquireDir(dir string, wait time.Duration) (*Lock, error) {
	deadline := time.Now().Add(wait)
	for {
		err := os.Mkdir(dir, 0o700)
		if err == nil {
			return &Lock{dir: dir}, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return nil, fmt.Errorf("lock %s: %w", dir, err)
		}
		if st, err := os.Stat(dir); err == nil && time.Since(st.ModTime()) > staleAfter {
			// Rename it aside under a unique name, which only one process
			// can do, then retry; it is never renamed back.
			aside := fmt.Sprintf("%s.stale-%d-%d", dir, os.Getpid(), time.Now().UnixNano())
			if os.Rename(dir, aside) == nil {
				os.Remove(aside)
			}
			continue
		} else if err != nil && errors.Is(err, fs.ErrNotExist) {
			continue // released between the mkdir and the stat
		}
		if time.Now().After(deadline) {
			return nil, ErrBusy
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// Release gives the lock up. The flock file stays (an empty lock is
// harmless and avoids a race between removing it and another open).
func (l *Lock) Release() error {
	if l.f != nil {
		unix.Flock(int(l.f.Fd()), unix.LOCK_UN)
		return l.f.Close()
	}
	if err := os.Remove(l.dir); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// CopyTree copies the folder src to dst, which must not exist: folders,
// regular files (with their modes) and symlinks (with their targets, not
// followed). skip, when set, leaves out a path relative to src and
// everything under it. Other file types are an error.
func CopyTree(src, dst string, skip func(rel string) bool) error {
	if _, err := os.Lstat(dst); err == nil {
		return fmt.Errorf("%s already exists", dst)
	}
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		if rel != "." && skip != nil && skip(rel) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		to := filepath.Join(dst, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		switch {
		case d.Type()&fs.ModeSymlink != 0:
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(target, to)
		case d.IsDir():
			return os.Mkdir(to, info.Mode().Perm()|0o700)
		case d.Type().IsRegular():
			return copyFile(path, to, info.Mode().Perm())
		}
		return fmt.Errorf("%s is not a file, folder or symlink", path)
	})
}

func copyFile(from, to string, perm fs.FileMode) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// An entry of a tree listing: what VerifyTree compares.
type entry struct {
	kind   string // "dir", "file" or "link"
	size   int64
	perm   fs.FileMode
	target string
}

func list(root string, skip func(rel string) bool) (map[string]entry, error) {
	out := map[string]entry{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if rel != "." && skip != nil && skip(rel) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		e := entry{perm: info.Mode().Perm()}
		switch {
		case d.Type()&fs.ModeSymlink != 0:
			e.kind, e.perm = "link", 0
			if e.target, err = os.Readlink(path); err != nil {
				return err
			}
		case d.IsDir():
			e.kind, e.perm = "dir", 0
		default:
			e.kind, e.size = "file", info.Size()
		}
		out[rel] = e
		return nil
	})
	return out, err
}

// VerifyTree checks that dst holds the same tree as src (with the same
// skip): the same paths, kinds, file sizes and modes, and symlink targets.
func VerifyTree(src, dst string, skip func(rel string) bool) error {
	a, err := list(src, skip)
	if err != nil {
		return err
	}
	b, err := list(dst, nil)
	if err != nil {
		return err
	}
	for rel, e := range a {
		got, ok := b[rel]
		if !ok {
			return fmt.Errorf("the copy lacks %s", rel)
		}
		if got != e {
			return fmt.Errorf("the copy of %s differs (%+v, want %+v)", rel, got, e)
		}
	}
	for rel := range b {
		if _, ok := a[rel]; !ok {
			return fmt.Errorf("the copy has an extra %s", rel)
		}
	}
	return nil
}
