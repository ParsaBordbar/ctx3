package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/parsabordbar/ctx3/internal/version"
	"github.com/spf13/cobra"
)

var updateCheckOnly bool

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update ctx3 to the latest released version",
	Long: `Update ctx3 in place by installing the latest tagged release with the Go
toolchain (go install ` + version.Module + `@latest).

--check reports the latest version without installing. Requires Go on PATH.`,
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		current := version.Version()
		fmt.Printf("Current: %s\n", current)

		latest, err := latestVersion(cmd.Context())
		if err != nil {
			return fmt.Errorf("checking latest version: %w", err)
		}
		fmt.Printf("Latest:  %s\n", latest)

		if latest == current {
			fmt.Println(glyph("✓", "+") + " Already up to date.")
			return nil
		}
		if updateCheckOnly {
			fmt.Printf("Run `ctx3 update` to install %s.\n", latest)
			return nil
		}

		goBin, err := exec.LookPath("go")
		if err != nil {
			return fmt.Errorf("go toolchain not found on PATH; install manually: go install %s@latest", version.Module)
		}

		target := version.Module + "@latest"
		fmt.Printf("Installing %s ...\n", target)
		install := exec.CommandContext(cmd.Context(), goBin, "install", target)
		install.Stdout = os.Stdout
		install.Stderr = os.Stderr
		if err := install.Run(); err != nil {
			return fmt.Errorf("go install failed: %w", err)
		}
		fmt.Printf("%s Updated to %s.\n", glyph("✓", "+"), latest)
		return nil
	},
}

// latestVersion asks the Go module proxy what `@latest` resolves to.
func latestVersion(ctx context.Context) (string, error) {
	url := "https://proxy.golang.org/" + version.Module + "/@latest"
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("proxy returned %s", resp.Status)
	}

	var body struct {
		Version string `json:"Version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", err
	}
	if body.Version == "" {
		return "", fmt.Errorf("no version in proxy response")
	}
	return body.Version, nil
}

func init() {
	updateCmd.Flags().BoolVar(&updateCheckOnly, "check", false, "Only report the latest version; do not install")
	rootCmd.AddCommand(updateCmd)
}
