package converter

import (
	"errors"
	"fmt"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/xuri/excelize/v2"
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

type imageCell struct {
	Sheet string
	Cell  string
	URL   string
}

type downloadedCell struct {
	Target imageCell
	Image  downloadedImage
	Err    error
}

type embeddedCellImage struct {
	Target imageCell
	Image  downloadedImage
}

func ConvertFile(inputPath string, opts Options) (Result, error) {
	opts = opts.withDefaults()
	result := Result{InputPath: inputPath}

	if strings.ToLower(filepath.Ext(inputPath)) != ".xlsx" {
		return result, fmt.Errorf("only .xlsx is supported: %s", inputPath)
	}

	outputPath, err := makeOutputPath(inputPath, opts)
	if err != nil {
		return result, err
	}
	result.OutputPath = outputPath

	f, err := excelize.OpenFile(inputPath)
	if err != nil {
		return result, err
	}
	defer f.Close()

	targets, skippedExistingImages := collectImageCells(f, opts, &result)
	if len(targets) > 0 && opts.Progress != nil {
		opts.Progress(ProgressEvent{Message: fmt.Sprintf("Downloading %d image(s) with %d worker(s)", len(targets), minInt(opts.DownloadWorkers, len(targets)))})
	}
	var embeddedImages []embeddedCellImage
	for item := range downloadImageCells(targets, opts) {
		if embeddedImage, ok := convertDownloadedCell(f, item, opts, &result); ok {
			embeddedImages = append(embeddedImages, embeddedImage)
		}
	}

	if result.Converted == 0 && result.Failed == 0 && len(skippedExistingImages) == 0 {
		return result, fmt.Errorf("no IMAGE(url) formulas or plain image links found")
	}
	if opts.Progress != nil {
		opts.Progress(ProgressEvent{Message: "Saving workbook"})
	}
	if err := disableFormulaViewForImageSheets(f, embeddedImages, skippedExistingImages); err != nil {
		return result, err
	}
	if err := f.SaveAs(outputPath); err != nil {
		return result, err
	}
	if len(embeddedImages) > 0 {
		if opts.Progress != nil {
			opts.Progress(ProgressEvent{Message: "Embedding pictures in cells"})
		}
		if err := writeEmbeddedCellImages(outputPath, embeddedImages, opts.KeepURL, opts.CellImageMode); err != nil {
			return result, err
		}
	}
	if len(skippedExistingImages) > 0 && !opts.KeepURL && opts.CellImageMode == CellImageModeExcel {
		if opts.Progress != nil {
			opts.Progress(ProgressEvent{Message: "Cleaning existing image links"})
		}
		if err := sanitizeExistingCellImageURLs(outputPath, skippedExistingImages); err != nil {
			return result, err
		}
	}
	return result, nil
}

func disableFormulaViewForImageSheets(f *excelize.File, embeddedImages []embeddedCellImage, skippedExistingImages []imageCell) error {
	sheets := make(map[string]struct{})
	for _, image := range embeddedImages {
		sheets[image.Target.Sheet] = struct{}{}
	}
	for _, image := range skippedExistingImages {
		sheets[image.Sheet] = struct{}{}
	}
	showFormulas := false
	for sheet := range sheets {
		if err := f.SetSheetView(sheet, -1, &excelize.ViewOptions{ShowFormulas: &showFormulas}); err != nil {
			return err
		}
	}
	return nil
}

func collectImageCells(f *excelize.File, opts Options, result *Result) ([]imageCell, []imageCell) {
	var targets []imageCell
	var skippedExistingImages []imageCell
	for _, sheet := range f.GetSheetList() {
		rows, err := f.GetRows(sheet, excelize.Options{RawCellValue: true})
		if err != nil {
			addFailure(result, FailureDetail{
				Sheet: sheet,
				Error: err.Error(),
			})
			continue
		}

		maxCols := 0
		for _, row := range rows {
			if len(row) > maxCols {
				maxCols = len(row)
			}
		}
		if maxCols == 0 {
			maxCols = 16384
		}

		for rowIdx := range rows {
			for colIdx := 0; colIdx < maxCols; colIdx++ {
				cell, err := excelize.CoordinatesToCellName(colIdx+1, rowIdx+1)
				if err != nil {
					continue
				}
				url, ok, err := imageURLFromCell(f, sheet, cell)
				if err != nil {
					addFailure(result, FailureDetail{
						Sheet: sheet,
						Cell:  cell,
						Error: err.Error(),
					})
					continue
				}
				if !ok {
					continue
				}
				targets = append(targets, imageCell{Sheet: sheet, Cell: cell, URL: url})
			}
		}
	}
	return targets, skippedExistingImages
}

func imageURLFromCell(f *excelize.File, sheet, cell string) (string, bool, error) {
	formula, err := f.GetCellFormula(sheet, cell)
	if err != nil {
		return "", false, err
	}
	if formula != "" {
		url, err := extractLiteralImageURL(formula)
		if err != nil {
			if errors.Is(err, errNotLiteralImageFormula) {
				return "", false, nil
			}
			return "", false, err
		}
		return url, true, nil
	}

	value, err := f.GetCellValue(sheet, cell, excelize.Options{RawCellValue: true})
	if err != nil {
		return "", false, err
	}
	if url, ok := extractPlainImageURL(value); ok {
		return url, true, nil
	}
	return "", false, nil
}

func downloadImageCells(targets []imageCell, opts Options) <-chan downloadedCell {
	out := make(chan downloadedCell)
	if len(targets) == 0 {
		close(out)
		return out
	}
	workers := minInt(opts.DownloadWorkers, len(targets))
	jobs := make(chan int)
	out = make(chan downloadedCell, workers)

	var wg sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				target := targets[idx]
				item := downloadedCell{Target: target}
				item.Image, item.Err = downloadCellImage(target, opts)
				out <- item
			}
		}()
	}

	go func() {
		for idx := range targets {
			jobs <- idx
		}
		close(jobs)
		wg.Wait()
		close(out)
	}()

	return out
}

func downloadCellImage(target imageCell, opts Options) (downloadedImage, error) {
	if opts.ProgressWriter != nil {
		opts.ProgressWriter.Printf("Downloading %s!%s", target.Sheet, target.Cell)
	}
	if opts.Progress != nil {
		opts.Progress(ProgressEvent{Sheet: target.Sheet, Cell: target.Cell, Message: "Downloading image"})
	}
	return downloadImage(opts, target.URL)
}

func convertDownloadedCell(f *excelize.File, item downloadedCell, opts Options, result *Result) (embeddedCellImage, bool) {
	if item.Err != nil {
		addFailure(result, FailureDetail{
			Sheet: item.Target.Sheet,
			Cell:  item.Target.Cell,
			URL:   item.Target.URL,
			Error: item.Err.Error(),
		})
		return embeddedCellImage{}, false
	}

	if err := setCellLinkVisibility(f, item.Target.Sheet, item.Target.Cell, item.Target.URL, opts.KeepURL); err != nil {
		addFailure(result, FailureDetail{
			Sheet: item.Target.Sheet,
			Cell:  item.Target.Cell,
			URL:   item.Target.URL,
			Error: err.Error(),
		})
		return embeddedCellImage{}, false
	}
	result.Converted++
	return embeddedCellImage{Target: item.Target, Image: item.Image}, true
}

func setCellLinkVisibility(f *excelize.File, sheet, cell, url string, keepURL bool) error {
	if err := f.SetCellFormula(sheet, cell, ""); err != nil {
		return err
	}
	if err := f.SetCellValue(sheet, cell, ""); err != nil {
		return err
	}
	if keepURL {
		tooltip := url
		return f.SetCellHyperLink(sheet, cell, url, "External", excelize.HyperlinkOpts{Tooltip: &tooltip})
	}
	return f.SetCellHyperLink(sheet, cell, "", "None")
}

func addFailure(result *Result, detail FailureDetail) {
	result.Failed++
	result.FailureDetails = append(result.FailureDetails, detail)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func makeOutputPath(inputPath string, opts Options) (string, error) {
	dir := filepath.Dir(inputPath)
	if opts.OutputDir != "" {
		dir = opts.OutputDir
	}
	base := filepath.Base(inputPath)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext) + "_pictures" + ext
	outputPath := filepath.Join(dir, name)

	if !opts.Overwrite {
		if exists(outputPath) {
			for i := 2; ; i++ {
				candidate := filepath.Join(dir, fmt.Sprintf("%s_pictures_%d%s", strings.TrimSuffix(base, ext), i, ext))
				if !exists(candidate) {
					return candidate, nil
				}
			}
		}
	}
	return outputPath, nil
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
