package deploy

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestIntegrationAllClearsEnvironmentAfterTestFailure(t *testing.T) {
	s3Listener := listenLocal(t)
	defer s3Listener.Close()

	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "lifecycle.log")
	dockerPath := writeExecutable(t, tempDir, "docker", `#!/usr/bin/env bash
printf 'docker' >> "$INTEGRATION_TEST_LOG"
printf ' %s' "$@" >> "$INTEGRATION_TEST_LOG"
printf '\n' >> "$INTEGRATION_TEST_LOG"
if [[ "$*" == *"--format"* ]]; then
  printf '%s\n' "$INTEGRATION_ENV_NAME"
fi
exit 0
`)
	runnerPath := writeExecutable(t, tempDir, "runner", `#!/usr/bin/env bash
printf 'runner\n' >> "$INTEGRATION_TEST_LOG"
exit 23
`)

	envName := fmt.Sprintf("xbh-lifecycle-%d", os.Getpid())
	cmd := exec.Command("../scripts/integration-all.sh")
	cmd.Env = append(os.Environ(),
		"INTEGRATION_DOCKER_BIN="+dockerPath,
		"INTEGRATION_TEST_RUNNER="+runnerPath,
		"INTEGRATION_TEST_LOG="+logPath,
		"INTEGRATION_ENV_NAME="+envName,
		fmt.Sprintf("INTEGRATION_S3_PORT=%d", s3Listener.Addr().(*net.TCPAddr).Port),
		"INTEGRATION_WAIT_SECONDS=1",
		"INTEGRATION_WAIT_INTERVAL=1",
	)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("integration-all succeeded despite runner failure; output:\n%s", output)
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok || exitErr.ExitCode() != 23 {
		t.Fatalf("integration-all exit = %v, want runner status 23; output:\n%s", err, output)
	}

	logBody, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	_, afterRunner, found := strings.Cut(string(logBody), "runner\n")
	if !found {
		t.Fatalf("runner marker not found in lifecycle log:\n%s", logBody)
	}
	for _, command := range []string{
		"docker rm --force " + envName + "-seaweedfs",
		"docker network rm " + envName + "-network",
	} {
		if !strings.Contains(afterRunner, command) {
			t.Errorf("cleanup command %q did not run after test failure; log:\n%s", command, logBody)
		}
	}
}

func listenLocal(t *testing.T) net.Listener {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return listener
}

func writeExecutable(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}
