package cache

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestNPMColdWarmAndCorruption(t *testing.T) {
	if os.Getenv("BELLOWS_NPM_INTEGRATION") != "1" {
		t.Skip("opt in to real npm and Node execution")
	}
	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)
	tw := tar.NewWriter(gz)
	for name, data := range map[string]string{"package/package.json": `{"name":"bellows-cache-fixture","version":"1.0.0","main":"index.js"}`, "package/index.js": "module.exports = 42;\n"} {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0644, Size: int64(len(data))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(data)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	var downloads atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/fixture.tgz" {
			http.NotFound(w, r)
			return
		}
		downloads.Add(1)
		_, _ = w.Write(archive.Bytes())
	}))
	defer server.Close()
	sum := sha512.Sum512(archive.Bytes())
	integrity := "sha512-" + base64.StdEncoding.EncodeToString(sum[:])
	manifest := map[string]any{"name": "bellows-cache-test", "version": "1.0.0", "private": true, "dependencies": map[string]string{"bellows-cache-fixture": "1.0.0"}}
	lockfile := map[string]any{"name": "bellows-cache-test", "version": "1.0.0", "lockfileVersion": 3, "requires": true, "packages": map[string]any{"": manifest, "node_modules/bellows-cache-fixture": map[string]any{"version": "1.0.0", "resolved": server.URL + "/fixture.tgz", "integrity": integrity}}}
	run := func(lease *Lease, offline bool) (time.Duration, error) {
		t.Helper()
		project := t.TempDir()
		for name, value := range map[string]any{"package.json": manifest, "package-lock.json": lockfile} {
			data, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(filepath.Join(project, name), data, 0600); err != nil {
				t.Fatal(err)
			}
		}
		for _, name := range []string{"user.npmrc", "global.npmrc"} {
			if err := os.WriteFile(filepath.Join(project, name), nil, 0600); err != nil {
				t.Fatal(err)
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		args := []string{"ci", "--ignore-scripts", "--no-audit", "--no-fund", "--prefer-offline"}
		if offline {
			args = append(args, "--offline")
		}
		cmd := exec.CommandContext(ctx, "npm", args...)
		cmd.Dir = project
		cmd.Env = append([]string{"PATH=" + os.Getenv("PATH"), "HOME=" + project, "npm_config_userconfig=" + filepath.Join(project, "user.npmrc"), "npm_config_globalconfig=" + filepath.Join(project, "global.npmrc")}, lease.Env()...)
		start := time.Now()
		output, err := cmd.CombinedOutput()
		elapsed := time.Since(start)
		if err != nil {
			t.Logf("npm rejected unavailable/corrupt content: %s", output)
			return elapsed, err
		}
		check := exec.CommandContext(ctx, "node", "-p", `require('./node_modules/bellows-cache-fixture')`)
		check.Dir = project
		check.Env = cmd.Env
		result, err := check.Output()
		if err != nil || strings.TrimSpace(string(result)) != "42" {
			t.Fatalf("installed result %q: %v", result, err)
		}
		return elapsed, nil
	}
	path := root(t)
	id := identity()
	cold := open(t, path, id, false)
	coldTime, err := run(cold, false)
	if err != nil {
		t.Fatal(err)
	}
	coldDownloads := downloads.Load()
	if coldDownloads != 1 {
		t.Fatalf("cold downloads=%d", coldDownloads)
	}
	if err = cold.Close(); err != nil {
		t.Fatal(err)
	}
	warm := open(t, path, id, false)
	warmTime, err := run(warm, true)
	if err != nil {
		t.Fatal(err)
	}
	if downloads.Load() != coldDownloads {
		t.Fatal("offline warm run downloaded")
	}
	content := ""
	err = filepath.WalkDir(filepath.Join(warm.path, "npm", "_cacache", "content-v2"), func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			content = path
			return fs.SkipAll
		}
		return nil
	})
	if err != nil || content == "" {
		t.Fatalf("cache content missing: %v", err)
	}
	if err = os.WriteFile(content, []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = run(warm, true); err == nil {
		t.Fatal("corrupt offline cache passed")
	}
	repairTime, err := run(warm, false)
	if err != nil {
		t.Fatal(err)
	}
	if downloads.Load() != coldDownloads+1 {
		t.Fatal("corrupt cache did not refetch")
	}
	bypass := open(t, path, id, true)
	if _, err = run(bypass, false); err != nil {
		t.Fatal(err)
	}
	if downloads.Load() != coldDownloads+2 {
		t.Fatal("explicit cold reused downloads")
	}
	npm, _ := exec.Command("npm", "--version").Output()
	node, _ := exec.Command("node", "--version").Output()
	t.Logf("platform=%s/%s npm=%s node=%s cold=%s downloads=1 warm-offline=%s downloads=0 repair=%s downloads=1 result=42 in every successful run", runtime.GOOS, runtime.GOARCH, strings.TrimSpace(string(npm)), strings.TrimSpace(string(node)), coldTime, warmTime, repairTime)
}
