package settings

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wutong/excel-image-converter/internal/converter"
)

func TestDefaultSettingsPreferWPSWithoutLinks(t *testing.T) {
	defaults := Default()
	if defaults.KeepURL {
		t.Fatal("Default().KeepURL = true, want false")
	}
	if defaults.CellImageMode != converter.CellImageModeWPS {
		t.Fatalf("Default().CellImageMode = %q, want %q", defaults.CellImageMode, converter.CellImageModeWPS)
	}
	if defaults.SettingsVersion != currentSettingsVersion {
		t.Fatalf("Default().SettingsVersion = %d, want %d", defaults.SettingsVersion, currentSettingsVersion)
	}
}

func TestLoadMigratesLegacySettingsToNewDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"keep_url":true,"cell_image_mode":"excel"}`), 0644); err != nil {
		t.Fatal(err)
	}

	loaded, err := (&Store{path: path}).Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.KeepURL {
		t.Fatal("Load() kept legacy keep_url=true, want migrated default false")
	}
	if loaded.CellImageMode != converter.CellImageModeWPS {
		t.Fatalf("Load().CellImageMode = %q, want %q", loaded.CellImageMode, converter.CellImageModeWPS)
	}
}

func TestSaveAndLoadPreservesCurrentUserChoice(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	store := &Store{path: path}

	if err := store.Save(Settings{KeepURL: true, CellImageMode: converter.CellImageModeExcel, IgnoredUpdateVersion: "1.0.14"}); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.KeepURL {
		t.Fatal("Load().KeepURL = false, want true")
	}
	if loaded.CellImageMode != converter.CellImageModeExcel {
		t.Fatalf("Load().CellImageMode = %q, want %q", loaded.CellImageMode, converter.CellImageModeExcel)
	}
	if loaded.SettingsVersion != currentSettingsVersion {
		t.Fatalf("Load().SettingsVersion = %d, want %d", loaded.SettingsVersion, currentSettingsVersion)
	}
	if loaded.IgnoredUpdateVersion != "1.0.14" {
		t.Fatalf("Load().IgnoredUpdateVersion = %q, want 1.0.14", loaded.IgnoredUpdateVersion)
	}
}
