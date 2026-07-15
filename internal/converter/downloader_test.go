package converter

import (
	"bytes"
	"io"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

func TestDownloadImageRetriesTransientHTTPStatus(t *testing.T) {
	var attempts atomic.Int32
	png := testPNGBytes(t)
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if attempts.Add(1) < 3 {
			return &http.Response{
				StatusCode: http.StatusServiceUnavailable,
				Body:       io.NopCloser(bytes.NewReader(nil)),
				Request:    req,
			}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"image/png"}},
			Body:       io.NopCloser(bytes.NewReader(png)),
			Request:    req,
		}, nil
	})}

	image, err := downloadImage(Options{
		HTTPClient:      client,
		MaxImageBytes:   1024 * 1024,
		UserAgent:       "test",
		DownloadRetries: 3,
		RetryBaseDelay:  time.Nanosecond,
	}, "https://example.com/image.png")
	if err != nil {
		t.Fatal(err)
	}
	if image.Extension != ".png" {
		t.Fatalf("extension = %q, want .png", image.Extension)
	}
	if got := attempts.Load(); got != 3 {
		t.Fatalf("attempts = %d, want 3", got)
	}
}

func TestDownloadImageDoesNotRetryPermanentHTTPStatus(t *testing.T) {
	var attempts atomic.Int32
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		attempts.Add(1)
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(bytes.NewReader(nil)),
			Request:    req,
		}, nil
	})}

	_, err := downloadImage(Options{
		HTTPClient:      client,
		MaxImageBytes:   1024 * 1024,
		UserAgent:       "test",
		DownloadRetries: 3,
		RetryBaseDelay:  time.Nanosecond,
	}, "https://example.com/missing.png")
	if err == nil {
		t.Fatal("expected error")
	}
	if got := attempts.Load(); got != 1 {
		t.Fatalf("attempts = %d, want 1", got)
	}
}

func TestDownloadImageDoesNotRetryUnsupportedURLScheme(t *testing.T) {
	var attempts atomic.Int32
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		attempts.Add(1)
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader(testPNGBytes(t))),
			Request:    req,
		}, nil
	})}

	_, err := downloadImage(Options{
		HTTPClient:      client,
		MaxImageBytes:   1024 * 1024,
		UserAgent:       "test",
		DownloadRetries: 3,
		RetryBaseDelay:  time.Nanosecond,
	}, "ftp://example.com/image.png")
	if err == nil {
		t.Fatal("expected error")
	}
	if got := attempts.Load(); got != 0 {
		t.Fatalf("attempts = %d, want 0", got)
	}
}

func TestOptionsDefaultMaxImageBytes(t *testing.T) {
	opts := Options{}.withDefaults()
	if opts.MaxImageBytes != 100*1024*1024 {
		t.Fatalf("MaxImageBytes = %d, want 100MB", opts.MaxImageBytes)
	}
}
