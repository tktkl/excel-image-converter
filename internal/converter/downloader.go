package converter

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path"
	"strings"
)

var errUnsupportedImageType = errors.New("unsupported image type")

type downloadedImage struct {
	Bytes     []byte
	Extension string
}

func downloadImage(opts Options, rawURL string) (downloadedImage, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return downloadedImage{}, err
	}
	req.Header.Set("User-Agent", opts.UserAgent)
	req.Header.Set("Accept", "image/webp,image/png,image/jpeg,image/gif,image/bmp,image/*;q=0.8,*/*;q=0.5")

	resp, err := opts.HTTPClient.Do(req)
	if err != nil {
		return downloadedImage{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return downloadedImage{}, fmt.Errorf("http status %d", resp.StatusCode)
	}

	limit := opts.MaxImageBytes + 1
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	if err != nil {
		return downloadedImage{}, err
	}
	if int64(len(body)) > opts.MaxImageBytes {
		return downloadedImage{}, fmt.Errorf("image exceeds %d bytes", opts.MaxImageBytes)
	}

	ext := extensionFromContent(resp.Header.Get("Content-Type"), body)
	if ext == "" {
		ext = extensionFromURL(rawURL)
	}
	if ext == "" {
		return downloadedImage{}, errUnsupportedImageType
	}

	return downloadedImage{Bytes: body, Extension: ext}, nil
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
