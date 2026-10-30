package resultcache

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

var ErrBusy = errors.New("result cache is busy")

func openDirectory(path, repository string) (*os.Root, os.FileInfo, error) {
	if path == "" || repository == "" {
		return nil, nil, fmt.Errorf("result cache and repository paths required")
	}
	repository, err := filepath.EvalSymlinks(repository)
	if err != nil {
		return nil, nil, err
	}
	repository, err = filepath.Abs(repository)
	if err != nil {
		return nil, nil, err
	}
	path, err = absoluteDestination(path)
	if err != nil {
		return nil, nil, err
	}
	relative, err := filepath.Rel(repository, path)
	if err != nil || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))) {
		return nil, nil, fmt.Errorf("result cache must be outside repository")
	}
	if err = os.MkdirAll(path, 0700); err != nil {
		return nil, nil, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, nil, err
	}
	if !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return nil, nil, fmt.Errorf("result cache requires a private nonsymlink directory")
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, nil, err
	}
	opened, err := root.Stat(".")
	if err != nil || !os.SameFile(info, opened) {
		_ = root.Close()
		return nil, nil, fmt.Errorf("result cache directory changed")
	}
	return root, info, nil
}

func readPrivate(root *os.Root, name string, maximum int64) ([]byte, error) {
	info, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > maximum {
		return nil, fmt.Errorf("invalid private result cache file")
	}
	file, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return nil, fmt.Errorf("result cache file changed")
	}
	data, err := io.ReadAll(io.LimitReader(file, maximum+1))
	if err != nil || int64(len(data)) > maximum {
		return nil, fmt.Errorf("result cache file exceeds bound")
	}
	return data, nil
}

func atomicCreate(root *os.Root, name string, data []byte) (err error) {
	temporary := ".write-" + rand.Text()
	file, err := root.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, root.Remove(temporary)) }()
	if _, err = file.Write(data); err == nil {
		err = file.Sync()
	}
	err = errors.Join(err, file.Close())
	if err != nil {
		return err
	}
	return root.Link(temporary, name)
}

func locked(root *os.Root) (func() error, error) {
	if err := root.Mkdir(".lock", 0700); err != nil {
		if os.IsExist(err) {
			return nil, ErrBusy
		}
		return nil, err
	}
	return func() error { return root.Remove(".lock") }, nil
}

func absoluteDestination(path string) (string, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	ancestor := filepath.Dir(path)
	for {
		_, err = os.Stat(ancestor)
		if err == nil {
			break
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		ancestor = filepath.Dir(ancestor)
	}
	resolved, err := filepath.EvalSymlinks(ancestor)
	if err != nil {
		return "", err
	}
	suffix, err := filepath.Rel(ancestor, path)
	if err != nil {
		return "", err
	}
	return filepath.Join(resolved, suffix), nil
}
