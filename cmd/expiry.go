package cmd

import (
	"io"
	"time"

	"github.com/eznix86/ekconf/internal/certinfo"
	"github.com/eznix86/ekconf/internal/config"
	"github.com/fatih/color"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

const expiryTimeFormat = "2006-01-02 15:04:05 MST"

var expiryWarningColor = color.New(color.FgYellow)

func recordContextExpiry(cfg *config.Config, kubeconfig *clientcmdapi.Config) bool {
	changed := false

	for name, entry := range cfg.Contexts {
		ctx, ok := kubeconfig.Contexts[name]
		if !ok || ctx == nil {
			continue
		}

		expiry, found := certinfo.Expiry(kubeconfig.AuthInfos[ctx.AuthInfo])
		switch {
		case !found:
			if entry.Expires == nil {
				continue
			}
			entry.Expires = nil
		case entry.Expires != nil && entry.Expires.Equal(expiry):
			continue
		default:
			entry.Expires = &expiry
		}

		cfg.Contexts[name] = entry
		changed = true
	}

	return changed
}

func refreshContextExpiry(w io.Writer, cfg *config.Config, kubeconfig *clientcmdapi.Config) {
	if !recordContextExpiry(cfg, kubeconfig) {
		return
	}
	if err := config.Save(cfg); err != nil {
		warnf(w, "Warning: failed to record certificate expiry: %v\n", err)
	}
}

func warnIfExpired(w io.Writer, contextName string, entry config.ContextEntry) {
	if entry.Expires == nil || time.Now().Before(*entry.Expires) {
		return
	}
	warnf(
		w,
		"Warning: client certificate for '%s' expired on %s\n",
		contextName,
		entry.Expires.Local().Format(expiryTimeFormat),
	)
}

func warnf(w io.Writer, format string, a ...any) {
	_, _ = expiryWarningColor.Fprintf(w, format, a...)
}
