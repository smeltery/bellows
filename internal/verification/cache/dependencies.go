package cache

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"
)

// Dependencies fingerprints committed npm inputs, not source files or installed trees.
func Dependencies(preparedRoot string) (string, error) {
	cmd := exec.Command("git", "-C", preparedRoot, "ls-tree", "-r", "-z", "HEAD")
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME"), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1"}
	data, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("cannot resolve committed dependency inputs: %w", err)
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte("bellows-npm-inputs-v1\x00"))
	for _, entry := range strings.Split(strings.TrimSuffix(string(data), "\x00"), "\x00") {
		if entry == "" {
			continue
		}
		metadata, name, ok := strings.Cut(entry, "\t")
		fields := strings.Fields(metadata)
		if !ok || len(fields) != 3 {
			return "", fmt.Errorf("invalid Git tree entry")
		}
		if strings.Contains("/"+name+"/", "/node_modules/") {
			continue
		}
		switch path.Base(name) {
		case "package.json", "package-lock.json", "npm-shrinkwrap.json", ".npmrc":
		default:
			if fields[0] != "160000" {
				continue
			}
		}
		if fields[0] == "120000" {
			return "", fmt.Errorf("symlink dependency input is unsupported")
		}
		_, _ = hash.Write([]byte(name + "\x00" + fields[2] + "\x00"))
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}
