# Excel Image Converter

Convert Excel `IMAGE(url)` formulas and plain image links into real Excel pictures embedded in cells.

Current version: `v1.0.5`.

The tool is designed for non-technical Windows and macOS users:

- Double-click the Windows executable or macOS app to open the app.
- Choose one or more `.xlsx` files.
- Or drag `.xlsx` files into the app window.
- Watch current conversions in the "正在转换" tab.
- Review past conversions in the "历史记录" tab.
- Open source or converted files from either tab.
- Click the folder column to reveal a file in Explorer or Finder.
- Choose "兼容Excel" for native Excel cell pictures, or "兼容飞书/WPS" for WPS `DISPIMG` cell images.
- The first-run default is "兼容飞书/WPS" and "保留链接：否"; the app remembers later user changes.
- Clear conversion history when needed.
- Converted workbooks are saved next to the source files with `_pictures.xlsx`.
- GUI conversions are queued globally: at most 2 workbooks convert at the same time, and each workbook uses 4 image download workers.

## What It Does

For every worksheet in each workbook, the converter:

1. Finds formulas like `=IMAGE("https://example.com/a.png")`.
2. Finds plain image links like `https://example.com/a.jpg?token=...`.
3. Downloads images concurrently, with 4 download workers per workbook by default.
4. Embeds images in the original cells, using either Excel rich-value cell pictures or WPS `DISPIMG` cell images.
5. Clears the original URL/formula by default, or keeps the original URL as a cell hyperlink when "保留链接：是" is selected.
6. Saves a new workbook without modifying the original file.

History is stored in the user's app data directory:

```text
%APPDATA%\ExcelImageConverter\history.json
~/Library/Application Support/ExcelImageConverter/history.json
```

If a source or converted file has been deleted, its file name is shown with strike-through text in the History tab.

User options are stored separately:

```text
%APPDATA%\ExcelImageConverter\settings.json
~/Library/Application Support/ExcelImageConverter/settings.json
```

## Limitations

- Only `.xlsx` is supported.
- URLs must be directly accessible by the converter. Login-only URLs, private cookies, anti-hotlinking, or expired signed links may fail.
- Formula support targets literal URL formulas such as `=IMAGE("https://...")`. Formulas that build the URL from other cells are left unchanged.
- Plain URL support requires an image extension before query or hash parameters, such as `.png`, `.jpg`, `.jpeg`, `.gif`, `.bmp`, or `.webp`.
- "兼容Excel" uses Excel's newer rich data cell image structure.
- "兼容飞书/WPS" uses WPS-compatible `DISPIMG` formulas plus `xl/cellimages.xml`.

## Command Line

The Windows executable starts the GUI. On non-Windows builds, the command line mode remains useful for development and testing:

```powershell
ExcelImageConverter.exe input.xlsx
ExcelImageConverter.exe input1.xlsx input2.xlsx -out-dir converted
ExcelImageConverter.exe -keep-url input.xlsx
ExcelImageConverter.exe -workers 4 input.xlsx
ExcelImageConverter.exe -compat wps input.xlsx
ExcelImageConverter.exe -version
```

## Build

Install Go 1.25+.

```bash
go mod tidy
./scripts/build_windows.sh
./scripts/build_macos_dmg.sh
```

The packaged outputs are written to:

```text
dist/windows/ExcelImageConverter.exe
dist/macos/ExcelImageConverter-mac-arm64.dmg
```

`assets/app-icon.png` is the generated app icon source. `scripts/make_icons.go` creates `assets/app-icon.ico` and `assets/app-icon.icns` for Windows taskbar and macOS Dock/Finder integration.
