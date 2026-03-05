package trace

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/vigo999/ms-cli/agent/session"
)

// Writer writes structured runtime events.
type Writer interface {
	Write(eventType string, payload any) error
}

// Record is one trace event persisted to disk.
type Record = session.EventRecord

// FileWriter writes JSONL trace records to disk.
type FileWriter = session.EventWriter

// NewFileWriter creates a writer that appends to a JSONL file.
func NewFileWriter(path string) (*FileWriter, error) {
	return session.NewEventWriter(path)
}

// NewTimestampWriter creates a writer under cacheDir with timestamp filename.
func NewTimestampWriter(cacheDir string) (*FileWriter, error) {
	if cacheDir == "" {
		return nil, fmt.Errorf("cache directory cannot be empty")
	}

	filename := fmt.Sprintf("%s.trajectory.jsonl", time.Now().Format("20060102-150405.000000000"))
	return NewFileWriter(filepath.Join(cacheDir, filename))
}
