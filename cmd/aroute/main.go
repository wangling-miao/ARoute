package main

import (
	"fmt"
	"os"

	"github.com/wangling-miao/aroute/internal/version"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Printf("aroute %s (commit: %s, built: %s, go: %s)\n",
			version.Version, version.Commit, version.BuildDate, version.GoVersion)
		return
	}
	fmt.Println("aroute - A microkernel CMS for Go")
	fmt.Println("Run 'aroute serve' to start the server.")
}
