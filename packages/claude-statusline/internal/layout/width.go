// Package layout composes widgets into rows, applies the reactive drop
// priority on overflow, and renders flex spacers / right-aligned content.
package layout

import (
	"os"
	"strconv"
	"syscall"
	"unsafe"
)

const defaultWidth = 80

// DetectWidth returns the effective terminal width. Precedence:
//  1. CLAUDE_STATUSLINE_WIDTH env var (must be positive int).
//  2. TIOCGWINSZ ioctl on stdout (Unix-only).
//  3. defaultWidth (80).
func DetectWidth() int {
	if s := os.Getenv("CLAUDE_STATUSLINE_WIDTH"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			return n
		}
	}
	if w := ttyWidth(); w > 0 {
		return w
	}
	return defaultWidth
}

type winsize struct {
	Row, Col       uint16
	XPixel, YPixel uint16
}

func ttyWidth() int {
	for _, fd := range []uintptr{os.Stdout.Fd(), os.Stderr.Fd(), os.Stdin.Fd()} {
		ws := &winsize{}
		_, _, errno := syscall.Syscall(syscall.SYS_IOCTL,
			fd, uintptr(syscall.TIOCGWINSZ), uintptr(unsafe.Pointer(ws)))
		if errno == 0 && ws.Col > 0 {
			return int(ws.Col)
		}
	}
	return 0
}
