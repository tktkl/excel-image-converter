package history

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type Entry struct {
	ID        string    `json:"id"`
	Source    string    `json:"source"`
	Output    string    `json:"output"`
	Status    string    `json:"status"`
	Converted int       `json:"converted"`
	Failed    int       `json:"failed"`
	Error     string    `json:"error,omitempty"`
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at"`
}

type Store struct {
	path string
}

var ErrHistoryCorrupt = errors.New("history file is corrupt")

func NewDefaultStore() (*Store, error) {
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		dir = "."
	}
	dir = filepath.Join(dir, "ExcelImageConverter")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	return &Store{path: filepath.Join(dir, "history.json")}, nil
}

func (s *Store) Load() ([]Entry, error) {
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var entries []Entry
	if err := json.Unmarshal(data, &entries); err != nil {
		if backupErr := s.backupCorruptFile(); backupErr != nil {
			return nil, errors.Join(ErrHistoryCorrupt, err, backupErr)
		}
		return nil, nil
	}
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].EndedAt.After(entries[j].EndedAt)
	})
	return entries, nil
}

func (s *Store) Save(entries []Entry) error {
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].EndedAt.After(entries[j].EndedAt)
	})
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), "history-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, s.path); err == nil {
		return nil
	} else {
		if !os.IsExist(err) {
			return err
		}
		if removeErr := os.Remove(s.path); removeErr != nil && !os.IsNotExist(removeErr) {
			return errors.Join(err, removeErr)
		}
		if renameErr := os.Rename(tmpPath, s.path); renameErr != nil {
			return errors.Join(err, renameErr)
		}
	}
	return nil
}

func (s *Store) Clear() error {
	if err := os.Remove(s.path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s *Store) backupCorruptFile() error {
	backupPath := s.path + ".corrupt-" + time.Now().Format("20060102150405")
	if err := os.Rename(s.path, backupPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
