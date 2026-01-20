package notes

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewStore(t *testing.T) {
	store := NewStore("/tmp/test-notes.json")
	if store == nil {
		t.Fatal("NewStore returned nil")
	}
}

func TestFirstLine(t *testing.T) {
	tests := []struct {
		note string
		want string
	}{
		{"single line", "single line"},
		{"first\nsecond\nthird", "first"},
		{"", ""},
		{"line with\nnewline", "line with"},
		{"no newline at end", "no newline at end"},
	}

	for _, tt := range tests {
		got := FirstLine(tt.note)
		if got != tt.want {
			t.Errorf("FirstLine(%q) = %q, want %q", tt.note, got, tt.want)
		}
	}
}

func TestStoreOperations(t *testing.T) {
	// Create temp file
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "notes.json")

	store := NewStore(filePath)

	// Load from non-existent file should not error
	if err := store.Load(); err != nil {
		t.Fatalf("Load from non-existent file failed: %v", err)
	}

	// Set a note
	if err := store.Set("session1", "Test note"); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Get the note
	note := store.Get("session1")
	if note != "Test note" {
		t.Errorf("Get('session1') = %q, want 'Test note'", note)
	}

	// Get non-existent note
	note = store.Get("nonexistent")
	if note != "" {
		t.Errorf("Get('nonexistent') = %q, want empty string", note)
	}

	// File should exist now
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Error("notes file should exist after Set")
	}
}

func TestStoreDelete(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "notes.json")

	store := NewStore(filePath)
	store.Set("session1", "Note 1")
	store.Set("session2", "Note 2")

	// Delete session1
	if err := store.Delete("session1"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// session1 should be gone
	if note := store.Get("session1"); note != "" {
		t.Errorf("deleted note should be empty, got %q", note)
	}

	// session2 should still exist
	if note := store.Get("session2"); note != "Note 2" {
		t.Errorf("session2 note should still be 'Note 2', got %q", note)
	}
}

func TestStorePersistence(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "notes.json")

	// Create and populate store
	store1 := NewStore(filePath)
	store1.Set("session1", "Persistent note")

	// Create new store and load
	store2 := NewStore(filePath)
	if err := store2.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Note should persist
	note := store2.Get("session1")
	if note != "Persistent note" {
		t.Errorf("loaded note = %q, want 'Persistent note'", note)
	}
}

func TestStoreGetAll(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "notes.json")

	store := NewStore(filePath)
	store.Set("session1", "Note 1")
	store.Set("session2", "Note 2")

	all := store.GetAll()

	if len(all) != 2 {
		t.Errorf("GetAll() returned %d notes, want 2", len(all))
	}

	if all["session1"] != "Note 1" {
		t.Errorf("all['session1'] = %q, want 'Note 1'", all["session1"])
	}
	if all["session2"] != "Note 2" {
		t.Errorf("all['session2'] = %q, want 'Note 2'", all["session2"])
	}
}

func TestStoreCleanup(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "notes.json")

	store := NewStore(filePath)
	store.Set("session1", "Note 1")
	store.Set("session2", "Note 2")
	store.Set("session3", "Note 3")

	// Cleanup - only session1 exists
	existingSessions := []string{"session1"}
	if err := store.Cleanup(existingSessions); err != nil {
		t.Fatalf("Cleanup failed: %v", err)
	}

	// Only session1 should remain
	all := store.GetAll()
	if len(all) != 1 {
		t.Errorf("after cleanup, expected 1 note, got %d", len(all))
	}
	if _, ok := all["session1"]; !ok {
		t.Error("session1 should still exist after cleanup")
	}
}

func TestStoreMultilineNote(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "notes.json")

	store := NewStore(filePath)

	multiline := "Line 1\nLine 2\nLine 3"
	store.Set("session", multiline)

	note := store.Get("session")
	if note != multiline {
		t.Errorf("multiline note not preserved: got %q, want %q", note, multiline)
	}
}

func TestStoreSpecialCharacters(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "notes.json")

	store := NewStore(filePath)

	special := `Note with "quotes" and 'apostrophes' and \backslashes\`
	store.Set("session", special)

	// Load in new store
	store2 := NewStore(filePath)
	store2.Load()

	note := store2.Get("session")
	if note != special {
		t.Errorf("special chars not preserved: got %q, want %q", note, special)
	}
}
