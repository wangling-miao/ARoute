// Package main implements the version subcommand for ARoute CMS.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/wangling-miao/aroute/internal/version"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version information",
	Long:  `Display the ARoute CMS version, git commit hash, build date, and Go version.`,
	Run:   runVersion,
}

var versionJSON bool

func init() {
	rootCmd.AddCommand(versionCmd)
	versionCmd.Flags().BoolVar(&versionJSON, "json", false, "output in JSON format")
}

func runVersion(cmd *cobra.Command, args []string) {
	if versionJSON {
		output := struct {
			Version   string `json:"version"`
			Commit    string `json:"commit"`
			BuildDate string `json:"buildDate"`
			GoVersion string `json:"goVersion"`
		}{
			Version:   version.Version,
			Commit:    version.Commit,
			BuildDate: version.BuildDate,
			GoVersion: version.GoVersion,
		}
		data, err := json.Marshal(output)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error marshaling version info: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(data))
	} else {
		fmt.Printf("ARoute CMS v%s\n", version.Version)
		fmt.Printf("  Commit:     %s\n", version.Commit)
		fmt.Printf("  Build Date: %s\n", version.BuildDate)
		fmt.Printf("  Go Version: %s\n", version.GoVersion)
	}
}
