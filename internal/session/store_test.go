package session

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func testStore(t *testing.T) (*Store, func()) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewStoreWithPath(dbPath)
	if err != nil {
		t.Fatalf("NewStoreWithPath() error = %v", err)
	}

	cleanup := func() {
		store.Close()
	}
	return store, cleanup
}

func TestNewStoreWithPath(t *testing.T) {
	store, cleanup := testStore(t)
	defer cleanup()

	if store == nil {
		t.Error("NewStoreWithPath() returned nil")
	}
}

func TestStore_CreateAndGetSession(t *testing.T) {
	store, cleanup := testStore(t)
	defer cleanup()
	ctx := context.Background()

	sess := &Session{
		ID:        "test-session-1",
		Name:      "Test Session",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Model:     "gpt-image-1",
	}

	if err := store.CreateSession(ctx, sess); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	got, err := store.GetSession(ctx, sess.ID)
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}

	if got.ID != sess.ID {
		t.Errorf("GetSession() ID = %v, want %v", got.ID, sess.ID)
	}
	if got.Name != sess.Name {
		t.Errorf("GetSession() Name = %v, want %v", got.Name, sess.Name)
	}
	if got.Model != sess.Model {
		t.Errorf("GetSession() Model = %v, want %v", got.Model, sess.Model)
	}
}

func TestStore_UpdateSession(t *testing.T) {
	store, cleanup := testStore(t)
	defer cleanup()
	ctx := context.Background()

	sess := &Session{
		ID:        "test-session-1",
		Name:      "Original Name",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Model:     "gpt-image-1",
	}
	if err := store.CreateSession(ctx, sess); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	sess.Name = "Updated Name"
	sess.CurrentIterationID = "iter-1"
	sess.UpdatedAt = time.Now()

	if err := store.UpdateSession(ctx, sess); err != nil {
		t.Fatalf("UpdateSession() error = %v", err)
	}

	got, err := store.GetSession(ctx, sess.ID)
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}

	if got.Name != "Updated Name" {
		t.Errorf("GetSession() Name = %v, want %v", got.Name, "Updated Name")
	}
	if got.CurrentIterationID != "iter-1" {
		t.Errorf("GetSession() CurrentIterationID = %v, want %v", got.CurrentIterationID, "iter-1")
	}
}

func TestStore_DeleteSession(t *testing.T) {
	store, cleanup := testStore(t)
	defer cleanup()
	ctx := context.Background()

	sess := &Session{
		ID:        "test-session-1",
		Name:      "Test",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Model:     "gpt-image-1",
	}
	if err := store.CreateSession(ctx, sess); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	if err := store.DeleteSession(ctx, sess.ID); err != nil {
		t.Fatalf("DeleteSession() error = %v", err)
	}

	_, err := store.GetSession(ctx, sess.ID)
	if err == nil {
		t.Error("GetSession() after delete should return error")
	}
}

func TestStore_ListSessions(t *testing.T) {
	store, cleanup := testStore(t)
	defer cleanup()
	ctx := context.Background()

	now := time.Now()
	sessions := []*Session{
		{ID: "s1", Name: "First", CreatedAt: now, UpdatedAt: now.Add(-2 * time.Hour), Model: "gpt-image-1"},
		{ID: "s2", Name: "Second", CreatedAt: now, UpdatedAt: now.Add(-1 * time.Hour), Model: "dall-e-3"},
		{ID: "s3", Name: "Third", CreatedAt: now, UpdatedAt: now, Model: "gpt-image-1"},
	}

	for _, s := range sessions {
		if err := store.CreateSession(ctx, s); err != nil {
			t.Fatalf("CreateSession() error = %v", err)
		}
	}

	got, err := store.ListSessions(ctx)
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}

	if len(got) != 3 {
		t.Errorf("ListSessions() returned %d sessions, want 3", len(got))
	}

	if got[0].ID != "s3" {
		t.Errorf("ListSessions() first session ID = %v, want s3 (most recent)", got[0].ID)
	}
}

func TestStore_CreateAndGetIteration(t *testing.T) {
	store, cleanup := testStore(t)
	defer cleanup()
	ctx := context.Background()

	sess := &Session{
		ID:        "test-session-1",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Model:     "gpt-image-1",
	}
	if err := store.CreateSession(ctx, sess); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	iter := &Iteration{
		ID:        "iter-1",
		SessionID: sess.ID,
		Operation: "generate",
		Prompt:    "test prompt",
		Model:     "gpt-image-1",
		ImagePath: "/path/to/image.png",
		Timestamp: time.Now(),
		Metadata:  IterationMetadata{Size: "1024x1024", Format: "png"},
	}

	if err := store.CreateIteration(ctx, iter); err != nil {
		t.Fatalf("CreateIteration() error = %v", err)
	}

	got, err := store.GetIteration(ctx, iter.ID)
	if err != nil {
		t.Fatalf("GetIteration() error = %v", err)
	}

	if got.ID != iter.ID {
		t.Errorf("GetIteration() ID = %v, want %v", got.ID, iter.ID)
	}
	if got.Prompt != iter.Prompt {
		t.Errorf("GetIteration() Prompt = %v, want %v", got.Prompt, iter.Prompt)
	}
	if got.Operation != iter.Operation {
		t.Errorf("GetIteration() Operation = %v, want %v", got.Operation, iter.Operation)
	}
	if got.Metadata.Size != "1024x1024" {
		t.Errorf("GetIteration() Metadata.Size = %v, want %v", got.Metadata.Size, "1024x1024")
	}
}

func TestStore_ListIterations(t *testing.T) {
	store, cleanup := testStore(t)
	defer cleanup()
	ctx := context.Background()

	sess := &Session{
		ID:        "test-session-1",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Model:     "gpt-image-1",
	}
	if err := store.CreateSession(ctx, sess); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	now := time.Now()
	iterations := []*Iteration{
		{ID: "i1", SessionID: sess.ID, Operation: "generate", Prompt: "first", Model: "gpt-image-1", ImagePath: "/p1.png", Timestamp: now.Add(-2 * time.Second)},
		{ID: "i2", SessionID: sess.ID, Operation: "edit", Prompt: "second", Model: "gpt-image-1", ImagePath: "/p2.png", Timestamp: now.Add(-1 * time.Second)},
		{ID: "i3", SessionID: sess.ID, Operation: "edit", Prompt: "third", Model: "gpt-image-1", ImagePath: "/p3.png", Timestamp: now},
	}

	for _, i := range iterations {
		if err := store.CreateIteration(ctx, i); err != nil {
			t.Fatalf("CreateIteration() error = %v", err)
		}
	}

	got, err := store.ListIterations(ctx, sess.ID)
	if err != nil {
		t.Fatalf("ListIterations() error = %v", err)
	}

	if len(got) != 3 {
		t.Errorf("ListIterations() returned %d iterations, want 3", len(got))
	}

	if got[0].ID != "i1" {
		t.Errorf("ListIterations() first iteration ID = %v, want i1 (oldest first)", got[0].ID)
	}
}

func TestStore_CountIterations(t *testing.T) {
	store, cleanup := testStore(t)
	defer cleanup()
	ctx := context.Background()

	sess := &Session{
		ID:        "test-session-1",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Model:     "gpt-image-1",
	}
	if err := store.CreateSession(ctx, sess); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	count, err := store.CountIterations(ctx, sess.ID)
	if err != nil {
		t.Fatalf("CountIterations() error = %v", err)
	}
	if count != 0 {
		t.Errorf("CountIterations() = %v, want 0", count)
	}

	iter := &Iteration{
		ID:        "iter-1",
		SessionID: sess.ID,
		Operation: "generate",
		Prompt:    "test",
		Model:     "gpt-image-1",
		ImagePath: "/p.png",
		Timestamp: time.Now(),
	}
	if err := store.CreateIteration(ctx, iter); err != nil {
		t.Fatalf("CreateIteration() error = %v", err)
	}

	count, err = store.CountIterations(ctx, sess.ID)
	if err != nil {
		t.Fatalf("CountIterations() error = %v", err)
	}
	if count != 1 {
		t.Errorf("CountIterations() = %v, want 1", count)
	}
}

func TestImageDir(t *testing.T) {
	dir, err := ImageDir("test-session")
	if err != nil {
		t.Fatalf("ImageDir() error = %v", err)
	}

	if dir == "" {
		t.Error("ImageDir() returned empty string")
	}

	if filepath.Base(dir) != "test-session" {
		t.Errorf("ImageDir() base = %v, want test-session", filepath.Base(dir))
	}
}

func TestEnsureImageDir(t *testing.T) {
	origHome := os.Getenv("HOME")
	tmpDir := t.TempDir()
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	dir, err := EnsureImageDir("test-session")
	if err != nil {
		t.Fatalf("EnsureImageDir() error = %v", err)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("os.Stat() error = %v", err)
	}
	if !info.IsDir() {
		t.Error("EnsureImageDir() did not create directory")
	}
}

func TestIterationMetadata_ToJSON(t *testing.T) {
	m := &IterationMetadata{
		Size:        "1024x1024",
		Quality:     "hd",
		Format:      "png",
		Transparent: true,
	}

	json := m.ToJSON()
	if json == "" {
		t.Error("ToJSON() returned empty string")
	}
}

func TestParseIterationMetadata(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  IterationMetadata
	}{
		{
			name:  "empty string",
			input: "",
			want:  IterationMetadata{},
		},
		{
			name:  "valid json",
			input: `{"size":"1024x1024","quality":"hd"}`,
			want:  IterationMetadata{Size: "1024x1024", Quality: "hd"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseIterationMetadata(tt.input)
			if got.Size != tt.want.Size {
				t.Errorf("ParseIterationMetadata() Size = %v, want %v", got.Size, tt.want.Size)
			}
			if got.Quality != tt.want.Quality {
				t.Errorf("ParseIterationMetadata() Quality = %v, want %v", got.Quality, tt.want.Quality)
			}
		})
	}
}

func TestFormatTimestamp(t *testing.T) {
	ts := time.Date(2024, 12, 17, 10, 30, 45, 0, time.UTC)
	got := FormatTimestamp(ts)
	want := "2024-12-17 10:30:45"

	if got != want {
		t.Errorf("FormatTimestamp() = %v, want %v", got, want)
	}
}
