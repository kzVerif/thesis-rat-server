package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateUniqueDoesNotOverwrite(t *testing.T) {
	store := &FileStore{dir: t.TempDir()}
	first, firstName, err := store.createUnique("report.txt")
	if err != nil {
		t.Fatal(err)
	}
	_ = first.Close()
	second, secondName, err := store.createUnique("report.txt")
	if err != nil {
		t.Fatal(err)
	}
	_ = second.Close()
	if firstName == secondName || !strings.HasPrefix(firstName, "report-") || filepath.Ext(firstName) != ".txt" {
		t.Fatalf("unexpected generated names: %q, %q", firstName, secondName)
	}
	if _, err := os.Stat(filepath.Join(store.dir, firstName)); err != nil {
		t.Fatal(err)
	}
}

func TestValidFilenameRejectsTraversal(t *testing.T) {
	for _, name := range []string{
		"", ".", "..", "../secret", `..\secret`, "line\nbreak.txt", ".env", " report.pdf",
		"report.pdf ", "report.", "bad:name.pdf", "CON", "con.txt", "COM1.log", "LPT9",
	} {
		if validFilename(name) {
			t.Errorf("validFilename(%q) = true", name)
		}
	}
	for _, name := range []string{"report.pdf", "รายงาน ประจำเดือน.pdf", "photo-01.JPG", "README"} {
		if !validFilename(name) {
			t.Errorf("validFilename(%q) = false", name)
		}
	}
}

func TestContentTypeFor(t *testing.T) {
	if got := contentTypeFor("report.pdf"); !strings.HasPrefix(got, "application/pdf") {
		t.Fatalf("contentTypeFor(report.pdf) = %q", got)
	}
	if got := contentTypeFor("unknown.custom-extension"); got != "application/octet-stream" {
		t.Fatalf("contentTypeFor(unknown) = %q", got)
	}
}

func TestValidateRenameExtension(t *testing.T) {
	if err := validateRenameExtension("report-id.pdf", "renamed.pdf"); err != nil {
		t.Fatalf("same extension rejected: %v", err)
	}
	for _, name := range []string{"renamed.PDF", "renamed.docx", "renamed"} {
		if err := validateRenameExtension("report-id.pdf", name); err == nil {
			t.Errorf("extension change to %q was accepted", name)
		}
	}
	if err := validateRenameExtension("README-id", "MANUAL"); err != nil {
		t.Fatalf("extensionless rename rejected: %v", err)
	}
}

func TestStorageFilePathSupportsLegacyAndAbsoluteValues(t *testing.T) {
	dir := t.TempDir()
	store := &FileStore{dir: dir}
	want := filepath.Join(dir, "report.txt")
	for _, value := range []string{"report.txt", want} {
		got, err := store.storageFilePath(value)
		if err != nil {
			t.Fatalf("storageFilePath(%q): %v", value, err)
		}
		if got != want {
			t.Fatalf("storageFilePath(%q) = %q, want %q", value, got, want)
		}
	}
}

func TestStorageFilePathRejectsOutsideUploadDirectory(t *testing.T) {
	store := &FileStore{dir: t.TempDir()}
	outside := filepath.Join(filepath.Dir(store.dir), "outside.txt")
	for _, value := range []string{"../outside.txt", outside} {
		if _, err := store.storageFilePath(value); err == nil {
			t.Errorf("storageFilePath(%q) accepted an outside path", value)
		}
	}
}
