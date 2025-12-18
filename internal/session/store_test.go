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

func TestStore_LogCost(t *testing.T) {
	store, cleanup := testStore(t)
	defer cleanup()
	ctx := context.Background()

	// Create session and iteration first
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
		Prompt:    "test",
		Model:     "gpt-image-1",
		ImagePath: "/path/to/image.png",
		Timestamp: time.Now(),
	}
	if err := store.CreateIteration(ctx, iter); err != nil {
		t.Fatalf("CreateIteration() error = %v", err)
	}

	entry := &CostEntry{
		IterationID: iter.ID,
		SessionID:   sess.ID,
		Provider:    "openai",
		Model:       "gpt-image-1",
		Cost:        0.042,
		ImageCount:  1,
		Timestamp:   time.Now(),
	}

	if err := store.LogCost(ctx, entry); err != nil {
		t.Fatalf("LogCost() error = %v", err)
	}
}

func TestStore_GetTotalCost_Empty(t *testing.T) {
	store, cleanup := testStore(t)
	defer cleanup()
	ctx := context.Background()

	summary, err := store.GetTotalCost(ctx)
	if err != nil {
		t.Fatalf("GetTotalCost() error = %v", err)
	}

	if summary.TotalCost != 0 {
		t.Errorf("GetTotalCost() TotalCost = %v, want 0", summary.TotalCost)
	}
	if summary.ImageCount != 0 {
		t.Errorf("GetTotalCost() ImageCount = %v, want 0", summary.ImageCount)
	}
	if summary.EntryCount != 0 {
		t.Errorf("GetTotalCost() EntryCount = %v, want 0", summary.EntryCount)
	}
}

func TestStore_GetTotalCost_WithEntries(t *testing.T) {
	store, cleanup := testStore(t)
	defer cleanup()
	ctx := context.Background()

	// Create session and iterations
	sess := &Session{
		ID:        "test-session-1",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Model:     "gpt-image-1",
	}
	store.CreateSession(ctx, sess)

	now := time.Now()
	entries := []CostEntry{
		{IterationID: "i1", SessionID: sess.ID, Provider: "openai", Model: "gpt-image-1", Cost: 0.042, ImageCount: 1, Timestamp: now},
		{IterationID: "i2", SessionID: sess.ID, Provider: "openai", Model: "dall-e-3", Cost: 0.080, ImageCount: 1, Timestamp: now},
		{IterationID: "i3", SessionID: sess.ID, Provider: "openai", Model: "gpt-image-1", Cost: 0.167, ImageCount: 2, Timestamp: now},
	}

	// Create iterations for foreign key constraints
	for i, e := range entries {
		iter := &Iteration{
			ID:        e.IterationID,
			SessionID: sess.ID,
			Operation: "generate",
			Prompt:    "test",
			Model:     e.Model,
			ImagePath: "/path.png",
			Timestamp: now,
		}
		store.CreateIteration(ctx, iter)
		store.LogCost(ctx, &entries[i])
	}

	summary, err := store.GetTotalCost(ctx)
	if err != nil {
		t.Fatalf("GetTotalCost() error = %v", err)
	}

	expectedTotal := 0.042 + 0.080 + 0.167
	if !floatEquals(summary.TotalCost, expectedTotal) {
		t.Errorf("GetTotalCost() TotalCost = %v, want %v", summary.TotalCost, expectedTotal)
	}
	if summary.ImageCount != 4 {
		t.Errorf("GetTotalCost() ImageCount = %v, want 4", summary.ImageCount)
	}
	if summary.EntryCount != 3 {
		t.Errorf("GetTotalCost() EntryCount = %v, want 3", summary.EntryCount)
	}
}

func TestStore_GetSessionCost(t *testing.T) {
	store, cleanup := testStore(t)
	defer cleanup()
	ctx := context.Background()

	// Create two sessions
	sess1 := &Session{ID: "sess-1", CreatedAt: time.Now(), UpdatedAt: time.Now(), Model: "gpt-image-1"}
	sess2 := &Session{ID: "sess-2", CreatedAt: time.Now(), UpdatedAt: time.Now(), Model: "gpt-image-1"}
	store.CreateSession(ctx, sess1)
	store.CreateSession(ctx, sess2)

	now := time.Now()

	// Session 1 entries
	iter1 := &Iteration{ID: "i1", SessionID: sess1.ID, Operation: "generate", Prompt: "test", Model: "gpt-image-1", ImagePath: "/p.png", Timestamp: now}
	store.CreateIteration(ctx, iter1)
	store.LogCost(ctx, &CostEntry{IterationID: "i1", SessionID: sess1.ID, Provider: "openai", Model: "gpt-image-1", Cost: 0.042, ImageCount: 1, Timestamp: now})

	iter2 := &Iteration{ID: "i2", SessionID: sess1.ID, Operation: "edit", Prompt: "test", Model: "gpt-image-1", ImagePath: "/p.png", Timestamp: now}
	store.CreateIteration(ctx, iter2)
	store.LogCost(ctx, &CostEntry{IterationID: "i2", SessionID: sess1.ID, Provider: "openai", Model: "gpt-image-1", Cost: 0.042, ImageCount: 1, Timestamp: now})

	// Session 2 entry
	iter3 := &Iteration{ID: "i3", SessionID: sess2.ID, Operation: "generate", Prompt: "test", Model: "dall-e-3", ImagePath: "/p.png", Timestamp: now}
	store.CreateIteration(ctx, iter3)
	store.LogCost(ctx, &CostEntry{IterationID: "i3", SessionID: sess2.ID, Provider: "openai", Model: "dall-e-3", Cost: 0.120, ImageCount: 1, Timestamp: now})

	// Check session 1
	summary1, err := store.GetSessionCost(ctx, sess1.ID)
	if err != nil {
		t.Fatalf("GetSessionCost() error = %v", err)
	}
	if !floatEquals(summary1.TotalCost, 0.084) {
		t.Errorf("GetSessionCost(sess1) TotalCost = %v, want 0.084", summary1.TotalCost)
	}
	if summary1.ImageCount != 2 {
		t.Errorf("GetSessionCost(sess1) ImageCount = %v, want 2", summary1.ImageCount)
	}

	// Check session 2
	summary2, err := store.GetSessionCost(ctx, sess2.ID)
	if err != nil {
		t.Fatalf("GetSessionCost() error = %v", err)
	}
	if !floatEquals(summary2.TotalCost, 0.120) {
		t.Errorf("GetSessionCost(sess2) TotalCost = %v, want 0.120", summary2.TotalCost)
	}
}

func TestStore_GetCostByProvider(t *testing.T) {
	store, cleanup := testStore(t)
	defer cleanup()
	ctx := context.Background()

	sess := &Session{ID: "sess-1", CreatedAt: time.Now(), UpdatedAt: time.Now(), Model: "gpt-image-1"}
	store.CreateSession(ctx, sess)

	now := time.Now()

	// Create entries with different providers
	entries := []struct {
		iterID   string
		provider string
		cost     float64
		count    int
	}{
		{"i1", "openai", 0.042, 1},
		{"i2", "openai", 0.080, 1},
		{"i3", "openai", 0.167, 2},
		{"i4", "stability", 0.050, 1},
		{"i5", "stability", 0.100, 2},
	}

	for _, e := range entries {
		iter := &Iteration{ID: e.iterID, SessionID: sess.ID, Operation: "generate", Prompt: "test", Model: "model", ImagePath: "/p.png", Timestamp: now}
		store.CreateIteration(ctx, iter)
		store.LogCost(ctx, &CostEntry{IterationID: e.iterID, SessionID: sess.ID, Provider: e.provider, Model: "model", Cost: e.cost, ImageCount: e.count, Timestamp: now})
	}

	summaries, err := store.GetCostByProvider(ctx)
	if err != nil {
		t.Fatalf("GetCostByProvider() error = %v", err)
	}

	if len(summaries) != 2 {
		t.Fatalf("GetCostByProvider() returned %d providers, want 2", len(summaries))
	}

	providerMap := make(map[string]ProviderCostSummary)
	for _, s := range summaries {
		providerMap[s.Provider] = s
	}

	openaiSummary := providerMap["openai"]
	if !floatEquals(openaiSummary.TotalCost, 0.289) {
		t.Errorf("OpenAI TotalCost = %v, want 0.289", openaiSummary.TotalCost)
	}
	if openaiSummary.ImageCount != 4 {
		t.Errorf("OpenAI ImageCount = %v, want 4", openaiSummary.ImageCount)
	}

	stabilitySummary := providerMap["stability"]
	if !floatEquals(stabilitySummary.TotalCost, 0.150) {
		t.Errorf("Stability TotalCost = %v, want 0.150", stabilitySummary.TotalCost)
	}
	if stabilitySummary.ImageCount != 3 {
		t.Errorf("Stability ImageCount = %v, want 3", stabilitySummary.ImageCount)
	}
}

func TestStore_GetCostByDateRange(t *testing.T) {
	store, cleanup := testStore(t)
	defer cleanup()
	ctx := context.Background()

	sess := &Session{ID: "sess-1", CreatedAt: time.Now(), UpdatedAt: time.Now(), Model: "gpt-image-1"}
	store.CreateSession(ctx, sess)

	now := time.Now()
	yesterday := now.Add(-24 * time.Hour)
	twoDaysAgo := now.Add(-48 * time.Hour)
	threeDaysAgo := now.Add(-72 * time.Hour)

	// Create entries at different times
	entries := []struct {
		iterID string
		ts     time.Time
		cost   float64
		count  int
	}{
		{"i1", threeDaysAgo, 0.042, 1},
		{"i2", twoDaysAgo, 0.080, 1},
		{"i3", yesterday, 0.167, 2},
		{"i4", now, 0.040, 1},
	}

	for _, e := range entries {
		iter := &Iteration{ID: e.iterID, SessionID: sess.ID, Operation: "generate", Prompt: "test", Model: "model", ImagePath: "/p.png", Timestamp: e.ts}
		store.CreateIteration(ctx, iter)
		store.LogCost(ctx, &CostEntry{IterationID: e.iterID, SessionID: sess.ID, Provider: "openai", Model: "model", Cost: e.cost, ImageCount: e.count, Timestamp: e.ts})
	}

	// Query for last 2 days (yesterday and today)
	start := time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 0, 0, 0, 0, yesterday.Location())
	end := now.Add(24 * time.Hour)

	summary, err := store.GetCostByDateRange(ctx, start, end)
	if err != nil {
		t.Fatalf("GetCostByDateRange() error = %v", err)
	}

	// Should include yesterday (0.167) and today (0.040)
	expectedTotal := 0.167 + 0.040
	if !floatEquals(summary.TotalCost, expectedTotal) {
		t.Errorf("GetCostByDateRange() TotalCost = %v, want %v", summary.TotalCost, expectedTotal)
	}
	if summary.ImageCount != 3 {
		t.Errorf("GetCostByDateRange() ImageCount = %v, want 3", summary.ImageCount)
	}
	if summary.EntryCount != 2 {
		t.Errorf("GetCostByDateRange() EntryCount = %v, want 2", summary.EntryCount)
	}
}

func TestStore_GetCostByDateRange_NoResults(t *testing.T) {
	store, cleanup := testStore(t)
	defer cleanup()
	ctx := context.Background()

	// Query for a range with no data
	now := time.Now()
	start := now.Add(-100 * 24 * time.Hour)
	end := now.Add(-90 * 24 * time.Hour)

	summary, err := store.GetCostByDateRange(ctx, start, end)
	if err != nil {
		t.Fatalf("GetCostByDateRange() error = %v", err)
	}

	if summary.TotalCost != 0 {
		t.Errorf("GetCostByDateRange() TotalCost = %v, want 0", summary.TotalCost)
	}
	if summary.EntryCount != 0 {
		t.Errorf("GetCostByDateRange() EntryCount = %v, want 0", summary.EntryCount)
	}
}

func TestStore_GetCostByProvider_Empty(t *testing.T) {
	store, cleanup := testStore(t)
	defer cleanup()
	ctx := context.Background()

	summaries, err := store.GetCostByProvider(ctx)
	if err != nil {
		t.Fatalf("GetCostByProvider() error = %v", err)
	}

	if len(summaries) != 0 {
		t.Errorf("GetCostByProvider() returned %d providers, want 0", len(summaries))
	}
}

func TestStore_GetSessionCost_Empty(t *testing.T) {
	store, cleanup := testStore(t)
	defer cleanup()
	ctx := context.Background()

	sess := &Session{ID: "sess-1", CreatedAt: time.Now(), UpdatedAt: time.Now(), Model: "gpt-image-1"}
	store.CreateSession(ctx, sess)

	summary, err := store.GetSessionCost(ctx, sess.ID)
	if err != nil {
		t.Fatalf("GetSessionCost() error = %v", err)
	}

	if summary.TotalCost != 0 {
		t.Errorf("GetSessionCost() TotalCost = %v, want 0", summary.TotalCost)
	}
	if summary.EntryCount != 0 {
		t.Errorf("GetSessionCost() EntryCount = %v, want 0", summary.EntryCount)
	}
}

func TestStore_GetSessionCost_NonExistentSession(t *testing.T) {
	store, cleanup := testStore(t)
	defer cleanup()
	ctx := context.Background()

	summary, err := store.GetSessionCost(ctx, "non-existent")
	if err != nil {
		t.Fatalf("GetSessionCost() error = %v", err)
	}

	if summary.TotalCost != 0 {
		t.Errorf("GetSessionCost() TotalCost = %v, want 0", summary.TotalCost)
	}
}

func TestCostEntry_Structure(t *testing.T) {
	entry := CostEntry{
		IterationID: "iter-1",
		SessionID:   "sess-1",
		Provider:    "openai",
		Model:       "gpt-image-1",
		Cost:        0.042,
		ImageCount:  1,
		Timestamp:   time.Now(),
	}

	if entry.IterationID != "iter-1" {
		t.Error("CostEntry IterationID mismatch")
	}
	if entry.Provider != "openai" {
		t.Error("CostEntry Provider mismatch")
	}
	if entry.Cost != 0.042 {
		t.Error("CostEntry Cost mismatch")
	}
}

func TestCostSummary_Structure(t *testing.T) {
	summary := CostSummary{
		TotalCost:  0.289,
		ImageCount: 4,
		EntryCount: 3,
	}

	if summary.TotalCost != 0.289 {
		t.Error("CostSummary TotalCost mismatch")
	}
	if summary.ImageCount != 4 {
		t.Error("CostSummary ImageCount mismatch")
	}
	if summary.EntryCount != 3 {
		t.Error("CostSummary EntryCount mismatch")
	}
}

func TestProviderCostSummary_Structure(t *testing.T) {
	summary := ProviderCostSummary{
		Provider:   "openai",
		TotalCost:  0.289,
		ImageCount: 4,
	}

	if summary.Provider != "openai" {
		t.Error("ProviderCostSummary Provider mismatch")
	}
	if summary.TotalCost != 0.289 {
		t.Error("ProviderCostSummary TotalCost mismatch")
	}
}

func TestIterationMetadata_WithCost(t *testing.T) {
	m := &IterationMetadata{
		Size:     "1024x1024",
		Quality:  "high",
		Format:   "png",
		Cost:     0.167,
		Provider: "openai",
	}

	json := m.ToJSON()
	if json == "" {
		t.Error("ToJSON() returned empty string")
	}

	parsed := ParseIterationMetadata(json)
	if !floatEquals(parsed.Cost, 0.167) {
		t.Errorf("ParseIterationMetadata() Cost = %v, want 0.167", parsed.Cost)
	}
	if parsed.Provider != "openai" {
		t.Errorf("ParseIterationMetadata() Provider = %v, want openai", parsed.Provider)
	}
}

func floatEquals(a, b float64) bool {
	const epsilon = 0.0001
	return (a-b) < epsilon && (b-a) < epsilon
}
