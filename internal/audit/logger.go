package audit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Logger struct {
	path string
	mu   sync.Mutex
}

type Event struct {
	Time    time.Time              `json:"time"`
	Type    string                 `json:"type"`
	Message string                 `json:"message"`
	Fields  map[string]interface{} `json:"fields,omitempty"`
}

func New(path string) *Logger {
	return &Logger{path: path}
}

func (l *Logger) Log(eventType, message string, fields map[string]interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()

	_ = os.MkdirAll(filepath.Dir(l.path), 0755)
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()

	e := Event{Time: time.Now(), Type: eventType, Message: message, Fields: fields}
	b, _ := json.Marshal(e)
	_, _ = f.Write(append(b, '\n'))
}
