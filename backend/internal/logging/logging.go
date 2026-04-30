package logging

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

func Configure(level string, w ...io.Writer) *slog.Logger {
	var out io.Writer = os.Stdout
	if len(w) > 0 && w[0] != nil {
		out = w[0]
	}
	h := slog.NewJSONHandler(out, &slog.HandlerOptions{
		Level: ParseLevel(level),
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				a.Key = "ts"
			}
			return a
		},
	})
	logger := slog.New(h)
	slog.SetDefault(logger)
	return logger
}

func ParseLevel(s string) slog.Level {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "DEBUG":
		return slog.LevelDebug
	case "WARN", "WARNING":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
