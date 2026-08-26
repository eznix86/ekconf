package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/eznix86/ekconf/internal/config"
	"github.com/spf13/cobra"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

var execNoShell bool

type execRequest struct {
	contextName string
	commandArgs []string
}

func wipeFile(path string) error {
	file, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return err
	}

	size := info.Size()
	if size > 0 {
		buf := make([]byte, 4096)
		for written := int64(0); written < size; written += int64(len(buf)) {
			rem := size - written
			if rem < int64(len(buf)) {
				buf = make([]byte, rem)
			}
			writtenBytes, err := file.Write(buf)
			if err != nil {
				return err
			}
			if writtenBytes != len(buf) {
				return fmt.Errorf("wipe file: short write")
			}
		}
		if err := file.Sync(); err != nil {
			return err
		}
	}

	return nil
}

func tempDir() string {
	if fi, err := os.Stat("/dev/shm"); err == nil && fi.IsDir() {
		return "/dev/shm"
	}
	return os.TempDir()
}

var execCmd = &cobra.Command{
	Use:   "exec [<name>] -- <cmd>",
	Short: "Run a command with decrypted config injected via KUBECONFIG",
	Long: `Run a command with the decrypted kubeconfig for a specific context injected
via the KUBECONFIG environment variable. The temp file is wiped and deleted
on normal exit and on SIGINT/SIGTERM. SIGKILL, power loss, and kernel panic can still orphan the file.

On Linux the temp file is written to /dev/shm (RAM-backed tmpfs). On macOS
/dev/shm does not exist, so the temp file is written to the system temp dir
(on disk, not RAM-backed) with 0600 permissions inside a 0700 directory.

If no context name is given, the active context from config.yaml is used.
Use -- to separate the context name from the command.`,
	Example: `  ekconf exec -- kubectl get pods
  ekconf exec staging -- kubectl get pods
  ekconf exec --no-shell -- sh -c "echo $KUBECONFIG"`,
	Args:              validateExecArgs,
	ValidArgsFunction: completeContext,
	RunE: func(cmd *cobra.Command, args []string) (retErr error) {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}

		req, err := parseExecRequest(cmd, cfg, args)
		if err != nil {
			return err
		}

		password, err := resolvePassword(cmd.Context())
		if err != nil {
			return err
		}
		defer clear(password)

		storePasswordIfNeeded(cmd.ErrOrStderr(), password)

		out, err := decryptedContextKubeconfig(cfg, req.contextName, password)
		if err != nil {
			return err
		}

		tmpPath, cleanupTemp, err := writeTempKubeconfig(out)
		clear(out)
		if err != nil {
			return err
		}
		var cleanupOnce sync.Once

		defer func() {
			cleanupOnce.Do(func() {
				if err := cleanupTemp(); err != nil {
					if retErr == nil {
						retErr = fmt.Errorf("cleanup temp kubeconfig: %w", err)
						return
					}
					fmt.Fprintf(cmd.ErrOrStderr(), "Warning: cleanup temp kubeconfig failed: %v\n", err)
				}
			})
		}()

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigCh
			signal.Stop(sigCh)
			cleanupOnce.Do(func() {
				_ = cleanupTemp()
			})
			os.Exit(1)
		}()

		c := buildExecCommand(cmd.Context(), req.commandArgs, tmpPath)
		c.Stdin = os.Stdin
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr

		if err := c.Run(); err != nil {
			if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
				return &exitCodeError{code: exitErr.ExitCode(), msg: fmt.Sprintf("command exited with status %d", exitErr.ExitCode())}
			}
			return fmt.Errorf("execute command: %w", err)
		}

		return nil
	},
}

func parseExecRequest(cmd *cobra.Command, cfg *config.Config, args []string) (execRequest, error) {
	dash := cmd.Flags().ArgsLenAtDash()
	if dash == 1 {
		if !cfg.ContextExists(args[0]) {
			if cfg.Current == "" {
				return execRequest{}, fmt.Errorf("no active context set and no context name provided")
			}
			return execRequest{}, fmt.Errorf("unknown context: %s", args[0])
		}
		return execRequest{contextName: args[0], commandArgs: args[1:]}, nil
	}

	commandArgs := args
	contextName := ""
	if len(args) >= 1 && cfg.ContextExists(args[0]) {
		contextName = args[0]
		commandArgs = args[1:]
	}
	if len(commandArgs) == 0 {
		return execRequest{}, fmt.Errorf("no command specified, use: ekconf exec [<name>] -- <cmd>")
	}
	if contextName == "" {
		if cfg.Current == "" {
			return execRequest{}, fmt.Errorf("no active context set and no context name provided")
		}
		contextName = cfg.Current
	}

	return execRequest{contextName: contextName, commandArgs: commandArgs}, nil
}

func decryptedContextKubeconfig(cfg *config.Config, contextName string, password []byte) ([]byte, error) {
	kubeconfig, err := loadDecryptedKubeconfig(password)
	if err != nil {
		return nil, err
	}

	singleCtx, err := singleContextKubeconfig(cfg, kubeconfig, contextName)
	if err != nil {
		return nil, err
	}

	out, err := clientcmd.Write(*singleCtx)
	if err != nil {
		return nil, fmt.Errorf("marshal kubeconfig: %w", err)
	}
	return out, nil
}

func singleContextKubeconfig(
	cfg *config.Config,
	kubeconfig *clientcmdapi.Config,
	contextName string,
) (*clientcmdapi.Config, error) {
	contextEntry, ok := kubeconfig.Contexts[contextName]
	if !ok || contextEntry == nil {
		return nil, fmt.Errorf("context '%s' not found", contextName)
	}
	if contextEntry.Cluster == "" {
		return nil, fmt.Errorf("context '%s' has no cluster", contextName)
	}
	cluster, ok := kubeconfig.Clusters[contextEntry.Cluster]
	if !ok || cluster == nil {
		return nil, fmt.Errorf("context '%s' references missing cluster '%s'", contextName, contextEntry.Cluster)
	}

	var authInfo *clientcmdapi.AuthInfo
	if contextEntry.AuthInfo != "" {
		authInfo, ok = kubeconfig.AuthInfos[contextEntry.AuthInfo]
		if !ok || authInfo == nil {
			return nil, fmt.Errorf("context '%s' references missing user '%s'", contextName, contextEntry.AuthInfo)
		}
	}

	singleCtx := clientcmdapi.NewConfig()
	singleCtx.CurrentContext = contextName
	singleCtx.Contexts[contextName] = contextEntry
	if entry, ok := cfg.Contexts[contextName]; ok && entry.Namespace != "" {
		ctxCopy := *singleCtx.Contexts[contextName]
		ctxCopy.Namespace = entry.Namespace
		singleCtx.Contexts[contextName] = &ctxCopy
	}
	singleCtx.Clusters[contextEntry.Cluster] = cluster
	if contextEntry.AuthInfo != "" {
		singleCtx.AuthInfos[contextEntry.AuthInfo] = authInfo
	}
	return singleCtx, nil
}

func writeTempKubeconfig(data []byte) (string, func() error, error) {
	tmpDir, err := os.MkdirTemp(tempDir(), "ekconf-*")
	if err != nil {
		return "", nil, fmt.Errorf("create temp dir: %w", err)
	}
	if err := os.Chmod(tmpDir, 0o700); err != nil {
		_ = os.RemoveAll(tmpDir)
		return "", nil, fmt.Errorf("chmod temp dir: %w", err)
	}

	tmpPath := filepath.Join(tmpDir, "config")
	tmpFile, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		return "", nil, fmt.Errorf("create temp kubeconfig: %w", err)
	}
	cleanup := func() error {
		var cleanupErr error
		if err := wipeFile(tmpPath); err != nil && !os.IsNotExist(err) {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("wipe temp kubeconfig: %w", err))
		}
		if err := os.Remove(tmpPath); err != nil && !os.IsNotExist(err) {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("remove temp kubeconfig: %w", err))
		}
		if err := os.Remove(tmpDir); err != nil && !os.IsNotExist(err) {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("remove temp dir: %w", err))
		}
		return cleanupErr
	}

	writtenBytes, err := tmpFile.Write(data)
	if err != nil {
		_ = tmpFile.Close()
		_ = cleanup()
		return "", nil, fmt.Errorf("write temp file: %w", err)
	}
	if writtenBytes != len(data) {
		_ = tmpFile.Close()
		_ = cleanup()
		return "", nil, fmt.Errorf("write temp file: short write")
	}
	if err := tmpFile.Chmod(0o600); err != nil {
		_ = tmpFile.Close()
		_ = cleanup()
		return "", nil, fmt.Errorf("chmod temp file: %w", err)
	}
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		_ = cleanup()
		return "", nil, fmt.Errorf("sync temp file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		_ = cleanup()
		return "", nil, fmt.Errorf("close temp file: %w", err)
	}

	return tmpPath, cleanup, nil
}

func buildExecCommand(ctx context.Context, commandArgs []string, kubeconfigPath string) *exec.Cmd {
	var c *exec.Cmd
	if execNoShell {
		//nolint:gosec // ekconf exec intentionally runs the command requested by the user.
		c = exec.CommandContext(ctx, commandArgs[0], commandArgs[1:]...)
	} else {
		fullCmd := strings.Join(commandArgs, " ")
		shell := os.Getenv("SHELL")
		if shell == "" {
			c = exec.CommandContext(ctx, "sh", "-c", fullCmd)
		} else {
			//nolint:gosec // Shell mode is intentional so aliases/functions work; use --no-shell to bypass it.
			c = exec.CommandContext(ctx, shell, "-ic", fullCmd)
		}
	}
	c.Env = append(os.Environ(), "KUBECONFIG="+kubeconfigPath)
	return c
}

func validateExecArgs(cmd *cobra.Command, args []string) error {
	dash := cmd.Flags().ArgsLenAtDash()
	if dash == -1 || dash == 0 {
		if len(args) == 0 {
			return fmt.Errorf("no command specified, use: ekconf exec [<name>] -- <cmd>")
		}
		return nil
	}
	if dash > 1 {
		return fmt.Errorf("exec accepts at most one context before --")
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if cfg.Current == "" && len(cfg.Contexts) == 0 {
		return nil
	}
	if !cfg.ContextExists(args[0]) {
		return fmt.Errorf("unknown context: %s", args[0])
	}
	if len(args) == 1 {
		return fmt.Errorf("no command specified, use: ekconf exec [<name>] -- <cmd>")
	}
	return nil
}

func init() {
	rootCmd.AddCommand(execCmd)
	execCmd.Flags().BoolVar(&execNoShell, "no-shell", false, "Run command directly without shell")
}
