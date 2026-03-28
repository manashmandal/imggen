package update

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

const (
	cacheFileName  = "update-check.json"
	checkCacheTTL  = 24 * time.Hour
	httpTimeout    = 5 * time.Second
)

// releaseURL is the GitHub API endpoint for the latest release.
// It is a var so tests can override it with an httptest server.
var releaseURL = githubAPI + "/repos/" + repoOwner + "/" + repoName + "/releases/latest"

// cacheDirOverride allows tests to redirect the cache to a temp directory.
var cacheDirOverride string

// UpdateInfo holds version comparison results when an update is available.
type UpdateInfo struct {
	CurrentVersion string
	LatestVersion  string
}

type updateCache struct {
	LastCheck     time.Time `json:"last_check"`
	LatestVersion string    `json:"latest_version"`
}

// githubRelease is the minimal GitHub API response we care about.
type githubRelease struct {
	TagName string `json:"tag_name"`
}

// CheckForUpdate checks whether a newer version of imggen is available.
// Returns nil if the current version is up to date, is "dev", or is empty.
func CheckForUpdate(currentVersion string) (*UpdateInfo, error) {
	if currentVersion == "" || currentVersion == "dev" {
		return nil, nil
	}

	latest, err := latestVersionWithCache()
	if err != nil {
		return nil, err
	}

	current := normalize(currentVersion)
	latestNorm := normalize(latest)

	if semver.Compare(latestNorm, current) > 0 {
		return &UpdateInfo{
			CurrentVersion: strings.TrimPrefix(current, "v"),
			LatestVersion:  strings.TrimPrefix(latestNorm, "v"),
		}, nil
	}

	return nil, nil
}

// latestVersionWithCache returns the latest version string, using a file cache
// to avoid hitting the GitHub API on every invocation.
func latestVersionWithCache() (string, error) {
	if c, err := readCache(); err == nil {
		if time.Since(c.LastCheck) < checkCacheTTL {
			return c.LatestVersion, nil
		}
	}

	latest, err := fetchLatestVersion()
	if err != nil {
		return "", err
	}

	_ = writeCache(&updateCache{
		LastCheck:     time.Now(),
		LatestVersion: latest,
	})

	return latest, nil
}

// fetchLatestVersion queries the GitHub releases API and returns the latest
// version string without a leading "v" prefix.
func fetchLatestVersion() (string, error) {
	client := &http.Client{Timeout: httpTimeout}

	req, err := http.NewRequest("GET", releaseURL, nil)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetching latest release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github API returned status %d", resp.StatusCode)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", fmt.Errorf("decoding response: %w", err)
	}

	return strings.TrimPrefix(release.TagName, "v"), nil
}

// normalize ensures a version string has a "v" prefix for semver comparison.
func normalize(v string) string {
	if !strings.HasPrefix(v, "v") {
		return "v" + v
	}
	return v
}

// cacheDir returns ~/.imggen/, creating it if it doesn't exist.
// If cacheDirOverride is set (for testing), it returns that instead.
func cacheDir() string {
	if cacheDirOverride != "" {
		_ = os.MkdirAll(cacheDirOverride, 0o755)
		return cacheDirOverride
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	dir := filepath.Join(home, ".imggen")
	_ = os.MkdirAll(dir, 0o755)
	return dir
}

// readCache reads and parses the update check cache file.
func readCache() (*updateCache, error) {
	dir := cacheDir()
	if dir == "" {
		return nil, fmt.Errorf("could not determine cache directory")
	}

	data, err := os.ReadFile(filepath.Join(dir, cacheFileName))
	if err != nil {
		return nil, err
	}

	var c updateCache
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// writeCache writes the update check cache to disk.
func writeCache(c *updateCache) error {
	dir := cacheDir()
	if dir == "" {
		return fmt.Errorf("could not determine cache directory")
	}

	data, err := json.Marshal(c)
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(dir, cacheFileName), data, 0o644)
}
