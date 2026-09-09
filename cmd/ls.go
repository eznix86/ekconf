package cmd

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/eznix86/ekconf/internal/config"
	"github.com/spf13/cobra"
)

const (
	columnGap        = "  "
	headerName       = "NAME"
	headerNamespace  = "NAMESPACE"
	headerExpires    = "EXPIRES"
	expiryDateFormat = "2006-01-02"
)

type contextRow struct {
	mark      string
	name      string
	namespace string
	expiry    string
	expired   bool
}

var lsCmd = &cobra.Command{
	Use:     "ls",
	Aliases: []string{"list"},
	Short:   "List all contexts",
	Long: `List all contexts stored in the encrypted kubeconfig. The active context is marked with *.

The EXPIRES column shows when the context's client certificate expires, in
local time, with the remaining lifetime in parentheses. Expired dates are
printed in yellow. The NAMESPACE and EXPIRES columns are dropped when no
context has a value for them.`,
	Example: `  ekconf ls`,
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}

		if len(cfg.Contexts) == 0 {
			_, err = fmt.Fprintln(cmd.OutOrStdout(), "No contexts found")
			return err
		}

		for _, line := range renderContextTable(buildContextRows(cfg, time.Now())) {
			if _, err := fmt.Fprintln(cmd.OutOrStdout(), line); err != nil {
				return err
			}
		}

		return nil
	},
}

func buildContextRows(cfg *config.Config, now time.Time) []contextRow {
	rows := make([]contextRow, 0, len(cfg.Contexts))

	for _, name := range sortedContextNames(cfg) {
		entry := cfg.Contexts[name]

		row := contextRow{mark: " ", name: name}
		if name == cfg.Current {
			row.mark = "*"
		}
		if entry.Namespace != config.DefaultNamespace {
			row.namespace = entry.Namespace
		}
		row.expiry, row.expired = expiryCell(entry, now)

		rows = append(rows, row)
	}

	return rows
}

func expiryCell(entry config.ContextEntry, now time.Time) (string, bool) {
	if entry.Expires == nil {
		return "", false
	}

	date := entry.Expires.Local().Format(expiryDateFormat)
	if !now.Before(*entry.Expires) {
		return date + " (expired)", true
	}
	return fmt.Sprintf("%s (in %s)", date, remainingLifetime(entry.Expires.Sub(now))), false
}

func remainingLifetime(d time.Duration) string {
	days := int(d.Hours() / 24)
	switch {
	case d < time.Minute:
		return "<1m"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case days < 1:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case days < 30:
		return fmt.Sprintf("%dd", days)
	case days < 365:
		return fmt.Sprintf("%dmo", days/30)
	default:
		return fmt.Sprintf("%dy", days/365)
	}
}

func renderContextTable(rows []contextRow) []string {
	showNamespace := false
	showExpiry := false
	nameWidth := len(headerName)
	namespaceWidth := len(headerNamespace)

	for _, row := range rows {
		nameWidth = max(nameWidth, len(row.name))
		namespaceWidth = max(namespaceWidth, len(row.namespace))
		showNamespace = showNamespace || row.namespace != ""
		showExpiry = showExpiry || row.expiry != ""
	}

	header := "  " + pad(headerName, nameWidth)
	if showNamespace {
		header += columnGap + pad(headerNamespace, namespaceWidth)
	}
	if showExpiry {
		header += columnGap + headerExpires
	}

	lines := []string{strings.TrimRight(header, " ")}
	for _, row := range rows {
		line := row.mark + " " + pad(row.name, nameWidth)
		if showNamespace {
			line += columnGap + pad(row.namespace, namespaceWidth)
		}
		if showExpiry {
			line += columnGap + colorizeExpiry(row)
		}
		lines = append(lines, strings.TrimRight(line, " "))
	}

	return lines
}

func colorizeExpiry(row contextRow) string {
	if row.expired {
		return expiryWarningColor.Sprint(row.expiry)
	}
	return row.expiry
}

func pad(s string, width int) string {
	return fmt.Sprintf("%-*s", width, s)
}

func sortedContextNames(cfg *config.Config) []string {
	names := make([]string, 0, len(cfg.Contexts))
	for name := range cfg.Contexts {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func init() {
	rootCmd.AddCommand(lsCmd)
}
