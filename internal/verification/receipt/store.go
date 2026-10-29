package receipt

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	v "github.com/smeltery/bellows/internal/verification"
)

// Save writes a completed receipt once in an existing private directory outside
// the worktree. The path is a content location, never evidence of trusted origin.
func Save(worktree, directory string, r v.Receipt) (path string, err error) {
	data, err := Encode(r)
	if err != nil {
		return "", err
	}
	if r.CompletedAt == nil {
		return "", ErrIncomplete
	}
	worktree, err = filepath.Abs(worktree)
	if err != nil {
		return "", err
	}
	worktree, err = filepath.EvalSymlinks(worktree)
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(directory)
	if err != nil {
		return "", err
	}
	if !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return "", fmt.Errorf("receipt directory must be private and not a symlink")
	}
	directory, err = filepath.Abs(directory)
	if err != nil {
		return "", err
	}
	directory, err = filepath.EvalSymlinks(directory)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(worktree, directory)
	if err != nil || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))) {
		return "", fmt.Errorf("receipt directory must be outside the worktree")
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return "", err
	}
	defer root.Close()
	opened, err := root.Stat(".")
	if err != nil {
		return "", err
	}
	if !os.SameFile(info, opened) || opened.Mode().Perm()&0077 != 0 {
		return "", fmt.Errorf("receipt directory changed while opening")
	}
	name := ".receipt-" + rand.Text()
	file, err := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", err
	}
	defer func() {
		if cleanupErr := root.Remove(name); err == nil {
			err = cleanupErr
		}
	}()
	if _, err = file.Write(data); err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return "", err
	}
	filename := fmt.Sprintf("%x.json", sha256.Sum256([]byte(r.RunID)))
	if err = root.Link(name, filename); err != nil {
		return "", err
	}
	return filepath.Join(directory, filename), nil
}
