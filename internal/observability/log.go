package observability

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

func New(level string, output io.Writer) *slog.Logger {
	if output == nil {
		output = os.Stdout
	}
	selected := slog.LevelInfo
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		selected = slog.LevelDebug
	case "warn":
		selected = slog.LevelWarn
	case "error":
		selected = slog.LevelError
	}
	return slog.New(slog.NewJSONHandler(output, &slog.HandlerOptions{Level: selected}))
}
