package main

import (
	"io"
	"os"
)

func main() {
	_, _ = io.Copy(io.Discard, os.Stdin)
	_, _ = os.Stdout.WriteString("claude-statusline: not yet implemented\n")
}
