// Package logger sets up, from a single place, the structured logger used
// throughout the application (JSON format, to stdout). Other packages can
// also use log/slog directly (since the global logger is set), but the
// setup logic lives here so that a future migration to zap would stay
// confined to this one file.
package logger

import (
	"log/slog"
	"os"
)

// Init sets up a slog.Logger in JSON format and configures it as the
// global default logger. It is called at the very start of the
// application, inside main().
func Init(level slog.Level) {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	slog.SetDefault(slog.New(handler))
}
