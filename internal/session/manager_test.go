package session

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func testManager(t *testing.T) (*Manager, *Store, func()) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)

	store, err := NewStoreWithPath(dbPath)
	if err != nil {
		t.Fatalf("NewStoreWithPath() error = %v", err)
	}

	mgr := NewManager(store, "gpt-image-1")

	cleanup := func() {
		store.Close()
		os.Setenv("HOME", origHome)
	}
	return mgr, store, cleanup
}

func TestNewManager(t *testing.T) {
	mgr, _, cleanup := testManager(t)
	defer cleanup()

	if mgr == nil {
		t.Error("NewManager() returned nil")
	}
	if mgr.GetModel() != "gpt-image-1" {
		t.Errorf("GetModel() = %v, want gpt-image-1", mgr.GetModel())
	}
}

func TestManager_StartNew(t *testing.T) {
	mgr, _, cleanup := testManager(t)
	defer cleanup()
	ctx := context.Background()

	sess, err := mgr.StartNew(ctx, "Test Session")
	if err != nil {
		t.Fatalf("StartNew() error = %v", err)
	}

	if sess == nil {
		t.Fatal("StartNew() returned nil session")
	}
	if sess.ID == "" {
		t.Error("StartNew() session ID is empty")
	}
	if sess.Name != "Test Session" {
		t.Errorf("StartNew() session Name = %v, want Test Session", sess.Name)
	}
	if !mgr.HasSession() {
		t.Error("HasSession() = false, want true")
	}
	if mgr.Current() != sess {
		t.Error("Current() != started session")
	}
}

func TestManager_EnsureSession(t *testing.T) {
	mgr, _, cleanup := testManager(t)
	defer cleanup()
	ctx := context.Background()

	if mgr.HasSession() {
		t.Error("HasSession() = true before EnsureSession")
	}

	if err := mgr.EnsureSession(ctx); err != nil {
		t.Fatalf("EnsureSession() error = %v", err)
	}

	if !mgr.HasSession() {
		t.Error("HasSession() = false after EnsureSession")
	}

	firstSession := mgr.Current()

	if err := mgr.EnsureSession(ctx); err != nil {
		t.Fatalf("EnsureSession() second call error = %v", err)
	}

	if mgr.Current() != firstSession {
		t.Error("EnsureSession() created new session when one exists")
	}
}

func TestManager_AddIteration(t *testing.T) {
	mgr, _, cleanup := testManager(t)
	defer cleanup()
	ctx := context.Background()

	iter := &Iteration{
		Operation: "generate",
		Prompt:    "test prompt",
		Model:     "gpt-image-1",
		ImagePath: "/test/image.png",
	}

	if err := mgr.AddIteration(ctx, iter); err != nil {
		t.Fatalf("AddIteration() error = %v", err)
	}

	if !mgr.HasIteration() {
		t.Error("HasIteration() = false after AddIteration")
	}
	if mgr.CurrentIteration() == nil {
		t.Fatal("CurrentIteration() = nil after AddIteration")
	}
	if mgr.CurrentIteration().ID == "" {
		t.Error("CurrentIteration().ID is empty")
	}
	if mgr.CurrentIteration().SessionID == "" {
		t.Error("CurrentIteration().SessionID is empty")
	}
}

func TestManager_Undo(t *testing.T) {
	mgr, _, cleanup := testManager(t)
	defer cleanup()
	ctx := context.Background()

	iter1 := &Iteration{
		Operation: "generate",
		Prompt:    "first",
		Model:     "gpt-image-1",
		ImagePath: "/test/1.png",
	}
	if err := mgr.AddIteration(ctx, iter1); err != nil {
		t.Fatalf("AddIteration() error = %v", err)
	}
	firstID := mgr.CurrentIteration().ID

	iter2 := &Iteration{
		Operation: "edit",
		Prompt:    "second",
		Model:     "gpt-image-1",
		ImagePath: "/test/2.png",
	}
	if err := mgr.AddIteration(ctx, iter2); err != nil {
		t.Fatalf("AddIteration() error = %v", err)
	}

	if mgr.CurrentIteration().Prompt != "second" {
		t.Error("CurrentIteration() not second iteration")
	}

	prev, err := mgr.Undo(ctx)
	if err != nil {
		t.Fatalf("Undo() error = %v", err)
	}

	if prev.ID != firstID {
		t.Errorf("Undo() returned iteration ID = %v, want %v", prev.ID, firstID)
	}
	if mgr.CurrentIteration().ID != firstID {
		t.Error("CurrentIteration() not first after Undo")
	}
}

func TestManager_Undo_AtFirstImage(t *testing.T) {
	mgr, _, cleanup := testManager(t)
	defer cleanup()
	ctx := context.Background()

	iter := &Iteration{
		Operation: "generate",
		Prompt:    "first",
		Model:     "gpt-image-1",
		ImagePath: "/test/1.png",
	}
	if err := mgr.AddIteration(ctx, iter); err != nil {
		t.Fatalf("AddIteration() error = %v", err)
	}

	_, err := mgr.Undo(ctx)
	if err != ErrAtFirstImage {
		t.Errorf("Undo() error = %v, want ErrAtFirstImage", err)
	}
}

func TestManager_Undo_NoIteration(t *testing.T) {
	mgr, _, cleanup := testManager(t)
	defer cleanup()
	ctx := context.Background()

	_, err := mgr.Undo(ctx)
	if err != ErrNoIteration {
		t.Errorf("Undo() error = %v, want ErrNoIteration", err)
	}
}

func TestManager_History(t *testing.T) {
	mgr, _, cleanup := testManager(t)
	defer cleanup()
	ctx := context.Background()

	history, err := mgr.History(ctx)
	if err != nil {
		t.Fatalf("History() error = %v", err)
	}
	if history != nil {
		t.Error("History() should be nil without session")
	}

	for i := 0; i < 3; i++ {
		iter := &Iteration{
			Operation: "generate",
			Prompt:    "test",
			Model:     "gpt-image-1",
			ImagePath: "/test.png",
		}
		if err := mgr.AddIteration(ctx, iter); err != nil {
			t.Fatalf("AddIteration() error = %v", err)
		}
	}

	history, err = mgr.History(ctx)
	if err != nil {
		t.Fatalf("History() error = %v", err)
	}
	if len(history) != 3 {
		t.Errorf("History() returned %d iterations, want 3", len(history))
	}
}

func TestManager_ListSessions(t *testing.T) {
	mgr, _, cleanup := testManager(t)
	defer cleanup()
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if _, err := mgr.StartNew(ctx, ""); err != nil {
			t.Fatalf("StartNew() error = %v", err)
		}
	}

	sessions, err := mgr.ListSessions(ctx)
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if len(sessions) != 3 {
		t.Errorf("ListSessions() returned %d sessions, want 3", len(sessions))
	}
}

func TestManager_Load(t *testing.T) {
	mgr, _, cleanup := testManager(t)
	defer cleanup()
	ctx := context.Background()

	sess, err := mgr.StartNew(ctx, "Original")
	if err != nil {
		t.Fatalf("StartNew() error = %v", err)
	}
	sessionID := sess.ID

	iter := &Iteration{
		Operation: "generate",
		Prompt:    "test",
		Model:     "gpt-image-1",
		ImagePath: "/test.png",
	}
	if err := mgr.AddIteration(ctx, iter); err != nil {
		t.Fatalf("AddIteration() error = %v", err)
	}
	iterID := mgr.CurrentIteration().ID

	if _, err := mgr.StartNew(ctx, "New"); err != nil {
		t.Fatalf("StartNew() error = %v", err)
	}

	if mgr.Current().ID == sessionID {
		t.Error("Current session should be new one")
	}

	if err := mgr.Load(ctx, sessionID); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if mgr.Current().ID != sessionID {
		t.Errorf("Load() Current().ID = %v, want %v", mgr.Current().ID, sessionID)
	}
	if mgr.CurrentIteration().ID != iterID {
		t.Errorf("Load() CurrentIteration().ID = %v, want %v", mgr.CurrentIteration().ID, iterID)
	}
}

func TestManager_DeleteSession(t *testing.T) {
	mgr, _, cleanup := testManager(t)
	defer cleanup()
	ctx := context.Background()

	sess, err := mgr.StartNew(ctx, "ToDelete")
	if err != nil {
		t.Fatalf("StartNew() error = %v", err)
	}
	sessionID := sess.ID

	if err := mgr.DeleteSession(ctx, sessionID); err != nil {
		t.Fatalf("DeleteSession() error = %v", err)
	}

	if mgr.HasSession() {
		t.Error("HasSession() = true after deleting current session")
	}
}

func TestManager_RenameSession(t *testing.T) {
	mgr, _, cleanup := testManager(t)
	defer cleanup()
	ctx := context.Background()

	if err := mgr.RenameSession(ctx, "New Name"); err != ErrNoSession {
		t.Errorf("RenameSession() without session error = %v, want ErrNoSession", err)
	}

	if _, err := mgr.StartNew(ctx, "Original"); err != nil {
		t.Fatalf("StartNew() error = %v", err)
	}

	if err := mgr.RenameSession(ctx, "Renamed"); err != nil {
		t.Fatalf("RenameSession() error = %v", err)
	}

	if mgr.Current().Name != "Renamed" {
		t.Errorf("Current().Name = %v, want Renamed", mgr.Current().Name)
	}
}

func TestManager_SetModel(t *testing.T) {
	mgr, _, cleanup := testManager(t)
	defer cleanup()
	ctx := context.Background()

	mgr.SetModel("dall-e-3")
	if mgr.GetModel() != "dall-e-3" {
		t.Errorf("GetModel() = %v, want dall-e-3", mgr.GetModel())
	}

	if _, err := mgr.StartNew(ctx, ""); err != nil {
		t.Fatalf("StartNew() error = %v", err)
	}

	mgr.SetModel("dall-e-2")
	if mgr.Current().Model != "dall-e-2" {
		t.Errorf("Current().Model = %v, want dall-e-2", mgr.Current().Model)
	}
}

func TestManager_ImagePath(t *testing.T) {
	mgr, _, cleanup := testManager(t)
	defer cleanup()
	ctx := context.Background()

	if path := mgr.ImagePath(); path != "" {
		t.Errorf("ImagePath() without session = %v, want empty", path)
	}

	if _, err := mgr.StartNew(ctx, ""); err != nil {
		t.Fatalf("StartNew() error = %v", err)
	}

	path := mgr.ImagePath()
	if path == "" {
		t.Error("ImagePath() with session is empty")
	}
}

func TestManager_CurrentImagePath(t *testing.T) {
	mgr, _, cleanup := testManager(t)
	defer cleanup()
	ctx := context.Background()

	if path := mgr.CurrentImagePath(); path != "" {
		t.Errorf("CurrentImagePath() without iteration = %v, want empty", path)
	}

	iter := &Iteration{
		Operation: "generate",
		Prompt:    "test",
		Model:     "gpt-image-1",
		ImagePath: "/test/image.png",
	}
	if err := mgr.AddIteration(ctx, iter); err != nil {
		t.Fatalf("AddIteration() error = %v", err)
	}

	if path := mgr.CurrentImagePath(); path != "/test/image.png" {
		t.Errorf("CurrentImagePath() = %v, want /test/image.png", path)
	}
}

func TestManager_IterationCount(t *testing.T) {
	mgr, _, cleanup := testManager(t)
	defer cleanup()
	ctx := context.Background()

	count, err := mgr.IterationCount(ctx)
	if err != nil {
		t.Fatalf("IterationCount() error = %v", err)
	}
	if count != 0 {
		t.Errorf("IterationCount() without session = %v, want 0", count)
	}

	if _, err := mgr.StartNew(ctx, ""); err != nil {
		t.Fatalf("StartNew() error = %v", err)
	}

	for i := 0; i < 5; i++ {
		iter := &Iteration{
			Operation: "generate",
			Prompt:    "test",
			Model:     "gpt-image-1",
			ImagePath: "/test.png",
		}
		if err := mgr.AddIteration(ctx, iter); err != nil {
			t.Fatalf("AddIteration() error = %v", err)
		}
	}

	count, err = mgr.IterationCount(ctx)
	if err != nil {
		t.Fatalf("IterationCount() error = %v", err)
	}
	if count != 5 {
		t.Errorf("IterationCount() = %v, want 5", count)
	}
}
