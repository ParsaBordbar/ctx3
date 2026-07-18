package cmd

import (
	"fmt"
	"runtime"

	"github.com/parsabordbar/ctx3/internal/version"
	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the ctx3 version",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("ctx3 %s (%s %s/%s)\n",
			version.Version(), runtime.Version(), runtime.GOOS, runtime.GOARCH)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
