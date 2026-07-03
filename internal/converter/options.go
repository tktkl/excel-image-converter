package converter

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/wutong/excel-image-converter/internal/buildinfo"
)

type CellImageMode string

const (
	CellImageModeExcel CellImageMode = "excel"
	CellImageModeWPS   CellImageMode = "wps"
)

func DefaultCellImageMode() CellImageMode {
	return CellImageModeWPS
}

type Options struct {
	OutputDir       string
	KeepURL         bool
	CellImageMode   CellImageMode
	Overwrite       bool
	DownloadWorkers int
	Timeout         time.Duration
	MaxImageBytes   int64
	UserAgent       string
	HTTPClient      *http.Client
	ProgressWriter  ProgressWriter
	Progress        func(ProgressEvent)
}

type ProgressWriter interface {
	Printf(format string, args ...any)
}

type Result struct {
	InputPath      string
	OutputPath     string
	Converted      int
	Skipped        int
	Failed         int
	FailureDetails []FailureDetail
}

type FailureDetail struct {
	Sheet string
	Cell  string
	URL   string
	Error string
}

type ProgressEvent struct {
	Sheet   string
	Cell    string
	Message string
}

func ParseCellImageMode(value string) (CellImageMode, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		return DefaultCellImageMode(), nil
	case string(CellImageModeExcel), "native", "office":
		return CellImageModeExcel, nil
	case string(CellImageModeWPS), "feishu", "lark", "dispimg", "feishu-wps", "wps-feishu":
		return CellImageModeWPS, nil
	default:
		return "", fmt.Errorf("unsupported compatibility mode %q", value)
	}
}

func (m CellImageMode) normalized() CellImageMode {
	mode, err := ParseCellImageMode(string(m))
	if err != nil {
		return DefaultCellImageMode()
	}
	return mode
}

func (o Options) withDefaults() Options {
	o.CellImageMode = o.CellImageMode.normalized()
	if o.DownloadWorkers <= 0 {
		o.DownloadWorkers = 4
	}
	if o.DownloadWorkers > 16 {
		o.DownloadWorkers = 16
	}
	if o.Timeout <= 0 {
		o.Timeout = 30 * time.Second
	}
	if o.MaxImageBytes <= 0 {
		o.MaxImageBytes = 25 * 1024 * 1024
	}
	if o.UserAgent == "" {
		o.UserAgent = fmt.Sprintf("ExcelImageConverter/%s", buildinfo.DisplayVersion())
	}
	if o.HTTPClient == nil {
		o.HTTPClient = &http.Client{Timeout: o.Timeout}
	}
	return o
}
