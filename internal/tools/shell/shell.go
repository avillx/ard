package shell

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	rt "runtime"
	"strings"
	"time"

	"ard/internal/agent"
	"ard/internal/tools"
)

const defaultTimeout = 30 * time.Second

const (
	// edge timeout of the shell tool, set in runtime.
	shellToolTimeout = 15 * time.Minute
	// how long to wait for the child to exit after the process group kill.
	processGroupKillGracePeriod = 3 * time.Second
	// how long to wait for a spawned orphan to be reaped.
	orphanReapGracePeriod = 2 * time.Second
)

type SecretsSource interface {
	All() map[string]string
}

type ToolServer struct {
	*tools.BuildInToolServer

	instruction string
}

func NewToolServer(
	cwd string,
	secretsSource SecretsSource,
	shell string,
	shellAttributes []string,
) *ToolServer {
	if shell == "" {
		shell = defaultShell
	}

	if shell == defaultShell && len(shellAttributes) <= 0 {
		shellAttributes = defaultShellAttributes
	}

	return &ToolServer{
		instruction: renderInstruction(rt.GOOS, shell),
		BuildInToolServer: tools.NewBuildInToolServer(
			&Tool{
				secretsSource:   secretsSource,
				cwd:             cwd,
				shell:           shell,
				shellAttributes: shellAttributes,
			},
		),
	}
}

func (t *ToolServer) Instruction() string {
	return t.instruction
}

func renderInstruction(os, shell string) string {
	return `## Shell Tool

- Current OS is "` + os + `."
- Your shell is "` + shell + `, command should be available in this shell"
- Each call runs in a fresh shell session — no state persists between calls
- Default timeout: 30s. Set "timeout" for longer operations (builds, tests)
- Use "cwd" parameter instead of cd within commands
- Combine operations with pipes, redirections, and heredocs
- Non-zero exit codes return error info with output; timed-out commands are terminated
- NEVER try to read env directly, just ask user env name if you need one.`
}

var _ agent.Tool = (*Tool)(nil)

type Tool struct {
	secretsSource   SecretsSource
	cwd             string
	shell           string
	shellAttributes []string
}

func (t *Tool) Name() agent.ToolName {
	return "shell"
}

// TimeOut this is edge timeout, sets in runtime.
func (t *Tool) TimeOut() time.Duration {
	return shellToolTimeout
}

func (t *Tool) Description() string {
	return "access to shell"
}

func (t *Tool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "Full shell command for execution",
			},
			"cwd": map[string]any{
				"type":        "string",
				"description": fmt.Sprintf("Working directory (default %s)", t.cwd),
			},
			"timeout": map[string]any{
				"type":        "number",
				"description": "Timeout in seconds",
			},
		},
		"required": []string{"command"},
	}
}

const waitDelayAfterShellExit = 5 * time.Second

func (t *Tool) Call(ctx context.Context, rawArgs agent.ToolArguments) ([]agent.ContentPart, error) {
	args, err := tools.UnwrapArgs[struct {
		Command string `json:"command"`
		Timeout int    `json:"timeout,omitempty"`
		Cwd     string `json:"cwd,omitempty"`
	}](rawArgs)
	if err != nil {
		return nil, err
	}

	// envirement variables for agent shell calls
	secrets := []string{}
	for k, v := range t.secretsSource.All() {
		secrets = append(secrets, fmt.Sprintf("%s=%s", k, v))
	}

	envs := os.Environ()
	envs = append(envs, secrets...)
	envs = append(envs, metadataToEnv(ctx)...)

	cmd := exec.CommandContext(ctx, t.shell, append(t.shellAttributes, args.Command)...) //nolint: gosec
	cmd.Dir = t.cwd
	if args.Cwd != "" {
		cmd.Dir = args.Cwd
	}
	cmd.Env = envs
	cmd.SysProcAttr = platformSpecificSysProcAttr()
	cmd.WaitDelay = waitDelayAfterShellExit

	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	if err := cmd.Start(); err != nil {
		return tools.Result(fmt.Sprintf("Error starting command: %s", err)), err
	}

	pg, err := createProcessGroup(cmd.Process)
	if err != nil {
		// Successfully started the child but couldn't install it in its own
		// process group: clean it up before bailing out.
		reapSpawnedChild(cmd, pg)
		return tools.Result(fmt.Sprintf("Error creating process group: %s", err)), err
	}

	//
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	// timeout
	to := defaultTimeout
	if args.Timeout > 0 {
		to = time.Duration(args.Timeout) * time.Second
	}

	ctx, cancel := context.WithDeadline(ctx, time.Now().Add(to))
	defer cancel()

	// wait for
	var cmdErr error
	select {
	case <-ctx.Done():
		_ = kill(cmd.Process, pg)

		select {
		case cmdErr = <-done:
		case <-time.After(processGroupKillGracePeriod):
			_ = cmd.Process.Kill()
			cmdErr = <-done
		}
	case cmdErr = <-done:
	}

	formattedOutput := formatCommandOutput(ctx, cmdErr, output.String(), to)
	return tools.Result(formattedOutput), nil
}

// helpers.
func formatCommandOutput(ctx context.Context, err error, output string, timeout time.Duration) string {
	var sb strings.Builder

	sb.WriteString(output)

	if ctxErr := ctx.Err(); ctxErr != nil {
		if errors.Is(ctxErr, context.Canceled) {
			sb.WriteString("Command cancelled\n")
		}
		if errors.Is(ctxErr, context.DeadlineExceeded) {
			fmt.Fprintf(&sb, "Command timed out after %v", timeout)
		}
	}

	if err != nil {
		fmt.Fprintf(&sb, "Error executing command: %s", err)
	}

	return cmp.Or(strings.TrimSpace(sb.String()), "<no output>")
}

func reapSpawnedChild(cmd *exec.Cmd, pg *processGroup) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = kill(cmd.Process, pg)

	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(orphanReapGracePeriod):
		_ = cmd.Process.Kill()
		<-done
	}
}

func metadataToEnv(ctx context.Context) []string {
	agentID := tools.MustAgentID(ctx)
	sessionID := tools.MustSessionID(ctx)

	return []string{
		fmt.Sprintf("ARD_AGENT_ID=%s", agentID),
		fmt.Sprintf("ARD_AGENT_SESSION_ID=%s", sessionID),
	}
}
