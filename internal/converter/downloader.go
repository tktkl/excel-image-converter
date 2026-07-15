package converter

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

var errUnsupportedImageType = errors.New("unsupported image type")

type downloadedImage struct {
	Bytes     []byte
	Extension string
	Width     int
	Height    int
}

func downloadImage(opts Options, rawURL string) (downloadedImage, error) {
	attempts := opts.DownloadRetries + 1
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		image, err := downloadImageOnce(opts, rawURL)
		if err == nil {
			return image, nil
		}
		lastErr = err
		if attempt == attempts || !isRetryableDownloadError(err) {
			return downloadedImage{}, err
		}
		time.Sleep(retryDelay(opts.RetryBaseDelay, attempt))
	}
	return downloadedImage{}, lastErr
}

func downloadImageOnce(opts Options, rawURL string) (downloadedImage, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return downloadedImage{}, permanentDownloadError{Err: err}
	}
	if err := validateImageURL(req.URL); err != nil {
		return downloadedImage{}, permanentDownloadError{Err: err}
	}
	req.Header.Set("User-Agent", opts.UserAgent)
	req.Header.Set("Accept", "image/webp,image/png,image/jpeg,image/gif,image/bmp,image/*;q=0.8,*/*;q=0.5")

	resp, err := opts.HTTPClient.Do(req)
	if err != nil {
		return downloadedImage{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return downloadedImage{}, httpStatusError{Code: resp.StatusCode}
	}

	limit := opts.MaxImageBytes + 1
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	if err != nil {
		return downloadedImage{}, err
	}
	if int64(len(body)) > opts.MaxImageBytes {
		return downloadedImage{}, imageTooLargeError{MaxBytes: opts.MaxImageBytes}
	}

	ext := extensionFromContent(resp.Header.Get("Content-Type"), body)
	if ext == "" {
		ext = extensionFromURL(rawURL)
	}
	if ext == "" {
		return downloadedImage{}, errUnsupportedImageType
	}

	width, height := imageDimensions(body)
	return downloadedImage{Bytes: body, Extension: ext, Width: width, Height: height}, nil
}

type httpStatusError struct {
	Code int
}

func (e httpStatusError) Error() string {
	return fmt.Sprintf("http status %d", e.Code)
}

type imageTooLargeError struct {
	MaxBytes int64
}

func (e imageTooLargeError) Error() string {
	return fmt.Sprintf("image exceeds %d bytes", e.MaxBytes)
}

type permanentDownloadError struct {
	Err error
}

func (e permanentDownloadError) Error() string {
	return e.Err.Error()
}

func (e permanentDownloadError) Unwrap() error {
	return e.Err
}

func validateImageURL(parsed *url.URL) error {
	if parsed == nil {
		return errors.New("invalid image URL")
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
	default:
		return fmt.Errorf("unsupported image URL scheme %q", parsed.Scheme)
	}
	if parsed.Host == "" {
		return errors.New("image URL must include host")
	}
	return nil
}

func isRetryableDownloadError(err error) bool {
	var permanent permanentDownloadError
	if errors.As(err, &permanent) {
		return false
	}
	var statusErr httpStatusError
	if errors.As(err, &statusErr) {
		return statusErr.Code == http.StatusRequestTimeout ||
			statusErr.Code == http.StatusTooManyRequests ||
			statusErr.Code >= http.StatusInternalServerError
	}
	var tooLarge imageTooLargeError
	if errors.As(err, &tooLarge) {
		return false
	}
	if errors.Is(err, errUnsupportedImageType) {
		return false
	}
	return true
}

func retryDelay(base time.Duration, failedAttempt int) time.Duration {
	if failedAttempt <= 1 {
		return base
	}
	delay := base << (failedAttempt - 1)
	maxDelay := 5 * time.Second
	if delay > maxDelay {
		return maxDelay
	}
	return delay
}

func imageDimensions(body []byte) (int, int) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(body))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		return 0, 0
	}
	return cfg.Width, cfg.Height
}

func extensionFromContent(contentType string, body []byte) string {
	if contentType != "" {
		if mediaType, _, err := mime.ParseMediaType(contentType); err == nil {
			switch strings.ToLower(mediaType) {
			case "image/png":
				return ".png"
			case "image/jpeg", "image/jpg":
				return ".jpg"
			case "image/gif":
				return ".gif"
			case "image/bmp", "image/x-ms-bmp":
				return ".bmp"
			case "image/webp":
				return ".webp"
			}
		}
	}

	if len(body) >= 8 && bytes.Equal(body[:8], []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}) {
		return ".png"
	}
	if len(body) >= 3 && body[0] == 0xff && body[1] == 0xd8 && body[2] == 0xff {
		return ".jpg"
	}
	if len(body) >= 6 && (string(body[:6]) == "GIF87a" || string(body[:6]) == "GIF89a") {
		return ".gif"
	}
	if len(body) >= 2 && string(body[:2]) == "BM" {
		return ".bmp"
	}
	if len(body) >= 12 && string(body[:4]) == "RIFF" && string(body[8:12]) == "WEBP" {
		return ".webp"
	}
	return ""
}

func extensionFromURL(rawURL string) string {
	clean := rawURL
	if idx := strings.IndexAny(clean, "?#"); idx >= 0 {
		clean = clean[:idx]
	}
	switch strings.ToLower(path.Ext(clean)) {
	case ".png":
		return ".png"
	case ".jpg", ".jpeg":
		return ".jpg"
	case ".gif":
		return ".gif"
	case ".bmp":
		return ".bmp"
	case ".webp":
		return ".webp"
	default:
		return ""
	}
}
