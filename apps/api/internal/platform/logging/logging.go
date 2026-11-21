package logging

import (
	"io"
	"log/slog"
)

func New(out io.Writer, level string) *slog.Logger {
	l := slog.LevelInfo
	switch level {
	case "debug":
		l = slog.LevelDebug
	case "warn":
		l = slog.LevelWarn
	case "error":
		l = slog.LevelError
	}
	return slog.New(slog.NewJSONHandler(out, &slog.HandlerOptions{Level: l}))
}
