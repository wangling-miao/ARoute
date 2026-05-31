package trust

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type Ledger interface {
	Append(decision Decision) error
	List(plugin string) ([]Decision, error)
}

type MemoryLedger struct {
	mu        sync.Mutex
	decisions []Decision
}

func NewMemoryLedger() *MemoryLedger {
	return &MemoryLedger{}
}

func (l *MemoryLedger) Append(decision Decision) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.decisions = append(l.decisions, decision)
	return nil
}

func (l *MemoryLedger) List(plugin string) ([]Decision, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]Decision, 0, len(l.decisions))
	for _, decision := range l.decisions {
		if plugin == "" || decision.Plugin == plugin {
			out = append(out, decision)
		}
	}
	return out, nil
}

type FileLedger struct {
	mu   sync.Mutex
	path string
}

func NewFileLedger(path string) (*FileLedger, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("trust ledger: create directory: %w", err)
	}
	return &FileLedger{path: path}, nil
}

func (l *FileLedger) Append(decision Decision) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("trust ledger: open: %w", err)
	}
	defer f.Close()

	data, err := json.Marshal(decision)
	if err != nil {
		return fmt.Errorf("trust ledger: marshal: %w", err)
	}
	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("trust ledger: append: %w", err)
	}
	return nil
}

func (l *FileLedger) List(plugin string) ([]Decision, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	f, err := os.Open(l.path)
	if os.IsNotExist(err) {
		return []Decision{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("trust ledger: open: %w", err)
	}
	defer f.Close()

	var decisions []Decision
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var decision Decision
		if err := json.Unmarshal(scanner.Bytes(), &decision); err != nil {
			continue
		}
		if plugin == "" || decision.Plugin == plugin {
			decisions = append(decisions, decision)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("trust ledger: scan: %w", err)
	}
	if decisions == nil {
		decisions = []Decision{}
	}
	return decisions, nil
}
