package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wutong/excel-image-converter/internal/buildinfo"
	"github.com/wutong/excel-image-converter/internal/converter"
)

type stdoutProgress struct{}

func (stdoutProgress) Printf(format string, args ...any) {
	fmt.Printf(format+"\n", args...)
}

func main() {
	code := runApp()
	if code != 0 {
		os.Exit(code)
	}
}

func runCLI() int {
	var outDir string
	var keepURL bool
	var compatMode string
	var overwrite bool
	var timeoutSeconds int
	var maxMB int
	var workers int
	var quiet bool
	var showVersion bool

	flags := flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	flags.StringVar(&outDir, "out-dir", "", "directory for converted files")
	flags.BoolVar(&keepURL, "keep-url", false, "keep original URLs as cell hyperlinks after embedding pictures")
	flags.StringVar(&compatMode, "compat", string(converter.DefaultCellImageMode()), "compatibility mode: excel or wps")
	flags.BoolVar(&overwrite, "overwrite", false, "overwrite existing output files")
	flags.IntVar(&timeoutSeconds, "timeout", 30, "download timeout in seconds")
	flags.IntVar(&maxMB, "max-mb", 100, "maximum image size in MB")
	flags.IntVar(&workers, "workers", 4, "number of concurrent image downloads per workbook")
	flags.BoolVar(&quiet, "quiet", false, "suppress per-image progress output")
	flags.BoolVar(&showVersion, "version", false, "print version and exit")

	if err := flags.Parse(os.Args[1:]); err != nil {
		showError("Invalid arguments", err)
		return 2
	}
	if showVersion {
		fmt.Printf("Excel Image Converter v%s\n", buildinfo.DisplayVersion())
		return 0
	}
	cellImageMode, err := converter.ParseCellImageMode(compatMode)
	if err != nil {
		showError("Invalid compatibility mode", err)
		return 2
	}

	files := flags.Args()
	if len(files) == 0 {
		var err error
		files, err = chooseExcelFiles()
		if err != nil {
			showError("Choose files failed", err)
			return 1
		}
		if len(files) == 0 {
			return 0
		}
	}

	opts := converter.Options{
		OutputDir:       outDir,
		KeepURL:         keepURL,
		CellImageMode:   cellImageMode,
		Overwrite:       overwrite,
		DownloadWorkers: workers,
		Timeout:         time.Duration(timeoutSeconds) * time.Second,
		MaxImageBytes:   int64(maxMB) * 1024 * 1024,
	}
	if !quiet {
		opts.ProgressWriter = stdoutProgress{}
	}

	var summaries []string
	var failed int

	for _, file := range files {
		file = strings.Trim(file, `"`)
		result, err := converter.ConvertFile(file, opts)
		if err != nil {
			failed++
			summaries = append(summaries, fmt.Sprintf("FAILED: %s\n  %s", filepath.Base(file), err))
			continue
		}
		summaries = append(summaries, formatResult(result))
		if result.Failed > 0 {
			failed++
		}
	}

	message := strings.Join(summaries, "\n\n")
	if failed > 0 {
		showWarning("Excel Image Converter", message)
		return 1
	}
	showInfo("Excel Image Converter", message)
	return 0
}

func formatResult(result converter.Result) string {
	lines := []string{
		fmt.Sprintf("OK: %s", filepath.Base(result.InputPath)),
		fmt.Sprintf("  Output: %s", result.OutputPath),
		fmt.Sprintf("  Converted: %d", result.Converted),
		fmt.Sprintf("  Failed: %d", result.Failed),
	}
	if len(result.FailureDetails) > 0 {
		lines = append(lines, "  Failure details:")
		for _, detail := range result.FailureDetails {
			where := detail.Sheet
			if detail.Cell != "" {
				where += "!" + detail.Cell
			}
			lines = append(lines, fmt.Sprintf("    %s: %s", where, detail.Error))
		}
	}
	return strings.Join(lines, "\n")
}
