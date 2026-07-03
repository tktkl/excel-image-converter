package converter

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
)

func TestConvertFileSupportsFormulaAndPlainImageLinks(t *testing.T) {
	input := filepath.Join(t.TempDir(), "links.xlsx")
	f := excelize.NewFile()
	t.Cleanup(func() {
		_ = f.Close()
	})
	plainURL := "https://example.com/plain.png?token=abc"
	formulaURL := "https://example.com/formula.png"
	if err := f.SetCellValue("Sheet1", "A1", plainURL); err != nil {
		t.Fatal(err)
	}
	if err := f.SetCellHyperLink("Sheet1", "A1", plainURL, "External"); err != nil {
		t.Fatal(err)
	}
	if err := f.SetCellFormula("Sheet1", "B1", `=IMAGE("`+formulaURL+`")`); err != nil {
		t.Fatal(err)
	}
	if err := f.SaveAs(input); err != nil {
		t.Fatal(err)
	}

	result, err := ConvertFile(input, Options{CellImageMode: CellImageModeExcel, HTTPClient: testImageHTTPClient(t)})
	if err != nil {
		t.Fatal(err)
	}
	if result.Converted != 2 {
		t.Fatalf("Converted = %d, want 2", result.Converted)
	}

	out, err := excelize.OpenFile(result.OutputPath)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	for _, cell := range []string{"A1", "B1"} {
		pictures, err := out.GetPictures("Sheet1", cell)
		if err != nil {
			t.Fatal(err)
		}
		if len(pictures) != 1 {
			t.Fatalf("%s pictures = %d, want 1", cell, len(pictures))
		}
		if pictures[0].InsertType != excelize.PictureInsertTypeIMAGE {
			t.Fatalf("%s insert type = %v, want IMAGE rich data", cell, pictures[0].InsertType)
		}
		formula, err := out.GetCellFormula("Sheet1", cell)
		if err != nil {
			t.Fatal(err)
		}
		if formula != "" {
			t.Fatalf("%s formula = %q, want empty", cell, formula)
		}
	}
	assertPackageDoesNotContain(t, result.OutputPath, plainURL, formulaURL, "_xmlns")
	assertPackageContains(t, result.OutputPath, sanitizedURLPlaceholder(plainURL))
	assertPackageContains(t, result.OutputPath, sanitizedURLPlaceholder(formulaURL))
	if sanitizedURLPlaceholder(plainURL) == sanitizedURLPlaceholder(formulaURL) {
		t.Fatal("different URLs were sanitized to the same placeholder")
	}
	assertXMLPartsParse(t, result.OutputPath)
	assertPackageDoesNotContainPart(t, result.OutputPath, "xl/drawings/")
	assertNoConverterTempFiles(t, result.OutputPath)
}

func TestConvertFileKeepURL(t *testing.T) {
	input := filepath.Join(t.TempDir(), "keep.xlsx")
	f := excelize.NewFile()
	t.Cleanup(func() {
		_ = f.Close()
	})
	url := "https://example.com/plain.jpg?size=1"
	if err := f.SetCellValue("Sheet1", "A1", url); err != nil {
		t.Fatal(err)
	}
	if err := f.SaveAs(input); err != nil {
		t.Fatal(err)
	}

	result, err := ConvertFile(input, Options{KeepURL: true, CellImageMode: CellImageModeExcel, HTTPClient: testImageHTTPClient(t)})
	if err != nil {
		t.Fatal(err)
	}
	out, err := excelize.OpenFile(result.OutputPath)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	pictures, err := out.GetPictures("Sheet1", "A1")
	if err != nil {
		t.Fatal(err)
	}
	if len(pictures) != 1 {
		t.Fatalf("A1 pictures = %d, want 1", len(pictures))
	}
	if pictures[0].InsertType != excelize.PictureInsertTypeIMAGE {
		t.Fatalf("A1 insert type = %v, want IMAGE rich data", pictures[0].InsertType)
	}
	hasLink, link, err := out.GetCellHyperLink("Sheet1", "A1")
	if err != nil {
		t.Fatal(err)
	}
	if !hasLink || link != url {
		t.Fatalf("A1 hyperlink = (%v, %q), want (%v, %q)", hasLink, link, true, url)
	}
	assertPackageContains(t, result.OutputPath, url)
	assertXMLPartsParse(t, result.OutputPath)
}

func TestConvertFileConvertsExistingImageFormulaCells(t *testing.T) {
	input := filepath.Join(t.TempDir(), "convert-existing-image-formula.xlsx")
	f := excelize.NewFile()
	plainURL := "https://example.com/plain.png"
	existingURL := "https://example.com/already-picture.png"
	if err := f.SetCellValue("Sheet1", "A1", plainURL); err != nil {
		t.Fatal(err)
	}
	if err := f.SetCellValue("Sheet1", "B1", existingURL); err != nil {
		t.Fatal(err)
	}
	if err := f.SaveAs(input); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	existingImage := append(testPNGBytes(t), []byte("existing-picture")...)
	if err := writeEmbeddedCellImages(input, []embeddedCellImage{{
		Target: imageCell{Sheet: "Sheet1", Cell: "B1", URL: existingURL},
		Image:  downloadedImage{Bytes: existingImage, Extension: ".png"},
	}}, false, CellImageModeExcel); err != nil {
		t.Fatal(err)
	}
	injectImageFormulaForTest(t, input, "xl/worksheets/sheet1.xml", "B1", existingURL)

	result, err := ConvertFile(input, Options{CellImageMode: CellImageModeExcel, HTTPClient: testImageHTTPClient(t)})
	if err != nil {
		t.Fatal(err)
	}
	if result.Converted != 2 {
		t.Fatalf("Converted = %d, want 2", result.Converted)
	}
	if result.Skipped != 0 {
		t.Fatalf("Skipped = %d, want 0", result.Skipped)
	}

	out, err := excelize.OpenFile(result.OutputPath)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	for _, cell := range []string{"A1", "B1"} {
		pictures, err := out.GetPictures("Sheet1", cell)
		if err != nil {
			t.Fatal(err)
		}
		if len(pictures) != 1 {
			t.Fatalf("%s pictures = %d, want 1", cell, len(pictures))
		}
	}
	b1Pictures, err := out.GetPictures("Sheet1", "B1")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(b1Pictures[0].File, existingImage) {
		t.Fatalf("B1 IMAGE formula picture was not converted through the normal URL flow")
	}
	formula, err := out.GetCellFormula("Sheet1", "B1")
	if err != nil {
		t.Fatal(err)
	}
	if formula != "" {
		t.Fatalf("B1 formula = %q, want empty", formula)
	}
	assertPackageDoesNotContain(t, result.OutputPath, plainURL, existingURL)
	assertPackageDoesNotContain(t, result.OutputPath, "IMAGE(")
	assertXMLPartsParse(t, result.OutputPath)
}

func TestConvertFileWPSModeUsesDispImage(t *testing.T) {
	input := filepath.Join(t.TempDir(), "wps.xlsx")
	f := excelize.NewFile()
	t.Cleanup(func() {
		_ = f.Close()
	})
	url := "https://example.com/for-wps.png?x=1"
	if err := f.SetCellValue("Sheet1", "A1", url); err != nil {
		t.Fatal(err)
	}
	if err := f.SaveAs(input); err != nil {
		t.Fatal(err)
	}
	addFakeExcelImageCacheParts(t, input)

	result, err := ConvertFile(input, Options{CellImageMode: CellImageModeWPS, HTTPClient: testImageHTTPClient(t)})
	if err != nil {
		t.Fatal(err)
	}
	out, err := excelize.OpenFile(result.OutputPath)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	formula, err := out.GetCellFormula("Sheet1", "A1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(formula, "DISPIMG") {
		t.Fatalf("A1 formula = %q, want DISPIMG", formula)
	}
	pictures, err := out.GetPictures("Sheet1", "A1")
	if err != nil {
		t.Fatal(err)
	}
	if len(pictures) != 1 {
		t.Fatalf("A1 pictures = %d, want 1", len(pictures))
	}
	if pictures[0].InsertType != excelize.PictureInsertTypeDISPIMG {
		t.Fatalf("A1 insert type = %v, want DISPIMG", pictures[0].InsertType)
	}
	assertPackageContainsPart(t, result.OutputPath, "xl/cellimages.xml")
	assertPackageContainsPart(t, result.OutputPath, "xl/_rels/cellimages.xml.rels")
	assertPackageDoesNotContainPart(t, result.OutputPath, "xl/richData/")
	assertPackageDoesNotContainPart(t, result.OutputPath, "xl/metadata.xml")
	assertPackageDoesNotContainPart(t, result.OutputPath, "xl/calcChain.xml")
	assertPackageDoesNotContain(t, result.OutputPath, url, "#VALUE!", "IMAGE(")
	assertPackageDoesNotContain(t, result.OutputPath,
		"_xlfn.DISPIMG", "<v>ID_", `t="str"`,
		"xdr:cNvPicPr", "a:stretch", "xdr:spPr",
		"richData/", "calcChain.xml",
	)
	assertXMLPartsParse(t, result.OutputPath)
	assertNoConverterTempFiles(t, result.OutputPath)
}

func TestConvertFileDisablesFormulaViewForImageSheets(t *testing.T) {
	input := filepath.Join(t.TempDir(), "show-formulas.xlsx")
	f := excelize.NewFile()
	t.Cleanup(func() {
		_ = f.Close()
	})
	showFormulas := true
	if err := f.SetSheetView("Sheet1", 0, &excelize.ViewOptions{ShowFormulas: &showFormulas}); err != nil {
		t.Fatal(err)
	}
	if err := f.SetCellFormula("Sheet1", "A1", `=IMAGE("https://example.com/formula-view.png")`); err != nil {
		t.Fatal(err)
	}
	if err := f.SaveAs(input); err != nil {
		t.Fatal(err)
	}

	result, err := ConvertFile(input, Options{CellImageMode: CellImageModeWPS, HTTPClient: testImageHTTPClient(t)})
	if err != nil {
		t.Fatal(err)
	}
	assertPackageDoesNotContain(t, result.OutputPath, "showFormulas")
	assertPackageContains(t, result.OutputPath, "DISPIMG")
}

func TestDefaultCellImageModeIsWPS(t *testing.T) {
	if got := DefaultCellImageMode(); got != CellImageModeWPS {
		t.Fatalf("DefaultCellImageMode() = %q, want %q", got, CellImageModeWPS)
	}
	if got, err := ParseCellImageMode(""); err != nil || got != CellImageModeWPS {
		t.Fatalf("ParseCellImageMode(\"\") = (%q, %v), want (%q, nil)", got, err, CellImageModeWPS)
	}
}

func TestConvertFileDownloadsConcurrently(t *testing.T) {
	input := filepath.Join(t.TempDir(), "many.xlsx")
	f := excelize.NewFile()
	t.Cleanup(func() {
		_ = f.Close()
	})
	for idx := 1; idx <= 6; idx++ {
		cell, err := excelize.CoordinatesToCellName(idx, 1)
		if err != nil {
			t.Fatal(err)
		}
		if err := f.SetCellValue("Sheet1", cell, "https://example.com/image"+cell+".png"); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.SaveAs(input); err != nil {
		t.Fatal(err)
	}

	client, peak := concurrentTestImageHTTPClient(t)
	result, err := ConvertFile(input, Options{DownloadWorkers: 4, HTTPClient: client})
	if err != nil {
		t.Fatal(err)
	}
	if result.Converted != 6 {
		t.Fatalf("Converted = %d, want 6", result.Converted)
	}
	if got := peak.Load(); got < 2 {
		t.Fatalf("peak concurrent downloads = %d, want at least 2", got)
	}
}

func TestSetWorksheetCellImagesPreservesExtensionNamespaces(t *testing.T) {
	sheetXML := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" xmlns:mc="http://schemas.openxmlformats.org/markup-compatibility/2006" xmlns:x14ac="http://schemas.microsoft.com/office/spreadsheetml/2009/9/ac" mc:Ignorable="x14ac">
  <mc:AlternateContent><mc:Choice Requires="x14ac"></mc:Choice></mc:AlternateContent>
  <sheetData><row r="1" x14ac:dyDescent="0.25"><c r="A1" t="s"><v>0</v></c></row></sheetData>
</worksheet>`)
	updated, err := setWorksheetCellImages(sheetXML, map[string]int{"A1": 1})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`mc:Ignorable="x14ac"`,
		`x14ac:dyDescent="0.25"`,
		`<mc:AlternateContent>`,
		`<c r="A1" t="e" vm="1"><v>#VALUE!</v></c>`,
	} {
		if !bytes.Contains(updated, []byte(want)) {
			t.Fatalf("updated worksheet does not contain %s:\n%s", want, updated)
		}
	}
	if bytes.Contains(updated, []byte("_xmlns")) {
		t.Fatalf("updated worksheet contains _xmlns:\n%s", updated)
	}
}

func testImageHTTPClient(t *testing.T) *http.Client {
	t.Helper()
	png := testPNGBytes(t)
	return &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"image/png"}},
			Body:       io.NopCloser(bytes.NewReader(png)),
			Request:    req,
		}, nil
	})}
}

func concurrentTestImageHTTPClient(t *testing.T) (*http.Client, *atomic.Int32) {
	t.Helper()
	png := testPNGBytes(t)
	var active atomic.Int32
	var peak atomic.Int32
	var once sync.Once
	release := make(chan struct{})

	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		current := active.Add(1)
		defer active.Add(-1)
		for {
			old := peak.Load()
			if current <= old || peak.CompareAndSwap(old, current) {
				break
			}
		}
		once.Do(func() {
			go func() {
				time.Sleep(50 * time.Millisecond)
				close(release)
			}()
		})
		<-release
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"image/png"}},
			Body:       io.NopCloser(bytes.NewReader(png)),
			Request:    req,
		}, nil
	})}

	return client, &peak
}

func testPNGBytes(t *testing.T) []byte {
	t.Helper()
	png, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/p9sAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	return png
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func assertPackageDoesNotContain(t *testing.T, xlsxPath string, values ...string) {
	t.Helper()
	for name, data := range readXMLPartsForTest(t, xlsxPath) {
		for _, value := range values {
			if bytes.Contains(data, []byte(value)) || bytes.Contains(data, []byte(xmlEscapedForTest(value))) {
				t.Fatalf("%s contains %q", name, value)
			}
		}
	}
}

func assertPackageContains(t *testing.T, xlsxPath string, value string) {
	t.Helper()
	for _, data := range readXMLPartsForTest(t, xlsxPath) {
		if bytes.Contains(data, []byte(value)) || bytes.Contains(data, []byte(xmlEscapedForTest(value))) {
			return
		}
	}
	t.Fatalf("package does not contain %q", value)
}

func addFakeExcelImageCacheParts(t *testing.T, xlsxPath string) {
	t.Helper()
	pkg, err := readXLSXPackage(xlsxPath)
	if err != nil {
		t.Fatal(err)
	}
	pkg[metadataPath] = []byte(`<metadata xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"></metadata>`)
	pkg[richValuePath] = []byte(`<rvData xmlns="http://schemas.microsoft.com/office/spreadsheetml/2017/richdata"></rvData>`)
	pkg[richValueStructurePath] = []byte(`<rvStructures xmlns="http://schemas.microsoft.com/office/spreadsheetml/2017/richdata"></rvStructures>`)
	pkg[richValueTypesPath] = []byte(`<rvTypesInfo xmlns="http://schemas.microsoft.com/office/spreadsheetml/2017/richdata2"></rvTypesInfo>`)
	pkg[richValueWebImagePath] = []byte(`<webImagesSrd xmlns="http://schemas.microsoft.com/office/spreadsheetml/2020/richdatawebimage"></webImagesSrd>`)
	pkg[richValueWebImageRels] = []byte(`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"></Relationships>`)
	pkg[calcChainPath] = []byte(`<calcChain xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"></calcChain>`)

	rels, err := readRelationships(pkg, workbookRelsPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		relType string
		target  string
	}{
		{sheetMetadataRelType, "metadata.xml"},
		{richValueRelType, "richData/rdrichvalue.xml"},
		{richValueStructureRel, "richData/rdrichvaluestructure.xml"},
		{richValueTypesRel, "richData/rdRichValueTypes.xml"},
		{richValueWebImageRel, "richData/rdRichValueWebImage.xml"},
		{calcChainRelationship, "calcChain.xml"},
	} {
		rels.Relationships = append(rels.Relationships, relationshipXML{
			ID:     nextRelationshipID(rels),
			Type:   item.relType,
			Target: item.target,
		})
	}
	normalizeRelationships(rels)
	relsBytes, err := xml.Marshal(rels)
	if err != nil {
		t.Fatal(err)
	}
	pkg[workbookRelsPath] = relsBytes

	if err := writeXLSXPackage(xlsxPath, pkg); err != nil {
		t.Fatal(err)
	}
}

func injectImageFormulaForTest(t *testing.T, xlsxPath, sheetPath, cell, url string) {
	t.Helper()
	pkg, err := readXLSXPackage(xlsxPath)
	if err != nil {
		t.Fatal(err)
	}
	data := pkg[sheetPath]
	startMarker := []byte(`<c r="` + cell + `"`)
	start := bytes.Index(data, startMarker)
	if start < 0 {
		t.Fatalf("%s does not contain cell %s", sheetPath, cell)
	}
	endOffset := bytes.Index(data[start:], []byte("</c>"))
	if endOffset < 0 {
		t.Fatalf("%s cell %s is not closed", sheetPath, cell)
	}
	end := start + endOffset + len("</c>")
	cellXML := data[start:end]
	if bytes.Contains(cellXML, []byte("<f>")) {
		t.Fatalf("%s cell %s already contains a formula: %s", sheetPath, cell, cellXML)
	}
	formula := []byte(`<f>_xlfn.IMAGE("` + url + `")</f>`)
	updatedCell := bytes.Replace(cellXML, []byte("<v>#VALUE!</v>"), append(formula, []byte("<v>#VALUE!</v>")...), 1)
	if bytes.Equal(updatedCell, cellXML) {
		t.Fatalf("%s cell %s does not contain a #VALUE! payload: %s", sheetPath, cell, cellXML)
	}
	data = append(append(append([]byte(nil), data[:start]...), updatedCell...), data[end:]...)
	pkg[sheetPath] = data
	if err := writeXLSXPackage(xlsxPath, pkg); err != nil {
		t.Fatal(err)
	}
}

func readXMLPartsForTest(t *testing.T, xlsxPath string) map[string][]byte {
	t.Helper()
	reader, err := zip.OpenReader(xlsxPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()

	parts := map[string][]byte{}
	for _, file := range reader.File {
		ext := filepath.Ext(file.Name)
		if ext != ".xml" && ext != ".rels" {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, readErr := io.ReadAll(rc)
		closeErr := rc.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
		if closeErr != nil {
			t.Fatal(closeErr)
		}
		parts[file.Name] = data
	}
	return parts
}

func assertXMLPartsParse(t *testing.T, xlsxPath string) {
	t.Helper()
	for name, data := range readXMLPartsForTest(t, xlsxPath) {
		decoder := xml.NewDecoder(bytes.NewReader(data))
		for {
			_, err := decoder.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("%s is not valid XML: %v\n%s", name, err, data)
			}
		}
	}
}

func assertPackageDoesNotContainPart(t *testing.T, xlsxPath, partPrefix string) {
	t.Helper()
	reader, err := zip.OpenReader(xlsxPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	for _, file := range reader.File {
		if strings.HasPrefix(file.Name, partPrefix) {
			t.Fatalf("package contains unexpected part %s", file.Name)
		}
	}
}

func assertPackageContainsPart(t *testing.T, xlsxPath, partName string) {
	t.Helper()
	reader, err := zip.OpenReader(xlsxPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	for _, file := range reader.File {
		if file.Name == partName {
			return
		}
	}
	t.Fatalf("package does not contain expected part %s", partName)
}

func xmlEscapedForTest(value string) string {
	var buf bytes.Buffer
	if err := xml.EscapeText(&buf, []byte(value)); err != nil {
		return value
	}
	return buf.String()
}

func assertNoConverterTempFiles(t *testing.T, xlsxPath string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(xlsxPath), ".excel-image-converter-*.xlsx"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) > 0 {
		t.Fatalf("converter temp files were not cleaned: %v", matches)
	}
}
