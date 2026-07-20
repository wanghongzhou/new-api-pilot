package service

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"new-api-pilot/dto"
)

func TestExportArtifactPathRejectsTraversalAndNestedPaths(t *testing.T) {
	directory := t.TempDir()
	valid, err := ExportArtifactPath(directory, "statistics-global-1-2-3.csv")
	if err != nil || filepath.Dir(valid) != directory {
		t.Fatalf("valid path = %q, err=%v", valid, err)
	}
	for _, stored := range []string{"", ".", "..", "../outside.csv", `..\outside.csv`, "nested/file.csv", `nested\file.csv`} {
		if path, pathErr := ExportArtifactPath(directory, stored); pathErr == nil {
			t.Errorf("stored %q resolved to %q", stored, path)
		}
	}
}

func TestSecureExportDirectoryRejectsSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks may require elevated Windows privileges")
	}
	root := t.TempDir()
	realDirectory := filepath.Join(root, "real")
	if err := os.Mkdir(realDirectory, 0o700); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(realDirectory, link); err != nil {
		t.Fatalf("Symlink: %v", err)
	}
	if _, err := SecureExportDirectory(link); err == nil {
		t.Fatal("symlink export directory was accepted")
	}
	child := filepath.Join(realDirectory, "child")
	if err := os.Mkdir(child, 0o700); err != nil {
		t.Fatalf("Mkdir child: %v", err)
	}
	componentLink := filepath.Join(root, "component")
	if err := os.Symlink(realDirectory, componentLink); err != nil {
		t.Fatalf("Symlink component: %v", err)
	}
	if _, err := SecureExportDirectory(filepath.Join(componentLink, "child")); err == nil {
		t.Fatal("directory with a symlink component was accepted")
	}
}

func TestPublishExportArtifactRenamesWithoutReplacing(t *testing.T) {
	directory := t.TempDir()
	temporary := ".statistics.csv.claim.tmp"
	final := "statistics-global-1-2-3.csv"
	temporaryPath := filepath.Join(directory, temporary)
	if err := os.WriteFile(temporaryPath, []byte("first"), 0o600); err != nil {
		t.Fatalf("write temporary: %v", err)
	}
	if err := PublishExportArtifact(directory, temporary, final); err != nil {
		t.Fatalf("PublishExportArtifact: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(directory, final))
	if err != nil || string(data) != "first" {
		t.Fatalf("published data = %q, err=%v", data, err)
	}
	if _, err := os.Stat(temporaryPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temporary still exists: %v", err)
	}
	if err := os.WriteFile(temporaryPath, []byte("second"), 0o600); err != nil {
		t.Fatalf("write second temporary: %v", err)
	}
	if err := PublishExportArtifact(directory, temporary, final); !errors.Is(err, ErrExportWrite) {
		t.Fatalf("replace error = %v", err)
	}
	data, err = os.ReadFile(filepath.Join(directory, final))
	if err != nil || string(data) != "first" {
		t.Fatalf("existing final was replaced: %q, err=%v", data, err)
	}
	data, err = os.ReadFile(temporaryPath)
	if err != nil || string(data) != "second" {
		t.Fatalf("failed publish lost temporary: %q, err=%v", data, err)
	}
}

func TestRemoveExportArtifactOnlyRemovesRegularFiles(t *testing.T) {
	directory := t.TempDir()
	regular := "statistics-site-1-2-3.csv"
	if err := os.WriteFile(filepath.Join(directory, regular), []byte("data"), 0o600); err != nil {
		t.Fatalf("write regular: %v", err)
	}
	if err := RemoveExportArtifact(directory, regular); err != nil {
		t.Fatalf("remove regular: %v", err)
	}
	if err := RemoveExportArtifact(directory, regular); err != nil {
		t.Fatalf("remove missing should be idempotent: %v", err)
	}
	subdirectory := "statistics-account-1-2-3.csv"
	if err := os.Mkdir(filepath.Join(directory, subdirectory), 0o700); err != nil {
		t.Fatalf("mkdir fixture: %v", err)
	}
	if err := RemoveExportArtifact(directory, subdirectory); !errors.Is(err, ErrExportContract) {
		t.Fatalf("directory removal error = %v", err)
	}
	if runtime.GOOS != "windows" {
		target := filepath.Join(directory, "target")
		if err := os.WriteFile(target, []byte("secret"), 0o600); err != nil {
			t.Fatalf("write symlink target: %v", err)
		}
		link := "statistics-model-1-2-3.csv"
		if err := os.Symlink(target, filepath.Join(directory, link)); err != nil {
			t.Fatalf("symlink: %v", err)
		}
		if err := RemoveExportArtifact(directory, link); !errors.Is(err, ErrExportContract) {
			t.Fatalf("symlink removal error = %v", err)
		}
		data, err := os.ReadFile(target)
		if err != nil || string(data) != "secret" {
			t.Fatalf("symlink target changed: %q, err=%v", data, err)
		}
	}
}

func TestValidExportDownloadNameRejectsHeaderInjection(t *testing.T) {
	valid := "statistics-global-1-2-3.csv"
	if !validExportDownloadName(valid, dto.ExportFormatCSV) {
		t.Fatalf("valid name %q rejected", valid)
	}
	for _, name := range []string{
		"../" + valid,
		valid + "\r\nX-Injected: yes",
		"report.csv",
		"statistics-global-1-2-3.xlsx",
	} {
		if validExportDownloadName(name, dto.ExportFormatCSV) {
			t.Errorf("unsafe name %q accepted", name)
		}
	}
}
