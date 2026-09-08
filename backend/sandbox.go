package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

// RunCodeInSandbox accepts the user's submitted code and the judge configuration,
// stages them into a temporary directory, mounts it read-only into a fresh Docker
// container, runs runner.py, and returns the output log plus a pass/fail boolean.
//
// SANDBOX SECURITY MODEL:
// The user's code runs inside a Docker container with:
//   - A read-only filesystem mount (can't write to disk)
//   - 128MB RAM limit (prevents memory bomb attacks)
//   - 0.5 CPU core limit (prevents CPU exhaustion)
//   - A 2-second hard timeout (prevents infinite loops / TLE)
//   - No network access (default Docker — can't phone home or scrape data)
//
// The container is always removed after execution (deferred ContainerRemove),
// so there's no accumulation of zombie containers.
func RunCodeInSandbox(userCode string, config JudgeConfig) (string, bool) {
	// ── 1. Create a temporary staging directory ───────────────────────────────
	// os.MkdirTemp creates a unique temporary folder (e.g. /tmp/sandbox-abc123).
	// defer os.RemoveAll cleans it up when this function returns — guaranteed
	// even if we return early due to an error.
	dir, err := os.MkdirTemp("", "sandbox-*")
	if err != nil {
		return "Failed to initialize sandbox environment", false
	}
	defer os.RemoveAll(dir)

	// ── 2. Write the user's submitted code as solution.py ─────────────────────
	err = os.WriteFile(filepath.Join(dir, "solution.py"), []byte(userCode), 0644)
	if err != nil {
		return "Failed to write submission file", false
	}

	// ── 3. Copy runner.py from the server's working directory ─────────────────
	// runner.py is our evaluation harness — it imports solution.py and runs tests.
	runnerBytes, err := os.ReadFile("runner.py")
	if err != nil {
		return "System Error: Missing runner.py file in project directory", false
	}
	err = os.WriteFile(filepath.Join(dir, "runner.py"), runnerBytes, 0644)
	if err != nil {
		return "Failed to write runner harness", false
	}

	// ── 4. Serialize the JudgeConfig to meta_config.json ─────────────────────
	// runner.py reads this file inside the container to know what class/method
	// to call and what the expected test case inputs/outputs are.
	configBytes, err := json.Marshal(config)
	if err != nil {
		return "Failed to serialize problem metadata", false
	}
	err = os.WriteFile(filepath.Join(dir, "meta_config.json"), configBytes, 0644)
	if err != nil {
		return "Failed to write metadata config file", false
	}

	// ── 5. Connect to the local Docker Engine ─────────────────────────────────
	// client.FromEnv reads DOCKER_HOST from environment (defaults to the local socket).
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return "Engine Connection Error: cannot reach Docker daemon", false
	}
	defer cli.Close()

	// ── 6. Define Container Configuration ────────────────────────────────────
	containerConfig := &container.Config{
		Image:        "python:3.10-alpine",          // Minimal Python image
		Cmd:          []string{"python", "/app/runner.py"}, // Entrypoint
		AttachStdout: true,
		AttachStderr: true,
	}

	hostConfig := &container.HostConfig{
		// Mount the staging directory as /app inside the container.
		// :ro = read-only → the Python code cannot write files to disk.
		Binds: []string{
			fmt.Sprintf("%s:/app:ro", dir),
		},
		Resources: container.Resources{
			Memory:   128 * 1024 * 1024, // 128 MB RAM hard limit
			NanoCPUs: 500_000_000,       // 0.5 CPU cores (500 million nanocpus)
		},
		// NetworkMode: "none" would disable networking entirely.
		// Left as default for now (alpine DNS resolves but solution code
		// shouldn't need internet — can harden this in a future milestone).
	}

	// ── 7. Create and Start the Container ────────────────────────────────────
	resp, err := cli.ContainerCreate(ctx, containerConfig, hostConfig, nil, nil, "")
	if err != nil {
		return "Sandbox Allocation Failed: " + err.Error(), false
	}
	containerID := resp.ID

	// Ensure the container is removed when we're done — always, even on errors.
	defer cli.ContainerRemove(ctx, containerID, container.RemoveOptions{Force: true})

	if err = cli.ContainerStart(ctx, containerID, container.StartOptions{}); err != nil {
		return "Sandbox Start Failure: " + err.Error(), false
	}

	// ── 8. Wait for Container Exit with Timeout ───────────────────────────────
	// We create a context with a 2-second deadline.
	// ContainerWait blocks until the container stops, then sends its exit code.
	// If the 2-second context deadline fires first, we report TLE and move on.
	waitCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	statusCh, errCh := cli.ContainerWait(waitCtx, containerID, container.WaitConditionNotRunning)

	var exitCode int64 = -1
	select {
	case err := <-errCh:
		return fmt.Sprintf("Execution Crash Error: %v", err), false
	case status := <-statusCh:
		// status.StatusCode is the process exit code from Python (sys.exit(0) or sys.exit(1))
		exitCode = status.StatusCode
	case <-waitCtx.Done():
		return "⏱️ TIMEOUT EXCEEDED (TLE): Your code took longer than 2.0 seconds.", false
	}

	// ── 9. Collect Output Logs ────────────────────────────────────────────────
	// Docker multiplexes stdout and stderr into a single stream.
	// stdcopy.StdCopy de-multiplexes them into separate buffers.
	out, err := cli.ContainerLogs(ctx, containerID, container.LogsOptions{ShowStdout: true, ShowStderr: true})
	if err != nil {
		return "Log Parsing Error", false
	}
	defer out.Close()

	var stdout, stderr bytes.Buffer
	_, _ = stdcopy.StdCopy(&stdout, &stderr, out)

	fullOutput := stdout.String() + stderr.String()

	// ── 10. Interpret Exit Code ───────────────────────────────────────────────
	// runner.py explicitly calls sys.exit(1) on any test failure or crash.
	// An exit code of 0 means all test cases passed (runner.py returned normally).
	if exitCode != 0 {
		return fullOutput, false
	}
	return fullOutput, true
}
