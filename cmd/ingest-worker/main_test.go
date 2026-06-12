package main

import (
	"os"
	"path/filepath"
	"testing"

	"argus/internal/manifest"
)

func TestGroupEntriesByMonthBuildsBoundedBatches(t *testing.T) {
	entries := []manifest.Entry{
		{EntryID: "comments-2021-01-000", RecordType: "comments", Month: "2021-01", Year: "2021", MonthPart: "01", SizeBytes: 200},
		{EntryID: "comments-2021-01-001", RecordType: "comments", Month: "2021-01", Year: "2021", MonthPart: "01", SizeBytes: 200},
		{EntryID: "comments-2021-01-002", RecordType: "comments", Month: "2021-01", Year: "2021", MonthPart: "01", SizeBytes: 200},
		{EntryID: "submissions-2021-01-000", RecordType: "submissions", Month: "2021-01", Year: "2021", MonthPart: "01", SizeBytes: 50},
	}

	groups := groupEntriesByMonth(entries, 2, 450)
	if len(groups) != 3 {
		t.Fatalf("expected 3 groups, got %d", len(groups))
	}

	if got := len(groups[0].Entries); got != 2 {
		t.Fatalf("expected first group size 2, got %d", got)
	}
	if got := len(groups[1].Entries); got != 1 {
		t.Fatalf("expected second group size 1, got %d", got)
	}
	if got := groups[2].RecordType; got != "submissions" {
		t.Fatalf("expected final group for submissions, got %s", got)
	}
}

func TestExistingUsableOutputRemovesZeroByteFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "zero.parquet")

	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	exists, err := existingUsableOutput(path)
	if err != nil {
		t.Fatalf("existingUsableOutput: %v", err)
	}
	if exists {
		t.Fatalf("expected zero-byte output to be unusable")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected zero-byte output to be removed, stat err=%v", err)
	}
}

func TestExistingUsableOutputAcceptsNonEmptyFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.parquet")

	if err := os.WriteFile(path, []byte("ok"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	exists, err := existingUsableOutput(path)
	if err != nil {
		t.Fatalf("existingUsableOutput: %v", err)
	}
	if !exists {
		t.Fatalf("expected non-empty output to be reusable")
	}
}
