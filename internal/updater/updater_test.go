package updater

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"
)

func TestCheckFindsNewerVersionFromAPI(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return response(http.StatusOK, `{
			"tag_name":"v1.0.14",
			"html_url":"https://github.com/tktkl/excel-image-converter/releases/tag/v1.0.14",
			"body":"更新说明",
			"assets":[
				{"name":"ExcelImageConverter-windows-amd64.exe","browser_download_url":"https://example.com/windows.exe"},
				{"name":"ExcelImageConverter-mac-arm64.dmg","browser_download_url":"https://example.com/mac.dmg"}
			]
		}`, req), nil
	})}

	result, err := (Checker{HTTPClient: client, APIURL: "https://example.com/api", Platform: "windows"}).Check(context.Background(), "1.0.13")
	if err != nil {
		t.Fatal(err)
	}
	if !result.HasUpdate {
		t.Fatal("HasUpdate = false, want true")
	}
	if result.LatestVersion != "1.0.14" {
		t.Fatalf("LatestVersion = %q, want 1.0.14", result.LatestVersion)
	}
	if result.DownloadURL != "https://example.com/windows.exe" {
		t.Fatalf("DownloadURL = %q, want windows asset", result.DownloadURL)
	}
}

func TestCheckFallsBackToLatestReleaseRedirect(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/api":
			return response(http.StatusServiceUnavailable, "api unavailable", req), nil
		case "/latest":
			resp := response(http.StatusFound, "", req)
			resp.Header.Set("Location", "https://example.com/releases/tag/v1.0.15")
			return resp, nil
		case "/releases/tag/v1.0.15":
			return response(http.StatusOK, "release page", req), nil
		default:
			return response(http.StatusNotFound, "not found", req), nil
		}
	})}

	result, err := (Checker{
		HTTPClient: client,
		APIURL:     "https://example.com/api",
		LatestURL:  "https://example.com/latest",
		Platform:   "windows",
	}).Check(context.Background(), "1.0.14")
	if err != nil {
		t.Fatal(err)
	}
	if !result.HasUpdate {
		t.Fatal("HasUpdate = false, want true")
	}
	if result.LatestVersion != "1.0.15" {
		t.Fatalf("LatestVersion = %q, want 1.0.15", result.LatestVersion)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func response(status int, body string, req *http.Request) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewBufferString(body)),
		Request:    req,
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		a    string
		b    string
		want int
	}{
		{a: "v1.0.14", b: "1.0.13", want: 1},
		{a: "1.0.13", b: "1.0.13", want: 0},
		{a: "1.0.9", b: "1.0.10", want: -1},
		{a: "1.2", b: "1.2.0", want: 0},
	}
	for _, tt := range tests {
		got, err := CompareVersions(tt.a, tt.b)
		if err != nil {
			t.Fatalf("CompareVersions(%q, %q) error: %v", tt.a, tt.b, err)
		}
		if got != tt.want {
			t.Fatalf("CompareVersions(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}
