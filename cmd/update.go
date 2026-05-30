package cmd

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/huh/spinner"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"golang.org/x/mod/semver"
)

const updateRepo = "eznix86/ekconf"

var (
	updateAPIBaseURL  = "https://api.github.com"
	updateHTTPClient  = &http.Client{Timeout: 30 * time.Second}
	currentExecutable = os.Executable
	updateCheckOnly   bool
)

type githubRelease struct {
	TagName string        `json:"tag_name"`
	Assets  []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type updatePlatform struct {
	goos   string
	goarch string
}

type releasePayload struct {
	assetName string
	data      []byte
}

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Self-update ekconf from the latest GitHub release",
	Long: `Download and install the latest ekconf release from GitHub.

Use --check to check for updates without installing.`,
	Example: `  ekconf update
  ekconf update --check`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if updateCheckOnly {
			notice, err := updateNoticeResult(cmd.Context())
			if err != nil {
				return err
			}
			if notice != "" {
				printNotice(cmd.ErrOrStderr(), notice)
			}
			return nil
		}

		var (
			release   *githubRelease
			assetName string
			data      []byte
			checksums []byte
		)

		steps := []struct {
			title  string
			action func() error
		}{
			{"Fetching release...", func() error {
				platform, err := normalizePlatform(runtime.GOOS, runtime.GOARCH)
				if err != nil {
					return err
				}

				release, err = fetchLatestRelease(cmd.Context())
				if err != nil {
					return err
				}

				assetName = updateAssetName(strings.TrimPrefix(release.TagName, "v"), platform)
				assetURL, err := releaseAssetURL(release, assetName)
				if err != nil {
					return err
				}
				data, err = downloadAsset(cmd.Context(), assetURL)
				if err != nil {
					return err
				}

				checksumURL, err := releaseAssetURL(release, "checksums.txt")
				if err != nil {
					return err
				}
				checksums, err = downloadAsset(cmd.Context(), checksumURL)
				return err
			}},
			{"Verifying checksum...", func() error {
				return verifyReleaseChecksum(checksums, releasePayload{assetName: assetName, data: data})
			}},
			{"Installing update...", func() error {
				binary, err := extractBinaryFromTarGz(data)
				if err != nil {
					return err
				}
				return replaceCurrentExecutable(binary)
			}},
		}

		for _, s := range steps {
			var actionErr error
			if err := spinner.New().Title(s.title).Action(func() {
				actionErr = s.action()
			}).Run(); err != nil {
				return err
			}
			if actionErr != nil {
				return actionErr
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✓ %s\n", s.title)
		}

		_, err := fmt.Fprintf(cmd.OutOrStdout(), "Updated ekconf to %s\n", release.TagName)
		return err
	},
}

func init() {
	rootCmd.AddCommand(updateCmd)
	updateCmd.Flags().BoolVar(&updateCheckOnly, "check", false, "Check for updates without installing")
}

func MaybePrintUpdateNotice(ctx context.Context, w io.Writer) bool {
	notice, ok := updateNotice(ctx)
	if !ok {
		return false
	}
	printNotice(w, notice)
	return true
}

func printNotice(w io.Writer, notice string) {
	c := color.New(color.FgYellow)
	c.EnableColor()
	_, _ = c.Fprintln(w, notice)
}

func updateNotice(ctx context.Context) (string, bool) {
	notice, err := updateNoticeResult(ctx)
	if err != nil || notice == "" {
		return "", false
	}
	return notice, true
}

func updateNoticeResult(ctx context.Context) (string, error) {
	if version == "dev" || version == "" {
		return "", nil
	}

	release, err := fetchLatestRelease(ctx)
	if err != nil {
		return "", err
	}

	if !isNewerVersion(strings.TrimPrefix(release.TagName, "v"), version) {
		return "", nil
	}

	return fmt.Sprintf("ekconf %s is available. Run 'ekconf update' to install it.", release.TagName), nil
}

func normalizePlatform(goos, goarch string) (updatePlatform, error) {
	switch goos {
	case "darwin", "linux":
	default:
		return updatePlatform{}, fmt.Errorf("unsupported operating system: %s", goos)
	}

	switch goarch {
	case "amd64", "arm64":
	default:
		return updatePlatform{}, fmt.Errorf("unsupported architecture: %s", goarch)
	}

	return updatePlatform{goos: goos, goarch: goarch}, nil
}

func updateAssetName(version string, platform updatePlatform) string {
	return fmt.Sprintf("ekconf_%s_%s_%s.tar.gz", version, platform.goos, platform.goarch)
}

func isNewerVersion(latest, current string) bool {
	latest = "v" + strings.TrimPrefix(latest, "v")
	current = "v" + strings.TrimPrefix(current, "v")
	if !semver.IsValid(latest) || !semver.IsValid(current) {
		return false
	}
	return semver.Compare(latest, current) > 0
}

func fetchLatestRelease(ctx context.Context) (*githubRelease, error) {
	url := strings.TrimRight(updateAPIBaseURL, "/") + "/repos/" + updateRepo + "/releases/latest"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build release request: %w", err)
	}

	resp, err := updateHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch latest release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch latest release: unexpected status %s", resp.Status)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("decode release response: %w", err)
	}

	if release.TagName == "" {
		return nil, fmt.Errorf("fetch latest release: missing tag_name")
	}

	return &release, nil
}

func releaseAssetURL(release *githubRelease, assetName string) (string, error) {
	for _, asset := range release.Assets {
		if asset.Name == assetName {
			if asset.BrowserDownloadURL == "" {
				return "", fmt.Errorf("release %s asset %s has no download url", release.TagName, assetName)
			}
			return asset.BrowserDownloadURL, nil
		}
	}

	return "", fmt.Errorf("release %s does not include asset %s", release.TagName, assetName)
}

func verifyReleaseChecksum(checksums []byte, payload releasePayload) error {
	want, err := checksumForAsset(checksums, payload.assetName)
	if err != nil {
		return err
	}

	have := sha256.Sum256(payload.data)
	haveHex := hex.EncodeToString(have[:])
	if haveHex != want {
		return fmt.Errorf("checksum mismatch for %s", payload.assetName)
	}

	return nil
}

func checksumForAsset(checksums []byte, assetName string) (string, error) {
	for line := range strings.SplitSeq(string(checksums), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		if filepath.Base(fields[len(fields)-1]) == assetName {
			return fields[0], nil
		}
	}

	return "", fmt.Errorf("checksum file does not include asset %s", assetName)
}

func downloadAsset(ctx context.Context, assetURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, assetURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build asset request: %w", err)
	}

	resp, err := updateHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download release asset: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download release asset: unexpected status %s", resp.Status)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read release asset: %w", err)
	}

	return data, nil
}

func extractBinaryFromTarGz(data []byte) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("open release archive: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read release archive: %w", err)
		}

		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		if filepath.Base(hdr.Name) != "ekconf" {
			continue
		}

		binary, err := io.ReadAll(tr)
		if err != nil {
			return nil, fmt.Errorf("extract ekconf: %w", err)
		}
		return binary, nil
	}

	return nil, fmt.Errorf("ekconf binary not found in release archive")
}

func replaceCurrentExecutable(data []byte) (retErr error) {
	exePath, err := currentExecutable()
	if err != nil {
		return fmt.Errorf("resolve current executable: %w", err)
	}

	tmpFile, err := os.CreateTemp(filepath.Dir(exePath), ".ekconf-update-*")
	if err != nil {
		return fmt.Errorf("current executable is not writable: %w", err)
	}
	tmpName := tmpFile.Name()
	tmpClosed := false
	defer func() {
		if !tmpClosed {
			if err := tmpFile.Close(); err != nil && retErr == nil {
				retErr = fmt.Errorf("close update payload: %w", err)
			}
		}
		if err := os.Remove(tmpName); err != nil && !os.IsNotExist(err) && retErr == nil {
			retErr = fmt.Errorf("remove update payload: %w", err)
		}
	}()

	if _, err := tmpFile.Write(data); err != nil {
		return fmt.Errorf("write update payload: %w", err)
	}
	if err := tmpFile.Chmod(0o755); err != nil {
		return fmt.Errorf("set update permissions: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close update payload: %w", err)
	}
	tmpClosed = true

	if err := os.Rename(tmpName, exePath); err != nil {
		return fmt.Errorf("replace current executable: %w", err)
	}

	return nil
}
