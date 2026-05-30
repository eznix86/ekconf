package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/eznix86/ekconf/internal/config"
	"github.com/eznix86/ekconf/internal/crypto"
	"github.com/spf13/cobra"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

var execNoShell bool

func wipeFile(path string) {
	data, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return
	}
	defer data.Close()

	info, err := data.Stat()
	if err != nil {
		return
	}

	size := info.Size()
	if size > 0 {
		buf := make([]byte, 4096)
		for written := int64(0); written < size; written += int64(len(buf)) {
			rem := size - written
			if rem < int64(len(buf)) {
				buf = make([]byte, rem)
			}
			data.Write(buf)
		}
		data.Sync()
	}
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
	ValidArgsFunction: completeContext,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}

		var contextName string
		var commandArgs []string

		if len(args) >= 1 && cfg.ContextExists(args[0]) {
			contextName = args[0]
			commandArgs = args[1:]
		} else {
			commandArgs = args
		}

		if len(commandArgs) == 0 {
			return fmt.Errorf("no command specified, use: ekconf exec [<name>] -- <cmd>")
		}

		if contextName == "" {
			if cfg.Current == "" {
				return fmt.Errorf("no active context set and no context name provided")
			}
			contextName = cfg.Current
		}

		password, err := resolvePassword()
		if err != nil {
			return err
		}

		encPath, err := config.EncPath()
		if err != nil {
			return err
		}

		data, err := os.ReadFile(encPath)
		if err != nil {
			return fmt.Errorf("read config.enc: %w", err)
		}

		ef, err := crypto.Unmarshal(data)
		if err != nil {
			return fmt.Errorf("parse encrypted file: %w", err)
		}

		plaintext, err := crypto.Decrypt(ef, password)
		if err != nil {
			return fmt.Errorf("decrypt (wrong password?): %w", err)
		}

		kubeconfig, err := clientcmd.Load(plaintext)
		if err != nil {
			return fmt.Errorf("parse kubeconfig: %w", err)
		}

		if _, ok := kubeconfig.Contexts[contextName]; !ok {
			return fmt.Errorf("context '%s' not found", contextName)
		}

		storePasswordIfNeeded(password)

		singleCtx := clientcmdapi.NewConfig()
		singleCtx.CurrentContext = contextName
		singleCtx.Contexts[contextName] = kubeconfig.Contexts[contextName]
		if entry, ok := cfg.Contexts[contextName]; ok && entry.Namespace != "" {
			ctxCopy := *singleCtx.Contexts[contextName]
			ctxCopy.Namespace = entry.Namespace
			singleCtx.Contexts[contextName] = &ctxCopy
		}
		singleCtx.Clusters[kubeconfig.Contexts[contextName].Cluster] = kubeconfig.Clusters[kubeconfig.Contexts[contextName].Cluster]
		singleCtx.AuthInfos[kubeconfig.Contexts[contextName].AuthInfo] = kubeconfig.AuthInfos[kubeconfig.Contexts[contextName].AuthInfo]

		out, err := clientcmd.Write(*singleCtx)
		if err != nil {
			return fmt.Errorf("marshal kubeconfig: %w", err)
		}

		tmpFile, err := os.CreateTemp(tempDir(), "ekconf-*.kubeconfig")
		if err != nil {
			return fmt.Errorf("create temp file: %w", err)
		}
		tmpPath := tmpFile.Name()

		defer func() {
			tmpFile.Close()
			wipeFile(tmpPath)
			os.Remove(tmpPath)
		}()

		if _, err := tmpFile.Write(out); err != nil {
			return fmt.Errorf("write temp file: %w", err)
		}
		if err := tmpFile.Chmod(0600); err != nil {
			return fmt.Errorf("chmod temp file: %w", err)
		}
		tmpFile.Close()

		var c *exec.Cmd
		if execNoShell {
			c = exec.Command(commandArgs[0], commandArgs[1:]...)
		} else {
			fullCmd := strings.Join(commandArgs, " ")
			shell := os.Getenv("SHELL")
			if shell == "" {
				c = exec.Command("sh", "-c", fullCmd)
			} else {
				c = exec.Command(shell, "-ic", fullCmd)
			}
		}
		c.Env = append(os.Environ(), "KUBECONFIG="+tmpPath)
		c.Stdin = os.Stdin
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr

		if err := c.Run(); err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				return fmt.Errorf("command exited with status %d", exitErr.ExitCode())
			}
			return fmt.Errorf("execute command: %w", err)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(execCmd)
	execCmd.Flags().BoolVar(&execNoShell, "no-shell", false, "Run command directly without shell")
}
