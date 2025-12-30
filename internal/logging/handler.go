// Package logging provides custom logging handlers.
package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
)

// HumanReadableHandler is a custom slog handler that formats logs in a human-readable way.
type HumanReadableHandler struct {
	writer io.Writer
	opts   slog.HandlerOptions
}

// NewHumanReadableHandler creates a new human-readable log handler.
func NewHumanReadableHandler(w io.Writer, opts *slog.HandlerOptions) *HumanReadableHandler {
	if opts == nil {
		opts = &slog.HandlerOptions{}
	}
	return &HumanReadableHandler{
		writer: w,
		opts:   *opts,
	}
}

// Enabled reports whether the handler handles records at the given level.
func (h *HumanReadableHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= h.opts.Level.Level()
}

// Handle formats and writes the log record.
func (h *HumanReadableHandler) Handle(ctx context.Context, r slog.Record) error {
	// Convert Record fields to attributes and apply ReplaceAttr
	// This allows ReplaceAttr to filter/modify time, level, and msg
	attrs := []slog.Attr{
		slog.Time("time", r.Time),
		slog.Any("level", r.Level),
		slog.String("msg", r.Message),
	}

	// Add all other attributes from the record
	r.Attrs(func(a slog.Attr) bool {
		attrs = append(attrs, a)
		return true
	})

	// Apply ReplaceAttr to all attributes (if provided)
	var filteredAttrs []slog.Attr
	for _, a := range attrs {
		replaced := a
		if h.opts.ReplaceAttr != nil {
			replaced = h.opts.ReplaceAttr(nil, a)
		}
		// Only include non-empty attributes (empty key means removed)
		if replaced.Key != "" {
			filteredAttrs = append(filteredAttrs, replaced)
		}
	}

	// Build the log line from remaining attributes
	var buf strings.Builder

	// Extract msg separately if it exists (for cleaner output)
	var msgAttr *slog.Attr
	var otherAttrs []slog.Attr
	for i := range filteredAttrs {
		if filteredAttrs[i].Key == "msg" {
			msgAttr = &filteredAttrs[i]
		} else {
			otherAttrs = append(otherAttrs, filteredAttrs[i])
		}
	}

	// Write message first if present
	if msgAttr != nil {
		msgVal := msgAttr.Value.Any()
		if msgStr, ok := msgVal.(string); ok {
			buf.WriteString(msgStr)
		} else {
			buf.WriteString(fmt.Sprintf("%v", msgVal))
		}
	}

	// Write other attributes
	if len(otherAttrs) > 0 {
		if msgAttr != nil {
			buf.WriteString(" (")
		}
		first := true
		for _, a := range otherAttrs {
			if !first {
				buf.WriteString(", ")
			}
			first = false
			buf.WriteString(a.Key)
			buf.WriteString("=")
			// Format the value, quoting strings that contain spaces
			val := a.Value.Any()
			if str, ok := val.(string); ok && (strings.Contains(str, " ") || strings.Contains(str, "=")) {
				buf.WriteString(`"`)
				buf.WriteString(str)
				buf.WriteString(`"`)
			} else {
				buf.WriteString(fmt.Sprintf("%v", val))
			}
		}
		if msgAttr != nil {
			buf.WriteString(")")
		}
	}
	buf.WriteString("\n")

	_, err := h.writer.Write([]byte(buf.String()))
	return err
}

// WithAttrs returns a new handler with the given attributes.
func (h *HumanReadableHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	// For simplicity, we'll just return the same handler
	// In a more complex implementation, we could store these attributes
	return h
}

// WithGroup returns a new handler with the given group name.
func (h *HumanReadableHandler) WithGroup(name string) slog.Handler {
	// For simplicity, we'll just return the same handler
	return h
}
