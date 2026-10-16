package app

import (
	"io"
	"log/slog"
)

func NewLogger(output io.Writer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(output, nil))
}
