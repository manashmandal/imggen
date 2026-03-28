package update

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// newTestServer returns an httptest.Server that responds with the given tag_name.
func newTestServer(tagName string, statusCode int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if statusCode != http.StatusOK {
			w.WriteHeader(statusCode)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"tag_name": tagName})
	}))
}

// setup configures the package-level vars to point at a temp cache dir and
// the provided test server URL. It returns a cleanup function.
func setup(t *testing.T, serverURL string) {
	t.Helper()

	origURL := releaseURL
	origCache := cacheDirOverride

	tmpDir := t.TempDir()
	cacheDirOverride = tmpDir
	releaseURL = serverURL

	t.Cleanup(func() {
		releaseURL = origURL
		cacheDirOverride = origCache
	})
}

func TestCheckForUpdate_DevVersion(t *testing.T) {
	info, err := CheckForUpdate("dev")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info != nil {
		t.Fatalf("expected nil UpdateInfo for dev version, got %+v", info)
	}

	info, err = CheckForUpdate("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info != nil {
		t.Fatalf("expected nil UpdateInfo for empty version, got %+v", info)
	}
}

func TestCheckForUpdate_NewVersionAvailable(t *testing.T) {
	ts := newTestServer("v2.0.0", http.StatusOK)
	defer ts.Close()
	setup(t, ts.URL)

	info, err := CheckForUpdate("1.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info == nil {
		t.Fatal("expected UpdateInfo, got nil")
	}
	if info.CurrentVersion != "1.0.0" {
		t.Errorf("expected CurrentVersion=1.0.0, got %s", info.CurrentVersion)
	}
	if info.LatestVersion != "2.0.0" {
		t.Errorf("expected LatestVersion=2.0.0, got %s", info.LatestVersion)
	}
}

func TestCheckForUpdate_AlreadyLatest(t *testing.T) {
	ts := newTestServer("v1.0.0", http.StatusOK)
	defer ts.Close()
	setup(t, ts.URL)

	info, err := CheckForUpdate("1.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info != nil {
		t.Fatalf("expected nil UpdateInfo when already latest, got %+v", info)
	}
}

func TestCheckForUpdate_CacheHit(t *testing.T) {
	var hitCount atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"tag_name": "v2.0.0"})
	}))
	defer ts.Close()
	setup(t, ts.URL)

	// First call should hit the server.
	info, err := CheckForUpdate("1.0.0")
	if err != nil {
		t.Fatalf("unexpected error on first call: %v", err)
	}
	if info == nil {
		t.Fatal("expected UpdateInfo on first call, got nil")
	}
	if hitCount.Load() != 1 {
		t.Fatalf("expected 1 HTTP hit after first call, got %d", hitCount.Load())
	}

	// Second call should use cache, no additional HTTP hit.
	info, err = CheckForUpdate("1.0.0")
	if err != nil {
		t.Fatalf("unexpected error on second call: %v", err)
	}
	if info == nil {
		t.Fatal("expected UpdateInfo on second call, got nil")
	}
	if hitCount.Load() != 1 {
		t.Fatalf("expected still 1 HTTP hit after cached call, got %d", hitCount.Load())
	}
}

func TestCheckForUpdate_CacheExpired(t *testing.T) {
	var hitCount atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"tag_name": "v2.0.0"})
	}))
	defer ts.Close()
	setup(t, ts.URL)

	// Write an expired cache entry.
	expired := &updateCache{
		LastCheck:     time.Now().Add(-25 * time.Hour),
		LatestVersion: "1.0.0",
	}
	data, _ := json.Marshal(expired)
	os.WriteFile(filepath.Join(cacheDirOverride, cacheFileName), data, 0o644)

	// Should fetch because cache is expired.
	info, err := CheckForUpdate("1.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info == nil {
		t.Fatal("expected UpdateInfo after expired cache, got nil")
	}
	if hitCount.Load() != 1 {
		t.Fatalf("expected 1 HTTP hit for expired cache, got %d", hitCount.Load())
	}
}

func TestFetchLatestVersion(t *testing.T) {
	ts := newTestServer("v1.5.3", http.StatusOK)
	defer ts.Close()

	origURL := releaseURL
	releaseURL = ts.URL
	defer func() { releaseURL = origURL }()

	version, err := fetchLatestVersion()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if version != "1.5.3" {
		t.Errorf("expected version=1.5.3, got %s", version)
	}
}

func TestFetchLatestVersion_Error(t *testing.T) {
	ts := newTestServer("", http.StatusInternalServerError)
	defer ts.Close()

	origURL := releaseURL
	releaseURL = ts.URL
	defer func() { releaseURL = origURL }()

	_, err := fetchLatestVersion()
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
}
