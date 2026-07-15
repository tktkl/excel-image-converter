package settings

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/wutong/excel-image-converter/internal/converter"
)

const currentSettingsVersion = 2

type Settings struct {
	SettingsVersion      int                     `json:"settings_version"`
	KeepURL              bool                    `json:"keep_url"`
	CellImageMode        converter.CellImageMode `json:"cell_image_mode"`
	IgnoredUpdateVersion string                  `json:"ignored_update_version,omitempty"`
}

type Store struct {
	path string
}

func Default() Settings {
	return Settings{
		SettingsVersion: currentSettingsVersion,
		CellImageMode:   converter.DefaultCellImageMode(),
	}
}

func NewDefaultStore() (*Store, error) {
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		dir = "."
	}
	dir = filepath.Join(dir, "ExcelImageConverter")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	return &Store{path: filepath.Join(dir, "settings.json")}, nil
}

func (s *Store) Load() (Settings, error) {
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return Default(), nil
	}
	if err != nil {
		return Default(), err
	}
	var settings Settings
	if err := json.Unmarshal(data, &settings); err != nil {
		return Default(), nil
	}
	if settings.SettingsVersion < currentSettingsVersion {
		return Default(), nil
	}
	settings.CellImageMode = normalizeMode(settings.CellImageMode)
	return settings, nil
}

func (s *Store) Save(settings Settings) error {
	settings.SettingsVersion = currentSettingsVersion
	settings.CellImageMode = normalizeMode(settings.CellImageMode)
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), "settings-*.tmp")
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
	} else if !os.IsExist(err) {
		return err
	}
	if removeErr := os.Remove(s.path); removeErr != nil && !os.IsNotExist(removeErr) {
		return removeErr
	}
	return os.Rename(tmpPath, s.path)
}

func normalizeMode(mode converter.CellImageMode) converter.CellImageMode {
	normalized, err := converter.ParseCellImageMode(string(mode))
	if err != nil {
		return converter.DefaultCellImageMode()
	}
	return normalized
}
