package log

import (
	"log/slog"
	"os"
)

type Handler struct {
	*slog.JSONHandler
	GcpProjectId string
}

func NewHandler(projectId string) *Handler {
	opt := slog.HandlerOptions{
		AddSource:   true,
		ReplaceAttr: replaceAttr,
	}
	return &Handler{
		JSONHandler:  slog.NewJSONHandler(os.Stderr, &opt),
		GcpProjectId: projectId,
	}
}

func replaceAttr(_ []string, a slog.Attr) slog.Attr {
	switch a.Key {
	case slog.LevelKey:
		return slog.String("severity", toServity(a.Value.Any().(slog.Level)))
	case slog.MessageKey:
		return slog.Attr{Key: "message", Value: a.Value}
	}
	switch a.Value.Kind() {
	case slog.KindDuration:
		return slog.String(a.Key, a.Value.Duration().String())
	}
	return a
}

func toServity(level slog.Level) string {
	switch level {
	case slog.LevelDebug:
		return "DEBUG"
	case slog.LevelInfo:
		return "INFO"
	case slog.LevelWarn:
		return "WARNING"
	case slog.LevelError:
		return "ERROR"
	default:
		return "DEFAULT"
	}
}
