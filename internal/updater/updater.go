package updater

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultAPIURL         = "https://api.github.com/repos/tktkl/excel-image-converter/releases/latest"
	DefaultLatestURL      = "https://github.com/tktkl/excel-image-converter/releases/latest"
	DefaultReleaseBaseURL = "https://github.com/tktkl/excel-image-converter/releases/tag/"
)

type Checker struct {
	HTTPClient *http.Client
	APIURL     string
	LatestURL  string
	UserAgent  string
	Platform   string
}

type Result struct {
	CurrentVersion string `json:"current_version"`
	LatestVersion  string `json:"latest_version"`
	HasUpdate      bool   `json:"has_update"`
	ReleaseURL     string `json:"release_url"`
	DownloadURL    string `json:"download_url,omitempty"`
	Notes          string `json:"notes,omitempty"`
}

type githubRelease struct {
	TagName string        `json:"tag_name"`
	HTMLURL string        `json:"html_url"`
	Body    string        `json:"body"`
	Assets  []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func (c Checker) Check(ctx context.Context, currentVersion string) (Result, error) {
	currentVersion = normalizeVersion(currentVersion)
	if currentVersion == "" || currentVersion == "dev" {
		return Result{CurrentVersion: currentVersion}, nil
	}

	release, err := c.fetchLatestRelease(ctx)
	if err != nil {
		return Result{CurrentVersion: currentVersion}, err
	}

	latestVersion := normalizeVersion(release.TagName)
	result := Result{
		CurrentVersion: currentVersion,
		LatestVersion:  latestVersion,
		ReleaseURL:     release.HTMLURL,
		DownloadURL:    release.downloadURL(c.platform()),
		Notes:          strings.TrimSpace(release.Body),
	}
	if result.ReleaseURL == "" && latestVersion != "" {
		result.ReleaseURL = DefaultReleaseBaseURL + "v" + latestVersion
	}

	cmp, err := CompareVersions(latestVersion, currentVersion)
	if err != nil {
		return result, err
	}
	result.HasUpdate = cmp > 0
	return result, nil
}

func (c Checker) fetchLatestRelease(ctx context.Context) (githubRelease, error) {
	if release, err := c.fetchLatestReleaseFromAPI(ctx); err == nil {
		return release, nil
	}
	return c.fetchLatestReleaseFromRedirect(ctx)
}

func (c Checker) fetchLatestReleaseFromAPI(ctx context.Context) (githubRelease, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.apiURL(), nil)
	if err != nil {
		return githubRelease{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", c.userAgent())

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return githubRelease{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return githubRelease{}, fmt.Errorf("github api status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return githubRelease{}, err
	}
	var release githubRelease
	if err := json.Unmarshal(body, &release); err != nil {
		return githubRelease{}, err
	}
	if normalizeVersion(release.TagName) == "" {
		return githubRelease{}, errors.New("github api response missing tag_name")
	}
	return release, nil
}

func (c Checker) fetchLatestReleaseFromRedirect(ctx context.Context) (githubRelease, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.latestURL(), nil)
	if err != nil {
		return githubRelease{}, err
	}
	req.Header.Set("User-Agent", c.userAgent())

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return githubRelease{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return githubRelease{}, fmt.Errorf("github release status %d", resp.StatusCode)
	}

	finalURL := resp.Request.URL.String()
	tag := tagFromReleaseURL(finalURL)
	if tag == "" {
		return githubRelease{}, fmt.Errorf("github latest release did not redirect to a tag: %s", finalURL)
	}
	return githubRelease{TagName: tag, HTMLURL: finalURL}, nil
}

func CompareVersions(a, b string) (int, error) {
	aParts, err := parseVersionParts(a)
	if err != nil {
		return 0, err
	}
	bParts, err := parseVersionParts(b)
	if err != nil {
		return 0, err
	}
	maxLen := len(aParts)
	if len(bParts) > maxLen {
		maxLen = len(bParts)
	}
	for i := 0; i < maxLen; i++ {
		var av, bv int
		if i < len(aParts) {
			av = aParts[i]
		}
		if i < len(bParts) {
			bv = bParts[i]
		}
		if av > bv {
			return 1, nil
		}
		if av < bv {
			return -1, nil
		}
	}
	return 0, nil
}

func parseVersionParts(version string) ([]int, error) {
	version = normalizeVersion(version)
	if version == "" {
		return nil, errors.New("empty version")
	}
	if idx := strings.IndexAny(version, "-+"); idx >= 0 {
		version = version[:idx]
	}
	rawParts := strings.Split(version, ".")
	parts := make([]int, 0, len(rawParts))
	for _, raw := range rawParts {
		if raw == "" {
			return nil, fmt.Errorf("invalid version %q", version)
		}
		part, err := strconv.Atoi(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid version %q", version)
		}
		parts = append(parts, part)
	}
	return parts, nil
}

func normalizeVersion(version string) string {
	version = strings.TrimSpace(version)
	version = strings.TrimPrefix(version, "v")
	version = strings.TrimPrefix(version, "V")
	return version
}

func tagFromReleaseURL(rawURL string) string {
	const marker = "/releases/tag/"
	idx := strings.Index(rawURL, marker)
	if idx < 0 {
		return ""
	}
	tag := rawURL[idx+len(marker):]
	if cut := strings.IndexAny(tag, "?#/"); cut >= 0 {
		tag = tag[:cut]
	}
	return tag
}

func (r githubRelease) downloadURL(platform string) string {
	platform = strings.ToLower(platform)
	var preferred []string
	switch platform {
	case "windows":
		preferred = []string{"windows", ".exe"}
	case "darwin":
		preferred = []string{"mac", "darwin", ".dmg"}
	default:
		return ""
	}
	for _, asset := range r.Assets {
		name := strings.ToLower(asset.Name)
		if containsAny(name, preferred) {
			return asset.BrowserDownloadURL
		}
	}
	return ""
}

func containsAny(value string, candidates []string) bool {
	for _, candidate := range candidates {
		if strings.Contains(value, candidate) {
			return true
		}
	}
	return false
}

func (c Checker) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: 5 * time.Second}
}

func (c Checker) apiURL() string {
	if strings.TrimSpace(c.APIURL) != "" {
		return c.APIURL
	}
	return DefaultAPIURL
}

func (c Checker) latestURL() string {
	if strings.TrimSpace(c.LatestURL) != "" {
		return c.LatestURL
	}
	return DefaultLatestURL
}

func (c Checker) userAgent() string {
	if strings.TrimSpace(c.UserAgent) != "" {
		return c.UserAgent
	}
	return "ExcelImageConverter"
}

func (c Checker) platform() string {
	if strings.TrimSpace(c.Platform) != "" {
		return c.Platform
	}
	return runtime.GOOS
}
