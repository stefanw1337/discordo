package logger

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/ayn2op/discordo/internal/consts"
)

const fileName = "logs.txt"

func DefaultPath() string {
	return filepath.Join(consts.CacheDir(), fileName)
}

// Load opens the log file and configures default logger.
func Load(path string, level slog.Level) (io.Closer, error) {
	if err := os.MkdirAll(filepath.Dir(path), os.ModePerm); err != nil {
		return nil, err
	}

	// A discordo that opened itself in Windows Terminal adds to the log of the one that opened it.
	flags := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	if os.Getenv("DISCORDO_RELAUNCHED") != "" {
		flags = os.O_CREATE | os.O_WRONLY | os.O_APPEND
	}
	file, err := os.OpenFile(path, flags, os.ModePerm)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}

	opts := &slog.HandlerOptions{Level: level}
	handler := slog.NewTextHandler(file, opts)
	slog.SetDefault(slog.New(handler))
	return file, nil
}
