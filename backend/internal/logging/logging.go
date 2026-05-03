package logging

import (
	"io"
	"log/slog"
	"os"
)

func Configure(writers ...io.Writer) *slog.Logger {
	var output io.Writer = os.Stdout
	if len(writers) > 0 && writers[0] != nil {
		output = writers[0]
	}
	handler := slog.NewJSONHandler(output, &slog.HandlerOptions{
		ReplaceAttr: func(_ []string, attribute slog.Attr) slog.Attr {
			if attribute.Key == slog.TimeKey {
				attribute.Key = "ts"
			}
			return attribute
		},
	})
	logger := slog.New(handler)
	slog.SetDefault(logger)
	return logger
}
