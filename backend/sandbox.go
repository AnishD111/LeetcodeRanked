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

func RunCodeInSandbox(userCode string, config JudgeConfig) (string, bool) {
	dir, err := os.MkdirTemp("", "sandbox-*")
	if err != nil {
		return "Failed to initialize sandbox environment", false
	}
	defer os.RemoveAll(dir)

	err = os.WriteFile(filepath.Join(dir, "solution.py"), []byte(userCode), 0644)
	if err != nil {
		return "Failed to write submission file", false
	}

	runnerBytes, err := os.ReadFile("runner.py")
	if err != nil {
		return "System Error: Missing runner.py file in project directory", false
	}
	err = os.WriteFile(filepath.Join(dir, "runner.py"), runnerBytes, 0644)
	if err != nil {
		return "Failed to write runner harness", false
	}

	configBytes, err := json.Marshal(config)
	if err != nil {
		return "Failed to serialize problem metadata", false
	}
	err = os.WriteFile(filepath.Join(dir, "meta_config.json"), configBytes, 0644)
	if err != nil {
		return "Failed to write metadata config file", false
	}

	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return "Engine Connection Error: cannot reach Docker daemon", false
	}
	defer cli.Close()

	containerConfig := &container.Config{
		Image:        "python:3.10-alpine",
		Cmd:          []string{"python", "/app/runner.py"},
		AttachStdout: true,
		AttachStderr: true,
	}

	hostConfig := &container.HostConfig{
		Binds: []string{
			fmt.Sprintf("%s:/app:ro", dir),
		},
		Resources: container.Resources{
			Memory:   128 * 1024 * 1024,
			NanoCPUs: 500_000_000,
		},
	}

	resp, err := cli.ContainerCreate(ctx, containerConfig, hostConfig, nil, nil, "")
	if err != nil {
		return fmt.Sprintf("Failed to initialize sandbox container: %v", err), false
	}
	containerID := resp.ID

	defer func() {
		removeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cli.ContainerRemove(removeCtx, containerID, container.RemoveOptions{Force: true})
	}()

	if err := cli.ContainerStart(ctx, containerID, container.StartOptions{}); err != nil {
		return fmt.Sprintf("Failed to start evaluation container: %v", err), false
	}

	waitCtx, cancelWait := context.WithTimeout(ctx, 2*time.Second)
	defer cancelWait()

	statusCh, errCh := cli.ContainerWait(waitCtx, containerID, container.WaitConditionNotRunning)

	select {
	case err := <-errCh:
		if err != nil {
			return fmt.Sprintf("Evaluation runtime error: %v", err), false
		}
	case <-statusCh:
	case <-waitCtx.Done():
		_ = cli.ContainerKill(context.Background(), containerID, "SIGKILL")
		return "Time Limit Exceeded: Process terminated after 2.0s", false
	}

	out, err := cli.ContainerLogs(ctx, containerID, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
	})
	if err != nil {
		return fmt.Sprintf("Failed to read sandbox output: %v", err), false
	}
	defer out.Close()

	var stdout, stderr bytes.Buffer
	_, err = stdcopy.StdCopy(&stdout, &stderr, out)
	if err != nil {
		return fmt.Sprintf("Failed to parse output streams: %v", err), false
	}

	fullLog := stdout.String()
	if stderr.Len() > 0 {
		fullLog += "\n" + stderr.String()
	}

	inspect, err := cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return fmt.Sprintf("Failed to inspect container: %v", err), false
	}

	passed := inspect.State.ExitCode == 0
	return fullLog, passed
}
