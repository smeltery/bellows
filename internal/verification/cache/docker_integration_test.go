package cache

import (
	"bytes"
	"context"
	"crypto/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/smeltery/bellows/internal/config"
	"github.com/smeltery/bellows/internal/verification/actexec"
)

func TestDockerCacheIntegration(t *testing.T) {
	if os.Getenv("BELLOWS_SERVICE_INTEGRATION") != "1" {
		t.Skip("requires opt-in privileged Docker")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	docker := func(args ...string) (string, string) {
		t.Helper()
		cmd := exec.CommandContext(ctx, "docker", args...)
		cmd.Env = append(os.Environ(), "DOCKER_BUILDKIT=1")
		var out, log bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &log
		if err := cmd.Run(); err != nil {
			t.Fatalf("docker %v: %v\n%s\n%s", args, err, &out, &log)
		}
		return strings.TrimSpace(out.String()), log.String()
	}
	name := "bellows-build-cache-" + strings.ToLower(rand.Text())
	id, _ := docker("run", "--detach", "--privileged", "--name", name, "--label", "bellows.fixture="+name, "--env", "DOCKER_TLS_CERTDIR=", "--publish", "127.0.0.1::2375", "docker:28-dind")
	t.Cleanup(func() {
		if output, err := exec.Command("docker", "rm", "--force", id).CombinedOutput(); err != nil {
			t.Errorf("owned daemon cleanup: %v %s", err, output)
		}
	})
	port, _ := docker("port", id, "2375/tcp")
	endpoint := "tcp://" + port
	for {
		if err := exec.CommandContext(ctx, "docker", "--host", endpoint, "info").Run(); err == nil {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("daemon not ready")
		case <-time.After(time.Second):
		}
	}
	e, err := actexec.New(ctx, config.DefaultConfig(), endpoint, root(t))
	if err != nil {
		t.Fatal(err)
	}
	bindings := root(t)
	if err = BindDaemon(bindings, e.DaemonID(), "owner/project"); err != nil {
		t.Fatal(err)
	}
	project := t.TempDir()
	for path, data := range map[string]string{"Dockerfile": "FROM scratch\nCOPY result /result\n", "result": "synthetic=42\n"} {
		if err = os.WriteFile(filepath.Join(project, path), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	start := time.Now()
	_, coldLog := docker("--host", endpoint, "build", "--no-cache", "--progress=plain", "--tag", "bellows-cache:synthetic", project)
	coldTime := time.Since(start)
	coldID, _ := docker("--host", endpoint, "image", "inspect", "bellows-cache:synthetic", "--format", "{{.Id}}")
	start = time.Now()
	_, warmLog := docker("--host", endpoint, "build", "--progress=plain", "--tag", "bellows-cache:synthetic", project)
	warmTime := time.Since(start)
	warmID, _ := docker("--host", endpoint, "image", "inspect", "bellows-cache:synthetic", "--format", "{{.Id}}")
	if coldID != warmID || !strings.Contains(warmLog, "CACHED") {
		t.Fatalf("cache result changed or no warm hit\ncold:%s\nwarm:%s", coldLog, warmLog)
	}
	if err = BindDaemon(bindings, e.DaemonID(), "other/project"); err == nil {
		t.Fatal("cross-repository layer reuse accepted")
	}
	if err = e.Close(); err != nil {
		t.Fatal("build cache added containers/volumes", err)
	}
	t.Logf("dedicated Linux Docker BuildKit cold=%s warm=%s warm-hit=CACHED image=%s identical=true; no containers/volumes/custom networks remain", coldTime, warmTime, warmID)
}
