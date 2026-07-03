package converter

import (
	"archive/zip"
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	contentTypesPath        = "[Content_Types].xml"
	workbookPath            = "xl/workbook.xml"
	workbookRelsPath        = "xl/_rels/workbook.xml.rels"
	cellImagesPath          = "xl/cellimages.xml"
	cellImagesRelsPath      = "xl/_rels/cellimages.xml.rels"
	calcChainPath           = "xl/calcChain.xml"
	metadataPath            = "xl/metadata.xml"
	richValuePath           = "xl/richData/rdrichvalue.xml"
	richValueRelPath        = "xl/richData/richValueRel.xml"
	richValueRelRelsPath    = "xl/richData/_rels/richValueRel.xml.rels"
	richValueStructurePath  = "xl/richData/rdrichvaluestructure.xml"
	richValueTypesPath      = "xl/richData/rdRichValueTypes.xml"
	richValueWebImagePath   = "xl/richData/rdRichValueWebImage.xml"
	richValueWebImageRels   = "xl/richData/_rels/rdRichValueWebImage.xml.rels"
	mainNamespace           = "http://schemas.openxmlformats.org/spreadsheetml/2006/main"
	relationshipsNamespace  = "http://schemas.openxmlformats.org/package/2006/relationships"
	officeRelsNamespace     = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
	drawingMLNamespace      = "http://schemas.openxmlformats.org/drawingml/2006/main"
	spreadsheetDrawingNS    = "http://schemas.openxmlformats.org/drawingml/2006/spreadsheetDrawing"
	wpsCellImageNamespace   = "http://www.wps.cn/officeDocument/2017/etCustomData"
	wpsCellImageRelType     = "http://www.wps.cn/officeDocument/2020/cellImage"
	richDataNamespace       = "http://schemas.microsoft.com/office/spreadsheetml/2017/richdata"
	richDataWebImageNS      = "http://schemas.microsoft.com/office/spreadsheetml/2020/richdatawebimage"
	imageRelationshipType   = officeRelsNamespace + "/image"
	hyperlinkRelationship   = officeRelsNamespace + "/hyperlink"
	calcChainRelationship   = officeRelsNamespace + "/calcChain"
	sheetMetadataRelType    = officeRelsNamespace + "/sheetMetadata"
	richValueRelType        = "http://schemas.microsoft.com/office/2017/06/relationships/rdRichValue"
	richValueStructureRel   = "http://schemas.microsoft.com/office/2017/06/relationships/rdRichValueStructure"
	richValueTypesRel       = "http://schemas.microsoft.com/office/2017/06/relationships/rdRichValueTypes"
	richValueWebImageRel    = "http://schemas.microsoft.com/office/2020/07/relationships/rdRichValueWebImage"
	metadataContentType     = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheetMetadata+xml"
	richValueContentType    = "application/vnd.ms-excel.rdrichvalue+xml"
	richValueRelContentType = "application/vnd.ms-excel.richvaluerel+xml"
	richValueStructureType  = "application/vnd.ms-excel.rdrichvaluestructure+xml"
	richValueTypesType      = "application/vnd.ms-excel.rdrichvaluetypes+xml"
	richValueWebImageType   = "application/vnd.ms-excel.rdrichvaluewebimage+xml"
	richValueBlockURI       = "{3e2802c4-a4d2-4d8b-9148-e3be6c30e623}"
)

var mediaFilePattern = regexp.MustCompile(`^xl/media/image(\d+)\.[A-Za-z0-9]+$`)

type cellKey struct {
	Sheet string
	Cell  string
}

type xmlInner struct {
	XMLName  xml.Name
	Attrs    []xml.Attr `xml:",any,attr"`
	InnerXML string     `xml:",innerxml"`
}

type metadataXML struct {
	XMLName         xml.Name        `xml:"metadata"`
	Attrs           []xml.Attr      `xml:",any,attr"`
	MetadataTypes   *xmlInner       `xml:"metadataTypes"`
	MetadataStrings *xmlInner       `xml:"metadataStrings"`
	MdxMetadata     *xmlInner       `xml:"mdxMetadata"`
	FutureMetadata  []xmlInner      `xml:"futureMetadata"`
	CellMetadata    *xmlInner       `xml:"cellMetadata"`
	ValueMetadata   *metadataBlocks `xml:"valueMetadata"`
	ExtLst          *xmlInner       `xml:"extLst"`
}

type metadataBlocks struct {
	XMLName xml.Name        `xml:"valueMetadata"`
	Attrs   []xml.Attr      `xml:",any,attr"`
	Count   int             `xml:"count,attr,omitempty"`
	Bk      []metadataBlock `xml:"bk"`
}

type metadataBlock struct {
	Rc []metadataRecord `xml:"rc"`
}

type metadataRecord struct {
	T int `xml:"t,attr"`
	V int `xml:"v,attr"`
}

type richValueDataXML struct {
	XMLName xml.Name    `xml:"rvData"`
	Attrs   []xml.Attr  `xml:",any,attr"`
	Count   int         `xml:"count,attr,omitempty"`
	Rv      []richValue `xml:"rv"`
	ExtLst  *xmlInner   `xml:"extLst"`
}

type richValue struct {
	XMLName xml.Name   `xml:"rv"`
	Attrs   []xml.Attr `xml:",any,attr"`
	S       int        `xml:"s,attr"`
	V       []string   `xml:"v"`
	Fb      *xmlInner  `xml:"fb"`
}

type richValueRelsXML struct {
	XMLName xml.Name       `xml:"richValueRels"`
	Attrs   []xml.Attr     `xml:",any,attr"`
	Rels    []richValueRel `xml:"rel"`
	ExtLst  *xmlInner      `xml:"extLst"`
}

type richValueRel struct {
	RID string `xml:"r:id,attr"`
}

type webImagesXML struct {
	XMLName     xml.Name      `xml:"webImagesSrd"`
	Attrs       []xml.Attr    `xml:",any,attr"`
	WebImageSrd []webImageSrd `xml:"webImageSrd"`
	ExtLst      *xmlInner     `xml:"extLst"`
}

type webImageSrd struct {
	Address           relationshipRef  `xml:"address"`
	MoreImagesAddress *relationshipRef `xml:"moreImagesAddress,omitempty"`
	Blip              relationshipRef  `xml:"blip"`
}

type wpsCellImagesXML struct {
	XMLName    xml.Name       `xml:"cellImages"`
	CellImages []wpsCellImage `xml:"cellImage"`
}

type wpsCellImage struct {
	Pic wpsPic `xml:"pic"`
}

type wpsPic struct {
	NvPicPr  wpsNvPicPr  `xml:"nvPicPr"`
	BlipFill wpsBlipFill `xml:"blipFill"`
	SpPr     wpsSpPr     `xml:"spPr"`
}

type wpsNvPicPr struct {
	CNvPr    wpsCNvPr    `xml:"cNvPr"`
	CNvPicPr wpsCNvPicPr `xml:"cNvPicPr"`
}

type wpsCNvPr struct {
	ID    int    `xml:"id,attr"`
	Name  string `xml:"name,attr"`
	Descr string `xml:"descr,attr,omitempty"`
}

type wpsBlipFill struct {
	Blip    wpsBlip    `xml:"blip"`
	Stretch wpsStretch `xml:"stretch"`
}

type wpsBlip struct {
	Embed string `xml:"embed,attr"`
}

type wpsCNvPicPr struct{}

type wpsStretch struct{}

type wpsSpPr struct{}

type wpsCellImagesWriteXML struct {
	XMLName    xml.Name            `xml:"etc:cellImages"`
	Attrs      []xml.Attr          `xml:",any,attr"`
	CellImages []wpsCellImageWrite `xml:"etc:cellImage"`
}

type wpsCellImageWrite struct {
	Pic wpsPicWrite `xml:"xdr:pic"`
}

type wpsPicWrite struct {
	NvPicPr  wpsNvPicPrWrite  `xml:"xdr:nvPicPr"`
	BlipFill wpsBlipFillWrite `xml:"xdr:blipFill"`
	SpPr     wpsSpPrWrite     `xml:"xdr:spPr"`
}

type wpsNvPicPrWrite struct {
	CNvPr    wpsCNvPrWrite    `xml:"xdr:cNvPr"`
	CNvPicPr wpsCNvPicPrWrite `xml:"xdr:cNvPicPr"`
}

type wpsCNvPrWrite struct {
	ID    int    `xml:"id,attr"`
	Name  string `xml:"name,attr"`
	Descr string `xml:"descr,attr,omitempty"`
}

type wpsBlipFillWrite struct {
	Blip    wpsBlipWrite    `xml:"a:blip"`
	Stretch wpsStretchWrite `xml:"a:stretch"`
}

type wpsBlipWrite struct {
	Embed string `xml:"r:embed,attr"`
}

type wpsCNvPicPrWrite struct {
	PicLocks wpsPicLocksWrite `xml:"a:picLocks"`
}

type wpsPicLocksWrite struct {
	NoChangeAspect int `xml:"noChangeAspect,attr"`
}

type wpsStretchWrite struct {
	FillRect wpsFillRectWrite `xml:"a:fillRect"`
}

type wpsFillRectWrite struct{}

type wpsSpPrWrite struct {
	Xfrm     wpsXfrmWrite     `xml:"a:xfrm"`
	PrstGeom wpsPrstGeomWrite `xml:"a:prstGeom"`
}

type wpsXfrmWrite struct {
	Off wpsPointWrite `xml:"a:off"`
	Ext wpsExtWrite   `xml:"a:ext"`
}

type wpsPointWrite struct {
	X int `xml:"x,attr"`
	Y int `xml:"y,attr"`
}

type wpsExtWrite struct {
	CX int `xml:"cx,attr"`
	CY int `xml:"cy,attr"`
}

type wpsPrstGeomWrite struct {
	Prst  string        `xml:"prst,attr"`
	AvLst wpsAvLstWrite `xml:"a:avLst"`
}

type wpsAvLstWrite struct{}

type relationshipRef struct {
	RID string `xml:"r:id,attr"`
}

func (ref *relationshipRef) UnmarshalXML(decoder *xml.Decoder, start xml.StartElement) error {
	for _, attr := range start.Attr {
		if attr.Name.Local == "id" && (attr.Name.Space == officeRelsNamespace || attr.Name.Space == "r" || attr.Name.Space == "") {
			ref.RID = attr.Value
			break
		}
	}
	return skipElement(decoder, start.Name)
}

type relationshipsXML struct {
	XMLName       xml.Name          `xml:"Relationships"`
	Attrs         []xml.Attr        `xml:",any,attr"`
	Relationships []relationshipXML `xml:"Relationship"`
}

type relationshipXML struct {
	ID         string `xml:"Id,attr"`
	Type       string `xml:"Type,attr"`
	Target     string `xml:"Target,attr"`
	TargetMode string `xml:"TargetMode,attr,omitempty"`
}

type contentTypesXML struct {
	XMLName   xml.Name          `xml:"Types"`
	Attrs     []xml.Attr        `xml:",any,attr"`
	Defaults  []contentDefault  `xml:"Default"`
	Overrides []contentOverride `xml:"Override"`
}

type contentDefault struct {
	Extension   string `xml:"Extension,attr"`
	ContentType string `xml:"ContentType,attr"`
}

type contentOverride struct {
	PartName    string `xml:"PartName,attr"`
	ContentType string `xml:"ContentType,attr"`
}

type workbookXML struct {
	Sheets workbookSheets `xml:"sheets"`
}

type workbookSheets struct {
	Sheet []workbookSheet `xml:"sheet"`
}

type workbookSheet struct {
	Name string `xml:"name,attr"`
	RID  string `xml:"http://schemas.openxmlformats.org/officeDocument/2006/relationships id,attr"`
}

type namespacePrefixes map[string]string

func writeEmbeddedCellImages(xlsxPath string, images []embeddedCellImage, keepURL bool, mode CellImageMode) error {
	switch mode.normalized() {
	case CellImageModeWPS:
		return writeDispImageCellImages(xlsxPath, images, keepURL)
	default:
		return writeRichDataCellImages(xlsxPath, images, keepURL)
	}
}

func writeRichDataCellImages(xlsxPath string, images []embeddedCellImage, keepURL bool) error {
	pkg, err := readXLSXPackage(xlsxPath)
	if err != nil {
		return err
	}

	sheetPaths, err := workbookSheetPaths(pkg)
	if err != nil {
		return err
	}

	vmByCell, err := addRichDataCellImages(pkg, images, keepURL)
	if err != nil {
		return err
	}

	patchesBySheet := map[string]map[string]int{}
	for key, vm := range vmByCell {
		sheetPath, ok := sheetPaths[key.Sheet]
		if !ok {
			return fmt.Errorf("sheet %s does not exist", key.Sheet)
		}
		if patchesBySheet[sheetPath] == nil {
			patchesBySheet[sheetPath] = map[string]int{}
		}
		patchesBySheet[sheetPath][key.Cell] = vm
	}
	for sheetPath, patches := range patchesBySheet {
		data, ok := pkg[sheetPath]
		if !ok {
			return fmt.Errorf("worksheet part %s does not exist", sheetPath)
		}
		updated, err := setWorksheetCellImages(data, patches)
		if err != nil {
			return fmt.Errorf("%s: %w", sheetPath, err)
		}
		pkg[sheetPath] = updated
	}

	if !keepURL {
		urls := convertedURLs(images)
		sanitizeURLsInPackage(pkg, urls)
		if err := ensureNoURLResidue(pkg, urls); err != nil {
			return err
		}
	}

	return writeXLSXPackage(xlsxPath, pkg)
}

func writeDispImageCellImages(xlsxPath string, images []embeddedCellImage, keepURL bool) error {
	pkg, err := readXLSXPackage(xlsxPath)
	if err != nil {
		return err
	}
	if err := removeExcelRichDataForDispImages(pkg); err != nil {
		return err
	}

	sheetPaths, err := workbookSheetPaths(pkg)
	if err != nil {
		return err
	}

	idByCell, err := addDispImageCellImages(pkg, images, keepURL)
	if err != nil {
		return err
	}

	patchesBySheet := map[string]map[string]string{}
	for key, imageID := range idByCell {
		sheetPath, ok := sheetPaths[key.Sheet]
		if !ok {
			return fmt.Errorf("sheet %s does not exist", key.Sheet)
		}
		if patchesBySheet[sheetPath] == nil {
			patchesBySheet[sheetPath] = map[string]string{}
		}
		patchesBySheet[sheetPath][key.Cell] = imageID
	}
	for sheetPath, patches := range patchesBySheet {
		data, ok := pkg[sheetPath]
		if !ok {
			return fmt.Errorf("worksheet part %s does not exist", sheetPath)
		}
		updated, err := setWorksheetDispImages(data, patches)
		if err != nil {
			return fmt.Errorf("%s: %w", sheetPath, err)
		}
		pkg[sheetPath] = updated
	}

	if !keepURL {
		urls := convertedURLs(images)
		sanitizeURLsInPackage(pkg, urls)
		if err := ensureNoURLResidue(pkg, urls); err != nil {
			return err
		}
	}

	return writeXLSXPackage(xlsxPath, pkg)
}

func sanitizeExistingCellImageURLs(xlsxPath string, cells []imageCell) error {
	pkg, err := readXLSXPackage(xlsxPath)
	if err != nil {
		return err
	}
	urls := imageCellURLs(cells)
	sanitizeURLsInPackage(pkg, urls)
	if err := ensureNoURLResidue(pkg, urls); err != nil {
		return err
	}
	return writeXLSXPackage(xlsxPath, pkg)
}

func readXLSXPackage(xlsxPath string) (map[string][]byte, error) {
	reader, err := zip.OpenReader(xlsxPath)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	pkg := make(map[string][]byte, len(reader.File))
	for _, file := range reader.File {
		rc, err := file.Open()
		if err != nil {
			return nil, err
		}
		data, readErr := io.ReadAll(rc)
		closeErr := rc.Close()
		if readErr != nil {
			return nil, readErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		pkg[file.Name] = data
	}
	return pkg, nil
}

func writeXLSXPackage(xlsxPath string, pkg map[string][]byte) error {
	dir := filepath.Dir(xlsxPath)
	tmp, err := os.CreateTemp(dir, ".excel-image-converter-*.xlsx")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()

	zw := zip.NewWriter(tmp)
	names := make([]string, 0, len(pkg))
	for name := range pkg {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		writer, err := zw.Create(name)
		if err != nil {
			_ = zw.Close()
			_ = tmp.Close()
			return err
		}
		if _, err := writer.Write(pkg[name]); err != nil {
			_ = zw.Close()
			_ = tmp.Close()
			return err
		}
	}
	if err := zw.Close(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	if err := replaceFileWithBackup(tmpName, xlsxPath); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func replaceFileWithBackup(source, destination string) error {
	if err := os.Rename(source, destination); err == nil {
		return nil
	}

	backup := destination + ".bak"
	for idx := 1; exists(backup); idx++ {
		backup = fmt.Sprintf("%s.bak%d", destination, idx)
	}
	if err := os.Rename(destination, backup); err != nil {
		return err
	}
	if err := os.Rename(source, destination); err != nil {
		_ = os.Rename(backup, destination)
		return err
	}
	return os.Remove(backup)
}

func workbookSheetPaths(pkg map[string][]byte) (map[string]string, error) {
	var workbook workbookXML
	if err := xml.Unmarshal(pkg[workbookPath], &workbook); err != nil {
		return nil, err
	}
	rels, err := readRelationships(pkg, workbookRelsPath)
	if err != nil {
		return nil, err
	}
	byID := map[string]string{}
	for _, rel := range rels.Relationships {
		byID[rel.ID] = resolvePackageTarget("xl/workbook.xml", rel.Target)
	}

	paths := map[string]string{}
	for _, sheet := range workbook.Sheets.Sheet {
		if sheet.RID == "" {
			continue
		}
		if sheetPath, ok := byID[sheet.RID]; ok {
			paths[sheet.Name] = sheetPath
		}
	}
	return paths, nil
}

func addRichDataCellImages(pkg map[string][]byte, images []embeddedCellImage, keepURL bool) (map[cellKey]int, error) {
	metadata, err := readMetadata(pkg)
	if err != nil {
		return nil, err
	}
	richValues, err := readRichValues(pkg)
	if err != nil {
		return nil, err
	}
	webImages, err := readWebImages(pkg)
	if err != nil {
		return nil, err
	}
	webImageRels, err := readRelationships(pkg, richValueWebImageRels)
	if err != nil {
		return nil, err
	}
	ensureRichValueMetadataScaffold(metadata)

	vmByCell := map[cellKey]int{}
	for _, image := range images {
		mediaPath := addMediaToPackage(pkg, image.Image)
		addressID := nextRelationshipID(webImageRels)
		addressTarget := sanitizedURLPlaceholder(image.Target.URL)
		if keepURL {
			addressTarget = image.Target.URL
		}
		webImageRels.Relationships = append(webImageRels.Relationships, relationshipXML{
			ID:         addressID,
			Type:       hyperlinkRelationship,
			Target:     addressTarget,
			TargetMode: "External",
		})
		blipID := nextRelationshipID(webImageRels)
		webImageRels.Relationships = append(webImageRels.Relationships, relationshipXML{
			ID:     blipID,
			Type:   imageRelationshipType,
			Target: "../media/" + path.Base(mediaPath),
		})

		webImageIndex := len(webImages.WebImageSrd)
		webImages.WebImageSrd = append(webImages.WebImageSrd, webImageSrd{
			Address: relationshipRef{RID: addressID},
			Blip:    relationshipRef{RID: blipID},
		})

		richValueIndex := len(richValues.Rv)
		richValues.Rv = append(richValues.Rv, richValue{S: 0, V: []string{strconv.Itoa(webImageIndex), "1", "0", "0"}})
		appendFutureMetadataBlock(metadata, richValueIndex)

		metadata.ValueMetadata.Bk = append(metadata.ValueMetadata.Bk, metadataBlock{
			Rc: []metadataRecord{{T: 1, V: richValueIndex}},
		})
		vmByCell[cellKey{Sheet: image.Target.Sheet, Cell: image.Target.Cell}] = len(metadata.ValueMetadata.Bk)
	}

	metadata.ValueMetadata.Count = len(metadata.ValueMetadata.Bk)
	richValues.Count = len(richValues.Rv)
	normalizeMetadata(metadata)
	normalizeRichValues(richValues)
	normalizeWebImages(webImages)
	normalizeRelationships(webImageRels)

	metadataBytes, err := xml.Marshal(metadata)
	if err != nil {
		return nil, err
	}
	richValueBytes, err := xml.Marshal(richValues)
	if err != nil {
		return nil, err
	}
	webImageBytes, err := xml.Marshal(webImages)
	if err != nil {
		return nil, err
	}
	webImageRelBytes, err := xml.Marshal(webImageRels)
	if err != nil {
		return nil, err
	}

	pkg[metadataPath] = metadataBytes
	pkg[richValuePath] = richValueBytes
	pkg[richValueWebImagePath] = webImageBytes
	pkg[richValueWebImageRels] = webImageRelBytes

	if err := ensureContentTypes(pkg, images); err != nil {
		return nil, err
	}
	if err := ensureWorkbookRichDataRelationships(pkg); err != nil {
		return nil, err
	}
	ensureRichDataSupportParts(pkg)
	return vmByCell, nil
}

func removeExcelRichDataForDispImages(pkg map[string][]byte) error {
	for name := range pkg {
		if strings.HasPrefix(name, "xl/richData/") {
			delete(pkg, name)
		}
	}
	delete(pkg, metadataPath)
	delete(pkg, calcChainPath)

	if err := removeWorkbookRelationships(pkg, map[string]bool{
		sheetMetadataRelType:  true,
		richValueRelType:      true,
		richValueStructureRel: true,
		richValueTypesRel:     true,
		richValueWebImageRel:  true,
		calcChainRelationship: true,
	}); err != nil {
		return err
	}
	return removeContentOverrides(pkg, func(partName string) bool {
		partName = strings.TrimPrefix(partName, "/")
		return partName == metadataPath ||
			partName == calcChainPath ||
			strings.HasPrefix(partName, "xl/richData/")
	})
}

func removeWorkbookRelationships(pkg map[string][]byte, relTypes map[string]bool) error {
	rels, err := readRelationships(pkg, workbookRelsPath)
	if err != nil {
		return err
	}
	filtered := rels.Relationships[:0]
	for _, rel := range rels.Relationships {
		if relTypes[rel.Type] {
			continue
		}
		filtered = append(filtered, rel)
	}
	rels.Relationships = filtered
	normalizeRelationships(rels)
	data, err := xml.Marshal(rels)
	if err != nil {
		return err
	}
	pkg[workbookRelsPath] = data
	return nil
}

func removeContentOverrides(pkg map[string][]byte, remove func(partName string) bool) error {
	data, ok := pkg[contentTypesPath]
	if !ok || len(data) == 0 {
		return nil
	}
	contentTypes := &contentTypesXML{}
	if err := xml.Unmarshal(data, contentTypes); err != nil {
		return err
	}
	filtered := contentTypes.Overrides[:0]
	for _, item := range contentTypes.Overrides {
		if remove(item.PartName) {
			continue
		}
		filtered = append(filtered, item)
	}
	contentTypes.Overrides = filtered
	normalizeContentTypes(contentTypes)
	updated, err := xml.Marshal(contentTypes)
	if err != nil {
		return err
	}
	pkg[contentTypesPath] = updated
	return nil
}

func addDispImageCellImages(pkg map[string][]byte, images []embeddedCellImage, keepURL bool) (map[cellKey]string, error) {
	cellImages, err := readWPSCellImages(pkg)
	if err != nil {
		return nil, err
	}
	rels, err := readRelationships(pkg, cellImagesRelsPath)
	if err != nil {
		return nil, err
	}

	usedIDs := usedDispImageIDs(cellImages)
	nextPictureID := nextWPSPictureID(cellImages)
	idByCell := map[cellKey]string{}

	for idx, image := range images {
		mediaPath := addMediaToPackage(pkg, image.Image)
		relID := nextRelationshipID(rels)
		rels.Relationships = append(rels.Relationships, relationshipXML{
			ID:     relID,
			Type:   imageRelationshipType,
			Target: strings.TrimPrefix(mediaPath, "xl/"),
		})

		imageID := uniqueDispImageID(image, idx, usedIDs)
		pictureID := nextPictureID
		nextPictureID++
		description := fmt.Sprintf("CellImage%d", pictureID)
		if keepURL {
			description = image.Target.URL
		}
		cellImages.CellImages = append(cellImages.CellImages, wpsCellImage{
			Pic: wpsPic{
				NvPicPr: wpsNvPicPr{
					CNvPr: wpsCNvPr{
						ID:    pictureID,
						Name:  imageID,
						Descr: description,
					},
					CNvPicPr: wpsCNvPicPr{},
				},
				BlipFill: wpsBlipFill{
					Blip:    wpsBlip{Embed: relID},
					Stretch: wpsStretch{},
				},
				SpPr: wpsSpPr{},
			},
		})
		idByCell[cellKey{Sheet: image.Target.Sheet, Cell: image.Target.Cell}] = imageID
	}

	normalizeRelationships(rels)
	cellImagesBytes, err := marshalWPSCellImages(cellImages)
	if err != nil {
		return nil, err
	}
	relsBytes, err := xml.Marshal(rels)
	if err != nil {
		return nil, err
	}
	pkg[cellImagesPath] = cellImagesBytes
	pkg[cellImagesRelsPath] = relsBytes
	if err := ensureBasicContentTypes(pkg, images); err != nil {
		return nil, err
	}
	if err := ensureWorkbookDispImageRelationship(pkg); err != nil {
		return nil, err
	}
	return idByCell, nil
}

func readWPSCellImages(pkg map[string][]byte) (*wpsCellImagesXML, error) {
	cellImages := &wpsCellImagesXML{XMLName: xml.Name{Local: "cellImages"}}
	if data, ok := pkg[cellImagesPath]; ok && len(data) > 0 {
		if err := xml.Unmarshal(data, cellImages); err != nil {
			return nil, err
		}
	}
	if cellImages.XMLName.Local == "" {
		cellImages.XMLName = xml.Name{Local: "cellImages"}
	}
	return cellImages, nil
}

func marshalWPSCellImages(cellImages *wpsCellImagesXML) ([]byte, error) {
	out := wpsCellImagesWriteXML{
		Attrs: []xml.Attr{
			{Name: xml.Name{Local: "xmlns:etc"}, Value: wpsCellImageNamespace},
			{Name: xml.Name{Local: "xmlns:xdr"}, Value: spreadsheetDrawingNS},
			{Name: xml.Name{Local: "xmlns:a"}, Value: drawingMLNamespace},
			{Name: xml.Name{Local: "xmlns:r"}, Value: officeRelsNamespace},
		},
		CellImages: make([]wpsCellImageWrite, 0, len(cellImages.CellImages)),
	}
	for _, image := range cellImages.CellImages {
		out.CellImages = append(out.CellImages, wpsCellImageWrite{
			Pic: wpsPicWrite{
				NvPicPr: wpsNvPicPrWrite{
					CNvPr: wpsCNvPrWrite{
						ID:    image.Pic.NvPicPr.CNvPr.ID,
						Name:  image.Pic.NvPicPr.CNvPr.Name,
						Descr: image.Pic.NvPicPr.CNvPr.Descr,
					},
					CNvPicPr: wpsCNvPicPrWrite{
						PicLocks: wpsPicLocksWrite{NoChangeAspect: 1},
					},
				},
				BlipFill: wpsBlipFillWrite{
					Blip:    wpsBlipWrite{Embed: image.Pic.BlipFill.Blip.Embed},
					Stretch: wpsStretchWrite{FillRect: wpsFillRectWrite{}},
				},
				SpPr: wpsSpPrWrite{
					Xfrm: wpsXfrmWrite{
						Off: wpsPointWrite{X: 3939540, Y: 914400},
						Ext: wpsExtWrite{CX: 1142365, CY: 198120},
					},
					PrstGeom: wpsPrstGeomWrite{
						Prst:  "rect",
						AvLst: wpsAvLstWrite{},
					},
				},
			},
		})
	}
	return xml.Marshal(out)
}

func usedDispImageIDs(cellImages *wpsCellImagesXML) map[string]bool {
	used := map[string]bool{}
	for _, image := range cellImages.CellImages {
		name := image.Pic.NvPicPr.CNvPr.Name
		if name != "" {
			used[name] = true
		}
	}
	return used
}

func uniqueDispImageID(image embeddedCellImage, index int, used map[string]bool) string {
	for attempt := 0; ; attempt++ {
		hash := sha1.New()
		_, _ = hash.Write([]byte(image.Target.Sheet))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write([]byte(image.Target.Cell))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write([]byte(image.Target.URL))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write([]byte(strconv.Itoa(index)))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write([]byte(strconv.Itoa(attempt)))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write(image.Image.Bytes)
		sum := hash.Sum(nil)
		id := "ID_" + strings.ToUpper(hex.EncodeToString(sum)[:32])
		if !used[id] {
			used[id] = true
			return id
		}
	}
}

func nextWPSPictureID(cellImages *wpsCellImagesXML) int {
	maxID := 0
	for _, image := range cellImages.CellImages {
		if image.Pic.NvPicPr.CNvPr.ID > maxID {
			maxID = image.Pic.NvPicPr.CNvPr.ID
		}
	}
	return maxID + 1
}

func readMetadata(pkg map[string][]byte) (*metadataXML, error) {
	metadata := &metadataXML{
		XMLName: xml.Name{Local: "metadata"},
		Attrs:   []xml.Attr{{Name: xml.Name{Local: "xmlns"}, Value: mainNamespace}},
		ValueMetadata: &metadataBlocks{
			XMLName: xml.Name{Local: "valueMetadata"},
		},
	}
	if data, ok := pkg[metadataPath]; ok && len(data) > 0 {
		if err := xml.Unmarshal(data, metadata); err != nil {
			return nil, err
		}
	}
	if metadata.XMLName.Local == "" {
		metadata.XMLName = xml.Name{Local: "metadata"}
	}
	if metadata.ValueMetadata == nil {
		metadata.ValueMetadata = &metadataBlocks{XMLName: xml.Name{Local: "valueMetadata"}}
	}
	if metadata.ValueMetadata.XMLName.Local == "" {
		metadata.ValueMetadata.XMLName = xml.Name{Local: "valueMetadata"}
	}
	ensureDefaultNamespace(&metadata.Attrs, mainNamespace)
	return metadata, nil
}

func readRichValues(pkg map[string][]byte) (*richValueDataXML, error) {
	richValues := &richValueDataXML{XMLName: xml.Name{Local: "rvData"}}
	if data, ok := pkg[richValuePath]; ok && len(data) > 0 {
		if err := xml.Unmarshal(data, richValues); err != nil {
			return nil, err
		}
	}
	if richValues.XMLName.Local == "" {
		richValues.XMLName = xml.Name{Local: "rvData"}
	}
	return richValues, nil
}

func readRichValueRels(pkg map[string][]byte) (*richValueRelsXML, error) {
	rels := &richValueRelsXML{XMLName: xml.Name{Local: "richValueRels"}}
	if data, ok := pkg[richValueRelPath]; ok && len(data) > 0 {
		if err := xml.Unmarshal(data, rels); err != nil {
			return nil, err
		}
	}
	if rels.XMLName.Local == "" {
		rels.XMLName = xml.Name{Local: "richValueRels"}
	}
	ensureRichRelationshipNamespace(rels)
	return rels, nil
}

func readWebImages(pkg map[string][]byte) (*webImagesXML, error) {
	webImages := &webImagesXML{
		XMLName: xml.Name{Local: "webImagesSrd"},
		Attrs: []xml.Attr{
			{Name: xml.Name{Local: "xmlns"}, Value: richDataWebImageNS},
			{Name: xml.Name{Local: "xmlns:r"}, Value: officeRelsNamespace},
		},
	}
	if data, ok := pkg[richValueWebImagePath]; ok && len(data) > 0 {
		if err := xml.Unmarshal(data, webImages); err != nil {
			return nil, err
		}
	}
	if webImages.XMLName.Local == "" {
		webImages.XMLName = xml.Name{Local: "webImagesSrd"}
	}
	normalizeWebImages(webImages)
	return webImages, nil
}

func readRelationships(pkg map[string][]byte, relsPath string) (*relationshipsXML, error) {
	rels := &relationshipsXML{
		XMLName: xml.Name{Local: "Relationships"},
		Attrs:   []xml.Attr{{Name: xml.Name{Local: "xmlns"}, Value: relationshipsNamespace}},
	}
	if data, ok := pkg[relsPath]; ok && len(data) > 0 {
		if err := xml.Unmarshal(data, rels); err != nil {
			return nil, err
		}
	}
	if rels.XMLName.Local == "" {
		rels.XMLName = xml.Name{Local: "Relationships"}
	}
	ensureDefaultNamespace(&rels.Attrs, relationshipsNamespace)
	return rels, nil
}

func ensureContentTypes(pkg map[string][]byte, images []embeddedCellImage) error {
	contentTypes := &contentTypesXML{
		XMLName: xml.Name{Local: "Types"},
		Attrs:   []xml.Attr{{Name: xml.Name{Local: "xmlns"}, Value: "http://schemas.openxmlformats.org/package/2006/content-types"}},
	}
	if data, ok := pkg[contentTypesPath]; ok && len(data) > 0 {
		if err := xml.Unmarshal(data, contentTypes); err != nil {
			return err
		}
	}
	if contentTypes.XMLName.Local == "" {
		contentTypes.XMLName = xml.Name{Local: "Types"}
	}
	ensureDefaultNamespace(&contentTypes.Attrs, "http://schemas.openxmlformats.org/package/2006/content-types")
	for _, image := range images {
		ensureContentDefault(contentTypes, strings.TrimPrefix(strings.ToLower(image.Image.Extension), "."), imageContentType(image.Image.Extension))
	}
	ensureContentDefault(contentTypes, "rels", "application/vnd.openxmlformats-package.relationships+xml")
	ensureContentDefault(contentTypes, "xml", "application/xml")
	ensureContentOverride(contentTypes, "/"+metadataPath, metadataContentType)
	ensureContentOverride(contentTypes, "/"+richValuePath, richValueContentType)
	ensureContentOverride(contentTypes, "/"+richValueStructurePath, richValueStructureType)
	ensureContentOverride(contentTypes, "/"+richValueTypesPath, richValueTypesType)
	ensureContentOverride(contentTypes, "/"+richValueWebImagePath, richValueWebImageType)
	normalizeContentTypes(contentTypes)

	data, err := xml.Marshal(contentTypes)
	if err != nil {
		return err
	}
	pkg[contentTypesPath] = data
	return nil
}

func ensureBasicContentTypes(pkg map[string][]byte, images []embeddedCellImage) error {
	contentTypes := &contentTypesXML{
		XMLName: xml.Name{Local: "Types"},
		Attrs:   []xml.Attr{{Name: xml.Name{Local: "xmlns"}, Value: "http://schemas.openxmlformats.org/package/2006/content-types"}},
	}
	if data, ok := pkg[contentTypesPath]; ok && len(data) > 0 {
		if err := xml.Unmarshal(data, contentTypes); err != nil {
			return err
		}
	}
	if contentTypes.XMLName.Local == "" {
		contentTypes.XMLName = xml.Name{Local: "Types"}
	}
	ensureDefaultNamespace(&contentTypes.Attrs, "http://schemas.openxmlformats.org/package/2006/content-types")
	for _, image := range images {
		ensureContentDefault(contentTypes, strings.TrimPrefix(strings.ToLower(image.Image.Extension), "."), imageContentType(image.Image.Extension))
	}
	ensureContentDefault(contentTypes, "rels", "application/vnd.openxmlformats-package.relationships+xml")
	ensureContentDefault(contentTypes, "xml", "application/xml")
	normalizeContentTypes(contentTypes)

	data, err := xml.Marshal(contentTypes)
	if err != nil {
		return err
	}
	pkg[contentTypesPath] = data
	return nil
}

func ensureWorkbookRichDataRelationships(pkg map[string][]byte) error {
	rels, err := readRelationships(pkg, workbookRelsPath)
	if err != nil {
		return err
	}
	ensureWorkbookRelationship(rels, sheetMetadataRelType, "metadata.xml", metadataPath)
	ensureWorkbookRelationship(rels, richValueRelType, "richData/rdrichvalue.xml", richValuePath)
	ensureWorkbookRelationship(rels, richValueStructureRel, "richData/rdrichvaluestructure.xml", richValueStructurePath)
	ensureWorkbookRelationship(rels, richValueTypesRel, "richData/rdRichValueTypes.xml", richValueTypesPath)
	ensureWorkbookRelationship(rels, richValueWebImageRel, "richData/rdRichValueWebImage.xml", richValueWebImagePath)
	normalizeRelationships(rels)
	data, err := xml.Marshal(rels)
	if err != nil {
		return err
	}
	pkg[workbookRelsPath] = data
	return nil
}

func ensureWorkbookDispImageRelationship(pkg map[string][]byte) error {
	rels, err := readRelationships(pkg, workbookRelsPath)
	if err != nil {
		return err
	}
	ensureWorkbookRelationship(rels, wpsCellImageRelType, "cellimages.xml", cellImagesPath)
	normalizeRelationships(rels)
	data, err := xml.Marshal(rels)
	if err != nil {
		return err
	}
	pkg[workbookRelsPath] = data
	return nil
}

func ensureWorkbookRelationship(rels *relationshipsXML, relType, target, packagePath string) {
	for _, rel := range rels.Relationships {
		if rel.Type == relType && resolvePackageTarget(workbookPath, rel.Target) == packagePath {
			return
		}
	}
	rels.Relationships = append(rels.Relationships, relationshipXML{
		ID:     nextRelationshipID(rels),
		Type:   relType,
		Target: target,
	})
}

func ensureRichDataSupportParts(pkg map[string][]byte) {
	if _, ok := pkg[richValueStructurePath]; !ok {
		pkg[richValueStructurePath] = []byte(`<rvStructures xmlns="http://schemas.microsoft.com/office/spreadsheetml/2017/richdata" count="1"><s t="_webimage"><k n="WebImageIdentifier" t="i"/><k n="CalcOrigin" t="i"/><k n="ComputedImage" t="b"/><k n="ImageSizing" t="i"/></s></rvStructures>`)
	}
	if _, ok := pkg[richValueTypesPath]; !ok {
		pkg[richValueTypesPath] = []byte(`<rvTypesInfo xmlns="http://schemas.microsoft.com/office/spreadsheetml/2017/richdata2" xmlns:mc="http://schemas.openxmlformats.org/markup-compatibility/2006" mc:Ignorable="x" xmlns:x="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><global><keyFlags><key name="_Self"><flag name="ExcludeFromFile" value="1"/><flag name="ExcludeFromCalcComparison" value="1"/></key><key name="_DisplayString"><flag name="ExcludeFromCalcComparison" value="1"/></key><key name="_Flags"><flag name="ExcludeFromCalcComparison" value="1"/></key><key name="_Format"><flag name="ExcludeFromCalcComparison" value="1"/></key><key name="_SubLabel"><flag name="ExcludeFromCalcComparison" value="1"/></key><key name="_Attribution"><flag name="ExcludeFromCalcComparison" value="1"/></key><key name="_Icon"><flag name="ExcludeFromCalcComparison" value="1"/></key><key name="_Display"><flag name="ExcludeFromCalcComparison" value="1"/></key><key name="_CanonicalPropertyNames"><flag name="ExcludeFromCalcComparison" value="1"/></key><key name="_ClassificationId"><flag name="ExcludeFromCalcComparison" value="1"/></key></keyFlags></global><types><type name="_webimage"><keyFlags><key name="WebImageIdentifier"><flag name="ShowInCardView" value="0"/></key></keyFlags></type></types></rvTypesInfo>`)
	}
}

func ensureContentDefault(types *contentTypesXML, extension, contentType string) {
	if extension == "" || contentType == "" {
		return
	}
	for idx, item := range types.Defaults {
		if strings.EqualFold(item.Extension, extension) {
			types.Defaults[idx].ContentType = contentType
			return
		}
	}
	types.Defaults = append(types.Defaults, contentDefault{Extension: extension, ContentType: contentType})
}

func ensureContentOverride(types *contentTypesXML, partName, contentType string) {
	for idx, item := range types.Overrides {
		if strings.EqualFold(item.PartName, partName) {
			types.Overrides[idx].ContentType = contentType
			return
		}
	}
	types.Overrides = append(types.Overrides, contentOverride{PartName: partName, ContentType: contentType})
}

func imageContentType(ext string) string {
	switch strings.ToLower(strings.TrimPrefix(ext, ".")) {
	case "png":
		return "image/png"
	case "jpg", "jpeg":
		return "image/jpeg"
	case "gif":
		return "image/gif"
	case "bmp":
		return "image/bmp"
	case "webp":
		return "image/webp"
	default:
		return ""
	}
}

func addMediaToPackage(pkg map[string][]byte, image downloadedImage) string {
	for name, existing := range pkg {
		if strings.HasPrefix(name, "xl/media/image") && bytes.Equal(existing, image.Bytes) {
			return name
		}
	}
	next := nextMediaIndex(pkg)
	ext := strings.ToLower(image.Extension)
	for {
		mediaPath := fmt.Sprintf("xl/media/image%d%s", next, ext)
		if _, ok := pkg[mediaPath]; !ok {
			pkg[mediaPath] = append([]byte(nil), image.Bytes...)
			return mediaPath
		}
		next++
	}
}

func nextMediaIndex(pkg map[string][]byte) int {
	maxIndex := 0
	for name := range pkg {
		match := mediaFilePattern.FindStringSubmatch(name)
		if len(match) != 2 {
			continue
		}
		idx, err := strconv.Atoi(match[1])
		if err == nil && idx > maxIndex {
			maxIndex = idx
		}
	}
	return maxIndex + 1
}

func nextRelationshipID(rels *relationshipsXML) string {
	maxID := 0
	for _, rel := range rels.Relationships {
		if !strings.HasPrefix(rel.ID, "rId") {
			continue
		}
		idx, err := strconv.Atoi(strings.TrimPrefix(rel.ID, "rId"))
		if err == nil && idx > maxID {
			maxID = idx
		}
	}
	return "rId" + strconv.Itoa(maxID+1)
}

func ensureDefaultNamespace(attrs *[]xml.Attr, namespace string) {
	for _, attr := range *attrs {
		if attr.Name.Space == "" && attr.Name.Local == "xmlns" {
			return
		}
	}
	*attrs = append([]xml.Attr{{Name: xml.Name{Local: "xmlns"}, Value: namespace}}, *attrs...)
}

func ensureRichRelationshipNamespace(rels *richValueRelsXML) {
	for _, attr := range rels.Attrs {
		if (attr.Name.Space == "" && attr.Name.Local == "xmlns:r") || (attr.Name.Space == "xmlns" && attr.Name.Local == "r") {
			return
		}
	}
	rels.Attrs = append([]xml.Attr{{Name: xml.Name{Local: "xmlns:r"}, Value: officeRelsNamespace}}, rels.Attrs...)
}

func ensureRichValueMetadataScaffold(metadata *metadataXML) {
	ensureDefaultNamespace(&metadata.Attrs, mainNamespace)
	ensurePrefixedNamespace(&metadata.Attrs, "xlrd", richDataNamespace)
	if metadata.MetadataTypes == nil {
		metadata.MetadataTypes = &xmlInner{
			XMLName:  xml.Name{Local: "metadataTypes"},
			Attrs:    []xml.Attr{{Name: xml.Name{Local: "count"}, Value: "1"}},
			InnerXML: `<metadataType name="XLRICHVALUE" minSupportedVersion="120000" copy="1" pasteAll="1" pasteValues="1" merge="1" splitFirst="1" rowColShift="1" clearFormats="1" clearComments="1" assign="1" coerce="1"/>`,
		}
	}
	if findRichValueFutureMetadata(metadata) == nil {
		metadata.FutureMetadata = append(metadata.FutureMetadata, xmlInner{
			XMLName: xml.Name{Local: "futureMetadata"},
			Attrs: []xml.Attr{
				{Name: xml.Name{Local: "name"}, Value: "XLRICHVALUE"},
				{Name: xml.Name{Local: "count"}, Value: "0"},
			},
		})
	}
}

func appendFutureMetadataBlock(metadata *metadataXML, richValueIndex int) {
	future := findRichValueFutureMetadata(metadata)
	if future == nil {
		ensureRichValueMetadataScaffold(metadata)
		future = findRichValueFutureMetadata(metadata)
	}
	if future == nil {
		return
	}
	count := attrInt(future.Attrs, "count")
	count++
	setAttr(&future.Attrs, "count", strconv.Itoa(count))
	future.InnerXML += fmt.Sprintf(`<bk><extLst><ext uri="%s"><xlrd:rvb i="%d"/></ext></extLst></bk>`, richValueBlockURI, richValueIndex)
}

func findRichValueFutureMetadata(metadata *metadataXML) *xmlInner {
	for idx := range metadata.FutureMetadata {
		if attrValue(metadata.FutureMetadata[idx].Attrs, "name") == "XLRICHVALUE" {
			return &metadata.FutureMetadata[idx]
		}
	}
	return nil
}

func normalizeMetadata(metadata *metadataXML) {
	normalizeRoot(&metadata.XMLName, &metadata.Attrs, mainNamespace)
	ensurePrefixedNamespace(&metadata.Attrs, "xlrd", richDataNamespace)
	normalizeXMLInner(metadata.MetadataTypes)
	normalizeXMLInner(metadata.MetadataStrings)
	normalizeXMLInner(metadata.MdxMetadata)
	for idx := range metadata.FutureMetadata {
		normalizeXMLInner(&metadata.FutureMetadata[idx])
	}
	normalizeXMLInner(metadata.CellMetadata)
	if metadata.ValueMetadata != nil {
		metadata.ValueMetadata.XMLName.Space = ""
		normalizeNamespaceAttrs(&metadata.ValueMetadata.Attrs)
	}
	normalizeXMLInner(metadata.ExtLst)
}

func normalizeRichValues(richValues *richValueDataXML) {
	normalizeRoot(&richValues.XMLName, &richValues.Attrs, richDataNamespace)
	for idx := range richValues.Rv {
		richValues.Rv[idx].XMLName.Space = ""
		normalizeNamespaceAttrs(&richValues.Rv[idx].Attrs)
		normalizeXMLInner(richValues.Rv[idx].Fb)
	}
	normalizeXMLInner(richValues.ExtLst)
}

func normalizeWebImages(webImages *webImagesXML) {
	normalizeRoot(&webImages.XMLName, &webImages.Attrs, richDataWebImageNS)
	ensurePrefixedNamespace(&webImages.Attrs, "r", officeRelsNamespace)
	normalizeXMLInner(webImages.ExtLst)
}

func normalizeRelationships(rels *relationshipsXML) {
	normalizeRoot(&rels.XMLName, &rels.Attrs, relationshipsNamespace)
}

func normalizeContentTypes(types *contentTypesXML) {
	normalizeRoot(&types.XMLName, &types.Attrs, "http://schemas.openxmlformats.org/package/2006/content-types")
}

func normalizeXMLInner(inner *xmlInner) {
	if inner == nil {
		return
	}
	inner.XMLName.Space = ""
	normalizeNamespaceAttrs(&inner.Attrs)
}

func normalizeRoot(name *xml.Name, attrs *[]xml.Attr, defaultNamespace string) {
	if name.Space != "" {
		ensureDefaultNamespace(attrs, name.Space)
		name.Space = ""
	}
	ensureDefaultNamespace(attrs, defaultNamespace)
	normalizeNamespaceAttrs(attrs)
}

func normalizeNamespaceAttrs(attrs *[]xml.Attr) {
	normalized := make([]xml.Attr, 0, len(*attrs))
	for _, attr := range *attrs {
		attr = normalizeNamespaceAttr(attr)
		if isDuplicateAttr(normalized, attr) {
			continue
		}
		normalized = append(normalized, attr)
	}
	*attrs = normalized
}

func normalizeNamespaceAttr(attr xml.Attr) xml.Attr {
	switch {
	case attr.Name.Space == "xmlns" && attr.Name.Local != "":
		attr.Name.Space = ""
		attr.Name.Local = "xmlns:" + attr.Name.Local
	case attr.Name.Space != "":
		if prefix, ok := knownNamespacePrefixes()[attr.Name.Space]; ok && prefix != "" {
			attr.Name.Space = ""
			attr.Name.Local = prefix + ":" + attr.Name.Local
		}
	}
	return attr
}

func knownNamespacePrefixes() map[string]string {
	return map[string]string{
		officeRelsNamespace: "r",
		richDataNamespace:   "xlrd",
		"http://schemas.openxmlformats.org/markup-compatibility/2006":      "mc",
		"http://schemas.microsoft.com/office/spreadsheetml/2009/9/ac":      "x14ac",
		"http://schemas.microsoft.com/office/spreadsheetml/2014/revision":  "xr",
		"http://schemas.microsoft.com/office/spreadsheetml/2015/revision2": "xr2",
		"http://schemas.microsoft.com/office/spreadsheetml/2016/revision3": "xr3",
	}
}

func ensurePrefixedNamespace(attrs *[]xml.Attr, prefix, namespace string) {
	local := "xmlns:" + prefix
	for _, attr := range *attrs {
		if (attr.Name.Space == "" && attr.Name.Local == local) || (attr.Name.Space == "xmlns" && attr.Name.Local == prefix) {
			return
		}
	}
	*attrs = append(*attrs, xml.Attr{Name: xml.Name{Local: local}, Value: namespace})
}

func attrInt(attrs []xml.Attr, local string) int {
	value := attrValue(attrs, local)
	parsed, _ := strconv.Atoi(value)
	return parsed
}

func setAttr(attrs *[]xml.Attr, local, value string) {
	for idx := range *attrs {
		if (*attrs)[idx].Name.Local == local {
			(*attrs)[idx].Value = value
			return
		}
	}
	*attrs = append(*attrs, xml.Attr{Name: xml.Name{Local: local}, Value: value})
}

func normalizeStartElement(start xml.StartElement, prefixes namespacePrefixes) xml.StartElement {
	prefixes.learn(start.Attr)
	start.Name = prefixes.normalizeName(start.Name)
	attrs := make([]xml.Attr, 0, len(start.Attr))
	for _, attr := range start.Attr {
		normalized := prefixes.normalizeAttr(attr)
		if isDuplicateAttr(attrs, normalized) {
			continue
		}
		attrs = append(attrs, normalized)
	}
	start.Attr = attrs
	return start
}

func normalizeEndElement(end xml.EndElement, prefixes namespacePrefixes) xml.EndElement {
	end.Name = prefixes.normalizeName(end.Name)
	return end
}

func (prefixes namespacePrefixes) learn(attrs []xml.Attr) {
	for _, attr := range attrs {
		switch {
		case attr.Name.Space == "xmlns" && attr.Name.Local != "":
			prefixes[attr.Value] = attr.Name.Local
		case attr.Name.Space == "" && attr.Name.Local == "xmlns":
			prefixes[attr.Value] = ""
		}
	}
}

func (prefixes namespacePrefixes) normalizeName(name xml.Name) xml.Name {
	if name.Space == "" {
		return name
	}
	if prefix, ok := prefixes[name.Space]; ok {
		name.Space = ""
		if prefix != "" {
			name.Local = prefix + ":" + name.Local
		}
		return name
	}
	name.Space = ""
	return name
}

func (prefixes namespacePrefixes) normalizeAttr(attr xml.Attr) xml.Attr {
	switch {
	case attr.Name.Space == "xmlns" && attr.Name.Local != "":
		attr.Name.Space = ""
		attr.Name.Local = "xmlns:" + attr.Name.Local
	case attr.Name.Space != "":
		attr.Name = prefixes.normalizeName(attr.Name)
	default:
		attr.Name.Space = ""
	}
	return attr
}

func isDuplicateAttr(attrs []xml.Attr, attr xml.Attr) bool {
	for _, existing := range attrs {
		if existing.Name.Space == attr.Name.Space && existing.Name.Local == attr.Name.Local {
			return true
		}
	}
	return false
}

func resolvePackageTarget(sourcePart, target string) string {
	if strings.HasPrefix(target, "/") {
		return strings.TrimPrefix(path.Clean(target), "/")
	}
	if strings.HasPrefix(target, "xl/") {
		return path.Clean(target)
	}
	return path.Clean(path.Join(path.Dir(sourcePart), target))
}

func setWorksheetCellImages(sheetXML []byte, vmByCell map[string]int) ([]byte, error) {
	remaining := map[string]int{}
	targetRows := map[int][]string{}
	for cell, vm := range vmByCell {
		remaining[cell] = vm
		row, err := rowFromCell(cell)
		if err != nil {
			return nil, err
		}
		targetRows[row] = append(targetRows[row], cell)
	}
	for row := range targetRows {
		sortCells(targetRows[row])
	}

	decoder := xml.NewDecoder(bytes.NewReader(sheetXML))
	var out bytes.Buffer
	encoder := xml.NewEncoder(&out)
	prefixes := namespacePrefixes{
		mainNamespace:       "",
		officeRelsNamespace: "r",
	}
	inSheetData := false
	currentRow := 0
	seenRows := map[int]bool{}

	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		switch typed := token.(type) {
		case xml.StartElement:
			switch typed.Name.Local {
			case "sheetData":
				inSheetData = true
			case "row":
				currentRow = rowNumberFromAttrs(typed.Attr)
				if currentRow > 0 {
					seenRows[currentRow] = true
				}
			case "c":
				cell := attrValue(typed.Attr, "r")
				if vm, ok := remaining[cell]; ok {
					if err := writeCellImageElement(encoder, typed, cell, vm, prefixes); err != nil {
						return nil, err
					}
					if err := skipElement(decoder, typed.Name); err != nil {
						return nil, err
					}
					delete(remaining, cell)
					continue
				}
			}
			if err := encoder.EncodeToken(normalizeStartElement(typed, prefixes)); err != nil {
				return nil, err
			}
		case xml.EndElement:
			if typed.Name.Local == "row" && currentRow > 0 {
				for _, cell := range targetRows[currentRow] {
					if vm, ok := remaining[cell]; ok {
						if err := writeNewCellImageElement(encoder, cell, vm); err != nil {
							return nil, err
						}
						delete(remaining, cell)
					}
				}
				currentRow = 0
			}
			if typed.Name.Local == "sheetData" {
				for row, cells := range targetRows {
					if seenRows[row] {
						continue
					}
					if err := writeMissingRow(encoder, row, cells, remaining); err != nil {
						return nil, err
					}
					for _, cell := range cells {
						delete(remaining, cell)
					}
				}
				inSheetData = false
			}
			if err := encoder.EncodeToken(normalizeEndElement(typed, prefixes)); err != nil {
				return nil, err
			}
		default:
			if err := encoder.EncodeToken(token); err != nil {
				return nil, err
			}
		}
	}
	if inSheetData {
		return nil, fmt.Errorf("worksheet sheetData was not closed")
	}
	if len(remaining) > 0 {
		return nil, fmt.Errorf("failed to write %d image cell(s)", len(remaining))
	}
	if err := encoder.Flush(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func setWorksheetDispImages(sheetXML []byte, idByCell map[string]string) ([]byte, error) {
	remaining := map[string]string{}
	targetRows := map[int][]string{}
	for cell, imageID := range idByCell {
		remaining[cell] = imageID
		row, err := rowFromCell(cell)
		if err != nil {
			return nil, err
		}
		targetRows[row] = append(targetRows[row], cell)
	}
	for row := range targetRows {
		sortCells(targetRows[row])
	}

	decoder := xml.NewDecoder(bytes.NewReader(sheetXML))
	var out bytes.Buffer
	encoder := xml.NewEncoder(&out)
	prefixes := namespacePrefixes{
		mainNamespace:       "",
		officeRelsNamespace: "r",
	}
	inSheetData := false
	currentRow := 0
	seenRows := map[int]bool{}

	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		switch typed := token.(type) {
		case xml.StartElement:
			switch typed.Name.Local {
			case "sheetData":
				inSheetData = true
			case "row":
				currentRow = rowNumberFromAttrs(typed.Attr)
				if currentRow > 0 {
					seenRows[currentRow] = true
				}
			case "c":
				cell := attrValue(typed.Attr, "r")
				if imageID, ok := remaining[cell]; ok {
					if err := writeDispImageElement(encoder, typed, cell, imageID, prefixes); err != nil {
						return nil, err
					}
					if err := skipElement(decoder, typed.Name); err != nil {
						return nil, err
					}
					delete(remaining, cell)
					continue
				}
			}
			if err := encoder.EncodeToken(normalizeStartElement(typed, prefixes)); err != nil {
				return nil, err
			}
		case xml.EndElement:
			if typed.Name.Local == "row" && currentRow > 0 {
				for _, cell := range targetRows[currentRow] {
					if imageID, ok := remaining[cell]; ok {
						if err := writeNewDispImageElement(encoder, cell, imageID); err != nil {
							return nil, err
						}
						delete(remaining, cell)
					}
				}
				currentRow = 0
			}
			if typed.Name.Local == "sheetData" {
				for row, cells := range targetRows {
					if seenRows[row] {
						continue
					}
					if err := writeMissingDispImageRow(encoder, row, cells, remaining); err != nil {
						return nil, err
					}
					for _, cell := range cells {
						delete(remaining, cell)
					}
				}
				inSheetData = false
			}
			if err := encoder.EncodeToken(normalizeEndElement(typed, prefixes)); err != nil {
				return nil, err
			}
		default:
			if err := encoder.EncodeToken(token); err != nil {
				return nil, err
			}
		}
	}
	if inSheetData {
		return nil, fmt.Errorf("worksheet sheetData was not closed")
	}
	if len(remaining) > 0 {
		return nil, fmt.Errorf("failed to write %d DISPIMG cell(s)", len(remaining))
	}
	if err := encoder.Flush(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func writeMissingRow(encoder *xml.Encoder, row int, cells []string, remaining map[string]int) error {
	start := xml.StartElement{
		Name: xml.Name{Local: "row"},
		Attr: []xml.Attr{{Name: xml.Name{Local: "r"}, Value: strconv.Itoa(row)}},
	}
	if err := encoder.EncodeToken(start); err != nil {
		return err
	}
	for _, cell := range cells {
		if vm, ok := remaining[cell]; ok {
			if err := writeNewCellImageElement(encoder, cell, vm); err != nil {
				return err
			}
		}
	}
	return encoder.EncodeToken(start.End())
}

func writeMissingDispImageRow(encoder *xml.Encoder, row int, cells []string, remaining map[string]string) error {
	start := xml.StartElement{
		Name: xml.Name{Local: "row"},
		Attr: []xml.Attr{{Name: xml.Name{Local: "r"}, Value: strconv.Itoa(row)}},
	}
	if err := encoder.EncodeToken(start); err != nil {
		return err
	}
	for _, cell := range cells {
		if imageID, ok := remaining[cell]; ok {
			if err := writeNewDispImageElement(encoder, cell, imageID); err != nil {
				return err
			}
		}
	}
	return encoder.EncodeToken(start.End())
}

func writeCellImageElement(encoder *xml.Encoder, original xml.StartElement, cell string, vm int, prefixes namespacePrefixes) error {
	original = normalizeStartElement(original, prefixes)
	attrs := make([]xml.Attr, 0, len(original.Attr)+2)
	hasRef := false
	for _, attr := range original.Attr {
		switch attr.Name.Local {
		case "r":
			hasRef = true
			attrs = append(attrs, attr)
		case "t", "vm":
			continue
		default:
			attrs = append(attrs, attr)
		}
	}
	if !hasRef {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "r"}, Value: cell})
	}
	attrs = append(attrs,
		xml.Attr{Name: xml.Name{Local: "t"}, Value: "e"},
		xml.Attr{Name: xml.Name{Local: "vm"}, Value: strconv.Itoa(vm)},
	)
	start := xml.StartElement{Name: original.Name, Attr: attrs}
	return writeCellValue(encoder, start)
}

func writeDispImageElement(encoder *xml.Encoder, original xml.StartElement, cell, imageID string, prefixes namespacePrefixes) error {
	original = normalizeStartElement(original, prefixes)
	attrs := make([]xml.Attr, 0, len(original.Attr))
	hasRef := false
	for _, attr := range original.Attr {
		switch attr.Name.Local {
		case "r":
			hasRef = true
			attrs = append(attrs, attr)
		case "t", "vm":
			continue
		default:
			attrs = append(attrs, attr)
		}
	}
	if !hasRef {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "r"}, Value: cell})
	}
	attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "t"}, Value: "str"})
	start := xml.StartElement{Name: original.Name, Attr: attrs}
	return writeDispImageFormula(encoder, start, imageID)
}

func writeNewCellImageElement(encoder *xml.Encoder, cell string, vm int) error {
	start := xml.StartElement{
		Name: xml.Name{Local: "c"},
		Attr: []xml.Attr{
			{Name: xml.Name{Local: "r"}, Value: cell},
			{Name: xml.Name{Local: "t"}, Value: "e"},
			{Name: xml.Name{Local: "vm"}, Value: strconv.Itoa(vm)},
		},
	}
	return writeCellValue(encoder, start)
}

func writeNewDispImageElement(encoder *xml.Encoder, cell, imageID string) error {
	start := xml.StartElement{
		Name: xml.Name{Local: "c"},
		Attr: []xml.Attr{
			{Name: xml.Name{Local: "r"}, Value: cell},
			{Name: xml.Name{Local: "t"}, Value: "str"},
		},
	}
	return writeDispImageFormula(encoder, start, imageID)
}

func writeCellValue(encoder *xml.Encoder, start xml.StartElement) error {
	if err := encoder.EncodeToken(start); err != nil {
		return err
	}
	valueStart := xml.StartElement{Name: xml.Name{Local: "v"}}
	if err := encoder.EncodeToken(valueStart); err != nil {
		return err
	}
	if err := encoder.EncodeToken(xml.CharData("#VALUE!")); err != nil {
		return err
	}
	if err := encoder.EncodeToken(valueStart.End()); err != nil {
		return err
	}
	return encoder.EncodeToken(start.End())
}

func writeDispImageFormula(encoder *xml.Encoder, start xml.StartElement, imageID string) error {
	if err := encoder.EncodeToken(start); err != nil {
		return err
	}
	displayFormula := fmt.Sprintf(`DISPIMG("%s",1)`, imageID)
	formulaStart := xml.StartElement{Name: xml.Name{Local: "f"}}
	if err := encoder.EncodeToken(formulaStart); err != nil {
		return err
	}
	if err := encoder.EncodeToken(xml.CharData("_xlfn." + displayFormula)); err != nil {
		return err
	}
	if err := encoder.EncodeToken(formulaStart.End()); err != nil {
		return err
	}
	valueStart := xml.StartElement{Name: xml.Name{Local: "v"}}
	if err := encoder.EncodeToken(valueStart); err != nil {
		return err
	}
	if err := encoder.EncodeToken(xml.CharData("=" + displayFormula)); err != nil {
		return err
	}
	if err := encoder.EncodeToken(valueStart.End()); err != nil {
		return err
	}
	return encoder.EncodeToken(start.End())
}

func skipElement(decoder *xml.Decoder, name xml.Name) error {
	depth := 1
	for depth > 0 {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		switch typed := token.(type) {
		case xml.StartElement:
			depth++
		case xml.EndElement:
			if typed.Name.Local == name.Local && typed.Name.Space == name.Space {
				depth--
			} else if depth > 1 {
				depth--
			}
		}
	}
	return nil
}

func rowNumberFromAttrs(attrs []xml.Attr) int {
	value := attrValue(attrs, "r")
	if value == "" {
		return 0
	}
	row, _ := strconv.Atoi(value)
	return row
}

func attrValue(attrs []xml.Attr, local string) string {
	for _, attr := range attrs {
		if attr.Name.Local == local {
			return attr.Value
		}
	}
	return ""
}

func rowFromCell(cell string) (int, error) {
	idx := 0
	for idx < len(cell) && ((cell[idx] >= 'A' && cell[idx] <= 'Z') || (cell[idx] >= 'a' && cell[idx] <= 'z')) {
		idx++
	}
	if idx == 0 || idx == len(cell) {
		return 0, fmt.Errorf("invalid cell reference %s", cell)
	}
	row, err := strconv.Atoi(cell[idx:])
	if err != nil {
		return 0, fmt.Errorf("invalid cell reference %s", cell)
	}
	return row, nil
}

func sortCells(cells []string) {
	sort.Slice(cells, func(i, j int) bool {
		colI, rowI := splitCellForSort(cells[i])
		colJ, rowJ := splitCellForSort(cells[j])
		if rowI != rowJ {
			return rowI < rowJ
		}
		return colI < colJ
	})
}

func splitCellForSort(cell string) (int, int) {
	idx := 0
	col := 0
	for idx < len(cell) && ((cell[idx] >= 'A' && cell[idx] <= 'Z') || (cell[idx] >= 'a' && cell[idx] <= 'z')) {
		ch := cell[idx]
		if ch >= 'a' && ch <= 'z' {
			ch -= 'a' - 'A'
		}
		col = col*26 + int(ch-'A'+1)
		idx++
	}
	row, _ := strconv.Atoi(cell[idx:])
	return col, row
}

func convertedURLs(images []embeddedCellImage) []string {
	seen := map[string]bool{}
	var urls []string
	for _, image := range images {
		url := image.Target.URL
		if url == "" || seen[url] {
			continue
		}
		seen[url] = true
		urls = append(urls, url)
	}
	return urls
}

func imageCellURLs(cells []imageCell) []string {
	seen := map[string]bool{}
	var urls []string
	for _, cell := range cells {
		url := cell.URL
		if url == "" || seen[url] {
			continue
		}
		seen[url] = true
		urls = append(urls, url)
	}
	return urls
}

func sanitizeURLsInPackage(pkg map[string][]byte, urls []string) {
	for name, data := range pkg {
		if !isXMLPart(name) {
			continue
		}
		text := string(data)
		for _, url := range urls {
			placeholder := sanitizedURLPlaceholder(url)
			text = strings.ReplaceAll(text, url, placeholder)
			text = strings.ReplaceAll(text, xmlEscaped(url), placeholder)
		}
		pkg[name] = []byte(text)
	}
}

func sanitizedURLPlaceholder(rawURL string) string {
	hash := sha1.Sum([]byte(rawURL))
	return "about:blank#eic-" + hex.EncodeToString(hash[:])[:16]
}

func ensureNoURLResidue(pkg map[string][]byte, urls []string) error {
	for name, data := range pkg {
		if !isXMLPart(name) {
			continue
		}
		for _, url := range urls {
			if bytes.Contains(data, []byte(url)) || bytes.Contains(data, []byte(xmlEscaped(url))) {
				return fmt.Errorf("url residue remains in %s", name)
			}
		}
	}
	return nil
}

func isXMLPart(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".xml") || strings.HasSuffix(lower, ".rels")
}

func xmlEscaped(value string) string {
	var buf bytes.Buffer
	_ = xml.EscapeText(&buf, []byte(value))
	return buf.String()
}
