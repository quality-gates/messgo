// Command messgo is a PHPMD-style mess detector for Go source code.
package main

import (
	"os"

	"github.com/quality-gates/messgo/internal/cli"
)

func main() {
	os.Exit(cli.Main(os.Args[1:], os.Stdout, os.Stderr))
}
