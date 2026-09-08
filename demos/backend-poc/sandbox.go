package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

// ProblemConfig represents the dynamic metadata pulled directly from PostgreSQL
type ProblemConfig struct {
	ClassName   string   `json:"class_name"`
	MethodName  string   `json:"method_name"`
	TestInputs  []string `json:"test_inputs"`  // Secret inputs for evaluation
	TestOutputs []string `json:"test_outputs"` // Secret outputs for evaluation
}

// RunCodeInSandbox accepts the user's submission string AND the SQL problem config,
// stage-mounts them into Docker, and executes the external runner.py file.
func RunCodeInSandbox(userCode string, config ProblemConfig) (string, bool) {
	// 1. Create a temporary staging folder
	dir, err := os.MkdirTemp("", "sandbox-*")
	if err != nil {
		return "Failed to initialize sandbox environment", false
	}
	defer os.RemoveAll(dir)

	// 2. Write User Code -> solution.py
	err = os.WriteFile(filepath.Join(dir, "solution.py"), []byte(userCode), 0644)
	if err != nil {
		return "Failed to write submission file", false
	}

	// 3. Read local runner.py execution script off the disk
	runnerBytes, err := os.ReadFile("runner.py")
	if err != nil {
		return "System Error: Missing runner.py file in project directory", false
	}

	// Write Harness -> runner.py in temporary staging folder
	err = os.WriteFile(filepath.Join(dir, "runner.py"), runnerBytes, 0644)
	if err != nil {
		return "Failed to write runner harness", false
	}

	// 4. Write SQL Configuration -> meta_config.json
	configBytes, err := json.Marshal(config)
	if err != nil {
		return "Failed to serialize problem metadata", false
	}
	err = os.WriteFile(filepath.Join(dir, "meta_config.json"), configBytes, 0644)
	if err != nil {
		return "Failed to write metadata config file", false
	}

	// 5. Connect to Docker Engine
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return "Engine Connection Error", false
	}
	defer cli.Close()

	// 6. Configure Container to execute runner.py
	containerConfig := &container.Config{
		Image:        "python:3.10-alpine",
		Cmd:          []string{"python", "/app/runner.py"}, // Targets runner.py!
		Tty:          false,
		AttachStdout: true,
		AttachStderr: true,
	}

	hostConfig := &container.HostConfig{
		// Mount the entire temporary directory read-only into /app
		Binds: []string{
			fmt.Sprintf("%s:/app:ro", dir),
		},
		Resources: container.Resources{
			Memory:   128 * 1024 * 1024, // 128MB RAM
			NanoCPUs: 500000000,         // 0.5 CPU Cores
		},
	}

	// 7. Create & Start Container
	resp, err := cli.ContainerCreate(ctx, containerConfig, hostConfig, nil, nil, "")
	if err != nil {
		return "Sandbox Allocation Failed", false
	}
	containerID := resp.ID

	defer func() {
		cli.ContainerRemove(ctx, containerID, types.ContainerRemoveOptions{Force: true})
	}()

	err = cli.ContainerStart(ctx, containerID, types.ContainerStartOptions{})
	if err != nil {
		return "Sandbox Start Failure", false
	}

	// 8. Time Guard & Exit Status Check
	waitCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	statusCh, errCh := cli.ContainerWait(waitCtx, containerID, container.WaitConditionNotRunning)

	var exitCode int64 = -1
	select {
	case err := <-errCh:
		return fmt.Sprintf("Execution Crash Error: %v", err), false
	case status := <-statusCh:
		exitCode = status.StatusCode // Get Python's sys.exit code!
	case <-waitCtx.Done():
		return "TIMEOUT EXCEEDED (TLE): Your code took longer than 2.0 seconds to finish.", false
	}

	// 9. Extract Execution Output Logs
	out, err := cli.ContainerLogs(ctx, containerID, types.ContainerLogsOptions{ShowStdout: true, ShowStderr: true})
	if err != nil {
		return "Log Parsing Error", false
	}
	defer out.Close()

	var stdout, stderr bytes.Buffer
	_, _ = stdcopy.StdCopy(&stdout, &stderr, out)

	fullOutput := stdout.String() + stderr.String()

	// IF EXIT CODE IS NOT 0, IT IS A FAILURE!
	if exitCode != 0 {
		return fullOutput, false
	}

	return fullOutput, true
}
