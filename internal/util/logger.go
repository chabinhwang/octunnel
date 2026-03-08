package util

import (
	"fmt"
	"os"
)

// Tag identifies the source of a log message.
type Tag string

const (
	TagPreflight   Tag = "preflight"
	TagOpencode    Tag = "opencode"
	TagCloudflared Tag = "cloudflared"
	TagOctunnel    Tag = "octunnel"
	TagRecover     Tag = "recover"
	TagError       Tag = "error"
)

var tagColors = map[Tag]string{
	TagPreflight:   "\033[36m",
	TagOpencode:    "\033[34m",
	TagCloudflared: "\033[35m",
	TagOctunnel:    "\033[32m",
	TagRecover:     "\033[33m",
	TagError:       "\033[31m",
}

const colorReset = "\033[0m"

func Log(tag Tag, format string, args ...any) {
	color := tagColors[tag]
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "%s[%s]%s %s\n", color, tag, colorReset, msg)
}

func LogSuccess(tag Tag, format string, args ...any) {
	color := tagColors[tag]
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "%s[%s]%s \u2713 %s\n", color, tag, colorReset, msg)
}

func LogWarn(tag Tag, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "\033[33m[warn]\033[0m \u26a0 %s\n", msg)
}

func LogError(tag Tag, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "\033[31m[error]\033[0m %s\n", msg)
}
