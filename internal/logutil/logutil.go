// Package logutil builds the logger and carries it through a context.
package logutil

import (
	"context"
	"fmt"
	"io"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// loggerKey is the context key under which WrapLogger stores the logger.
type loggerKey struct{}

// New returns a logger that writes console lines at level and above to w, which the command
// sets to stderr so stdout carries only the result. The level is a zap level name such as
// debug, info, warn, or error. Writes to w are serialized. New panics when w is nil.
func New(w io.Writer, level string) (*zap.Logger, error) {
	if w == nil {
		panic("logutil.New: writer required")
	}
	lvl, err := zapcore.ParseLevel(level)
	if err != nil {
		return nil, fmt.Errorf("%w: %q", ErrLevel, level)
	}
	enc := zap.NewProductionEncoderConfig()
	enc.EncodeTime = zapcore.ISO8601TimeEncoder
	out := zapcore.Lock(zapcore.AddSync(w))
	core := zapcore.NewCore(zapcore.NewConsoleEncoder(enc), out, lvl)
	return zap.New(core, zap.ErrorOutput(out)), nil
}

// WrapLogger returns a copy of ctx that carries log.
func WrapLogger(ctx context.Context, log *zap.Logger) context.Context {
	return context.WithValue(ctx, loggerKey{}, log)
}

// UnwrapLogger returns the logger carried by ctx, or a no-op logger when ctx carries none.
func UnwrapLogger(ctx context.Context) *zap.Logger {
	if log, ok := ctx.Value(loggerKey{}).(*zap.Logger); ok && log != nil {
		return log
	}
	return zap.NewNop()
}
