package observability

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// DailyRollingWriter writes to a file that rotates at midnight.
type DailyRollingWriter struct {
	mu       sync.Mutex
	dir      string
	prefix   string
	currDate string
	file     *os.File
}

// NewDailyRollingWriter creates a new writer that will store logs in the given directory.
func NewDailyRollingWriter(dir, prefix string) *DailyRollingWriter {
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create log directory %s: %v\n", dir, err)
	}
	return &DailyRollingWriter{
		dir:    dir,
		prefix: prefix,
	}
}

func (w *DailyRollingWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	now := time.Now().Format("2006-01-02")
	if w.currDate != now || w.file == nil {
		if w.file != nil {
			w.file.Close()
		}
		filename := filepath.Join(w.dir, fmt.Sprintf("%s-%s.log", w.prefix, now))
		file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			// If we fail to open the file, we can't write to it, but we shouldn't crash the app
			return 0, err
		}
		w.file = file
		w.currDate = now
	}

	return w.file.Write(p)
}

// Close closes the underlying file if it is open.
func (w *DailyRollingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file != nil {
		err := w.file.Close()
		w.file = nil
		return err
	}
	return nil
}
