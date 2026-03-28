package update

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

func TestCacheDir_Default(t *testing.T) {
	origCache := cacheDirOverride
	cacheDirOverride = ""
	defer func() { cacheDirOverride = origCache }()

	dir := cacheDir()
	if dir == "" {
		t.Fatal("expected non-empty cache dir")
	}
	if !filepath.IsAbs(dir) {
		t.Fatalf("expected absolute path, got %s", dir)
	}
	if filepath.Base(dir) != ".imggen" {
		t.Errorf("expected dir to end in .imggen, got %s", filepath.Base(dir))
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("expected cache dir to exist: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("expected cache dir to be a directory")
	}
}

func TestReadCache_NoFile(t *testing.T) {
	tmpDir := t.TempDir()
	origCache := cacheDirOverride
	cacheDirOverride = tmpDir
	defer func() { cacheDirOverride = origCache }()

	_, err := readCache()
	if err == nil {
		t.Fatal("expected error when cache file does not exist, got nil")
	}
}

func TestWriteCache_Success(t *testing.T) {
	tmpDir := t.TempDir()
	origCache := cacheDirOverride
	cacheDirOverride = tmpDir
	defer func() { cacheDirOverride = origCache }()

	c := &updateCache{
		LastCheck:     time.Now().Truncate(time.Second),
		LatestVersion: "3.2.1",
	}

	if err := writeCache(c); err != nil {
		t.Fatalf("writeCache failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, cacheFileName))
	if err != nil {
		t.Fatalf("could not read written cache file: %v", err)
	}

	var got updateCache
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("could not unmarshal written cache: %v", err)
	}
	if got.LatestVersion != "3.2.1" {
		t.Errorf("expected LatestVersion=3.2.1, got %s", got.LatestVersion)
	}
}

func TestFetchLatestVersion_InvalidJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{invalid`))
	}))
	defer ts.Close()

	origURL := releaseURL
	releaseURL = ts.URL
	defer func() { releaseURL = origURL }()

	_, err := fetchLatestVersion()
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestCheckForUpdate_FetchError(t *testing.T) {
	ts := newTestServer("", http.StatusInternalServerError)
	defer ts.Close()
	setup(t, ts.URL)

	info, err := CheckForUpdate("1.0.0")
	if err == nil {
		t.Fatal("expected error when server returns 500, got nil")
	}
	if info != nil {
		t.Fatalf("expected nil UpdateInfo on error, got %+v", info)
	}
}

func TestNormalize(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"1.0.0", "v1.0.0"},
		{"v1.0.0", "v1.0.0"},
		{"2.3.4", "v2.3.4"},
		{"v0.0.1", "v0.0.1"},
	}
	for _, tc := range tests {
		got := normalize(tc.input)
		if got != tc.want {
			t.Errorf("normalize(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestCacheRoundtrip(t *testing.T) {
	tmpDir := t.TempDir()
	origCache := cacheDirOverride
	cacheDirOverride = tmpDir
	defer func() { cacheDirOverride = origCache }()

	now := time.Now().Truncate(time.Second)
	original := &updateCache{
		LastCheck:     now,
		LatestVersion: "4.5.6",
	}

	if err := writeCache(original); err != nil {
		t.Fatalf("writeCache failed: %v", err)
	}

	got, err := readCache()
	if err != nil {
		t.Fatalf("readCache failed: %v", err)
	}
	if !got.LastCheck.Equal(now) {
		t.Errorf("LastCheck mismatch: got %v, want %v", got.LastCheck, now)
	}
	if got.LatestVersion != "4.5.6" {
		t.Errorf("LatestVersion mismatch: got %s, want 4.5.6", got.LatestVersion)
	}
}

func TestReadCache_CorruptJSON(t *testing.T) {
	tmpDir := t.TempDir()
	origCache := cacheDirOverride
	cacheDirOverride = tmpDir
	defer func() { cacheDirOverride = origCache }()

	// Write corrupt JSON to cache file.
	if err := os.WriteFile(filepath.Join(tmpDir, cacheFileName), []byte(`{garbage`), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := readCache()
	if err == nil {
		t.Fatal("expected error for corrupt JSON cache, got nil")
	}
}

func TestFetchLatestVersion_BadURL(t *testing.T) {
	origURL := releaseURL
	releaseURL = "http://[invalid"
	defer func() { releaseURL = origURL }()

	_, err := fetchLatestVersion()
	if err == nil {
		t.Fatal("expected error for bad URL, got nil")
	}
}

func TestCacheDir_HomeUnset(t *testing.T) {
	// When cacheDirOverride is empty and HOME is empty, os.UserHomeDir()
	// fails and cacheDir() returns "".
	origCache := cacheDirOverride
	cacheDirOverride = ""
	defer func() { cacheDirOverride = origCache }()

	origHome := os.Getenv("HOME")
	os.Setenv("HOME", "")
	defer os.Setenv("HOME", origHome)

	dir := cacheDir()
	if dir != "" {
		t.Errorf("expected empty cacheDir when HOME is unset, got %q", dir)
	}
}

func TestReadCache_NoCacheDir(t *testing.T) {
	// When cacheDir() returns "", readCache should return an error.
	origCache := cacheDirOverride
	cacheDirOverride = ""
	defer func() { cacheDirOverride = origCache }()

	origHome := os.Getenv("HOME")
	os.Setenv("HOME", "")
	defer os.Setenv("HOME", origHome)

	_, err := readCache()
	if err == nil {
		t.Fatal("expected error when cache dir is empty, got nil")
	}
	if !strings.Contains(err.Error(), "could not determine cache directory") {
		t.Errorf("expected 'could not determine cache directory', got: %s", err.Error())
	}
}

func TestWriteCache_NoCacheDir(t *testing.T) {
	// When cacheDir() returns "", writeCache should return an error.
	origCache := cacheDirOverride
	cacheDirOverride = ""
	defer func() { cacheDirOverride = origCache }()

	origHome := os.Getenv("HOME")
	os.Setenv("HOME", "")
	defer os.Setenv("HOME", origHome)

	err := writeCache(&updateCache{
		LastCheck:     time.Now(),
		LatestVersion: "1.0.0",
	})
	if err == nil {
		t.Fatal("expected error when cache dir is empty, got nil")
	}
	if !strings.Contains(err.Error(), "could not determine cache directory") {
		t.Errorf("expected 'could not determine cache directory', got: %s", err.Error())
	}
}

func TestCheckForUpdate_InvalidLatestTag(t *testing.T) {
	// Server returns an invalid semver tag. semver.Compare treats invalid
	// versions as equal (returns 0), so no update is reported and no error.
	ts := newTestServer("not-valid-semver-!!!!", http.StatusOK)
	defer ts.Close()
	setup(t, ts.URL)

	info, err := CheckForUpdate("1.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info != nil {
		t.Fatalf("expected nil UpdateInfo for invalid latest tag, got %+v", info)
	}
}
