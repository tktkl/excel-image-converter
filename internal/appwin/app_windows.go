//go:build windows

package appwin

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"github.com/lxn/win"

	"github.com/wutong/excel-image-converter/internal/buildinfo"
	"github.com/wutong/excel-image-converter/internal/converter"
	"github.com/wutong/excel-image-converter/internal/history"
	"github.com/wutong/excel-image-converter/internal/settings"
)

const (
	statusQueued  = "Queued"
	statusRunning = "Running"
	statusDone    = "Done"
	statusFailed  = "Failed"

	appIconResourceID         = "2"
	appIconFallbackResourceID = "1"

	sourceFileColumn   = 0
	sourceFolderColumn = 1
	outputFileColumn   = 2
	outputFolderColumn = 3
)

type App struct {
	mw *walk.MainWindow

	runningView *walk.TableView
	historyView *walk.TableView
	statusLabel *walk.Label
	addButton   *walk.PushButton
	clearButton *walk.PushButton
	compatExcel *walk.RadioButton
	compatWPS   *walk.RadioButton

	runningModel  *taskModel
	historyModel  *historyModel
	store         *history.Store
	settingsStore *settings.Store
	settings      settings.Settings
	convertSlots  chan struct{}

	runningClickColumn int
	historyClickColumn int
	runningClickAt     time.Time
	historyClickAt     time.Time
	lastOpenKey        string
	lastOpenAt         time.Time

	history []history.Entry
	mu      sync.Mutex
}

type taskRow struct {
	ID        string
	Source    string
	Output    string
	Status    string
	Detail    string
	StartedAt time.Time
	Result    converter.Result
	Error     string
}

type taskModel struct {
	walk.TableModelBase
	mu    sync.RWMutex
	items []*taskRow
}

func (m *taskModel) RowCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.items)
}

func (m *taskModel) Value(row, col int) any {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if row < 0 || row >= len(m.items) {
		return ""
	}
	item := m.items[row]
	switch col {
	case sourceFileColumn:
		return filepath.Base(item.Source)
	case sourceFolderColumn:
		return "📁"
	case outputFileColumn:
		if item.Output != "" {
			return filepath.Base(item.Output)
		}
		return "-"
	case outputFolderColumn:
		if item.Output != "" {
			return "📁"
		}
		return ""
	case 4:
		return displayStatus(item.Status)
	case 5:
		return item.Detail
	default:
		return ""
	}
}

func (m *taskModel) appendRows(rows []*taskRow) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items = append(m.items, rows...)
}

func (m *taskModel) update(row *taskRow, status, detail, output string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	row.Status = status
	row.Detail = detail
	if output != "" {
		row.Output = output
	}
}

func (m *taskModel) statusAt(row int) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if row < 0 || row >= len(m.items) {
		return "", false
	}
	return m.items[row].Status, true
}

func (m *taskModel) selectedPaths(row int) (source, output string, ok bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if row < 0 || row >= len(m.items) {
		return "", "", false
	}
	item := m.items[row]
	return item.Source, item.Output, true
}

func (m *taskModel) snapshot(row *taskRow) taskRow {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return *row
}

type historyModel struct {
	walk.TableModelBase
	mu    sync.RWMutex
	items []history.Entry
}

func (m *historyModel) RowCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.items)
}

func (m *historyModel) Value(row, col int) any {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if row < 0 || row >= len(m.items) {
		return ""
	}
	item := m.items[row]
	switch col {
	case sourceFileColumn:
		return filepath.Base(item.Source)
	case sourceFolderColumn:
		return "📁"
	case outputFileColumn:
		if item.Output != "" {
			return filepath.Base(item.Output)
		}
		return "-"
	case outputFolderColumn:
		if item.Output != "" {
			return "📁"
		}
		return ""
	case 4:
		return displayStatus(item.Status)
	case 5:
		if item.EndedAt.IsZero() {
			return "-"
		}
		return item.EndedAt.Format("2006-01-02 15:04")
	case 6:
		if item.Error != "" {
			return displayErrorMessage(item.Error)
		}
		return fmt.Sprintf("已转换 %d 张，失败 %d 张", item.Converted, item.Failed)
	default:
		return ""
	}
}

func (m *historyModel) setItems(items []history.Entry) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items = append([]history.Entry(nil), items...)
}

func (m *historyModel) itemAt(row int) (history.Entry, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if row < 0 || row >= len(m.items) {
		return history.Entry{}, false
	}
	return m.items[row], true
}

func (m *historyModel) clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items = nil
}

func Run(initialFiles []string) error {
	store, err := history.NewDefaultStore()
	if err != nil {
		return err
	}
	settingsStore, err := settings.NewDefaultStore()
	if err != nil {
		return err
	}
	appSettings, err := settingsStore.Load()
	if err != nil {
		return err
	}
	appSettings.KeepURL = false

	app := &App{
		runningModel:       &taskModel{},
		historyModel:       &historyModel{},
		store:              store,
		settingsStore:      settingsStore,
		settings:           appSettings,
		convertSlots:       make(chan struct{}, 2),
		runningClickColumn: -1,
		historyClickColumn: -1,
	}
	if err := app.loadHistory(); err != nil {
		return err
	}
	if err := app.buildUI(); err != nil {
		return err
	}
	app.applySettings()
	if len(initialFiles) > 0 {
		app.enqueueFiles(initialFiles)
	}
	app.mw.Run()
	return nil
}

func (a *App) loadHistory() error {
	entries, err := a.store.Load()
	if err != nil {
		return err
	}
	a.history = entries
	a.historyModel.setItems(entries)
	return nil
}

func (a *App) buildUI() error {
	deletedFont, _ := walk.NewFont("Segoe UI", 9, walk.FontStrikeOut)
	runningBold, _ := walk.NewFont("Segoe UI", 9, walk.FontBold)

	if err := (MainWindow{
		AssignTo: &a.mw,
		Title:    fmt.Sprintf("Excel 图片转换器 v%s", buildinfo.DisplayVersion()),
		MinSize:  Size{Width: 980, Height: 650},
		Size:     Size{Width: 1120, Height: 720},
		Font:     Font{Family: "Segoe UI", PointSize: 9},
		Layout:   VBox{Margins: Margins{Left: 16, Top: 16, Right: 16, Bottom: 16}, Spacing: 12},
		OnDropFiles: func(files []string) {
			a.enqueueFiles(files)
		},
		Children: []Widget{
			Composite{
				Layout: HBox{MarginsZero: true, Spacing: 8},
				Children: []Widget{
					Composite{
						Layout:        VBox{MarginsZero: true, Spacing: 8},
						StretchFactor: 1,
						Children: []Widget{
							Label{
								Text: "Excel IMAGE(URL) 转图片对象",
								Font: Font{Family: "Segoe UI", PointSize: 16, Bold: true},
							},
							Label{
								Text: "选择或拖入 Excel 工作簿，原文件不会被修改，转换后的文件会保存到原文件旁边。",
							},
						},
					},
					Label{
						Text:      fmt.Sprintf("v%s\npower by Thunder.Wu", buildinfo.DisplayVersion()),
						Alignment: AlignHFarVFar,
						Font:      Font{Family: "Segoe UI", PointSize: 9, Italic: true},
					},
				},
			},
			Composite{
				Layout: HBox{Margins: Margins{Left: 14, Top: 12, Right: 14, Bottom: 12}, Spacing: 12},
				Children: []Widget{
					PushButton{
						AssignTo: &a.addButton,
						Text:     "选择 Excel 文件",
						MinSize:  Size{Width: 170, Height: 34},
						OnClicked: func() {
							a.chooseFiles()
						},
					},
					Label{
						Text:          "不保留原链接；也可以把 .xlsx 文件直接拖到窗口里开始转换",
						StretchFactor: 1,
					},
					Label{
						Text: "兼容模式：",
					},
					Composite{
						Layout: HBox{MarginsZero: true, Spacing: 6},
						Children: []Widget{
							RadioButton{
								AssignTo:  &a.compatExcel,
								Text:      "兼容Excel",
								OnClicked: a.saveCurrentSettings,
							},
							RadioButton{
								AssignTo:  &a.compatWPS,
								Text:      "兼容飞书/WPS",
								OnClicked: a.saveCurrentSettings,
							},
						},
					},
					Label{
						AssignTo: &a.statusLabel,
						Text:     "准备就绪",
					},
				},
			},
			TabWidget{
				Pages: []TabPage{
					{
						Title:  "正在转换",
						Layout: VBox{Margins: Margins{Left: 0, Top: 10, Right: 0, Bottom: 0}, Spacing: 8},
						Children: []Widget{
							TableView{
								AssignTo:         &a.runningView,
								AlternatingRowBG: true,
								ColumnsOrderable: false,
								Columns: []TableViewColumn{
									{Title: "源文件", Width: 210},
									{Title: "位置", Width: 42},
									{Title: "转换后文件", Width: 210},
									{Title: "位置", Width: 42},
									{Title: "状态", Width: 110},
									{Title: "详情", Width: 380},
								},
								OnMouseDown: func(x, y int, button walk.MouseButton) {
									if button == walk.LeftButton {
										a.rememberRunningClickColumn(x)
									}
								},
								OnMouseUp: func(x, y int, button walk.MouseButton) {
									if button == walk.LeftButton {
										a.handleTaskTableClick(x)
									}
								},
								StyleCell: func(style *walk.CellStyle) {
									status, ok := a.runningModel.statusAt(style.Row())
									if !ok {
										return
									}
									switch status {
									case statusRunning:
										style.Font = runningBold
										style.TextColor = walk.RGB(18, 96, 170)
									case statusDone:
										style.TextColor = walk.RGB(22, 128, 72)
									case statusFailed:
										style.TextColor = walk.RGB(176, 48, 48)
									}
								},
								Model: a.runningModel,
								OnItemActivated: func() {
									a.handleTaskTableActivate()
								},
							},
							Composite{
								Layout: HBox{MarginsZero: true, Spacing: 8},
								Children: []Widget{
									PushButton{
										Text: "打开源文件",
										OnClicked: func() {
											a.openSelectedTaskSource()
										},
									},
									PushButton{
										Text: "打开转换后文件",
										OnClicked: func() {
											a.openSelectedTaskOutput()
										},
									},
									HSpacer{},
								},
							},
						},
					},
					{
						Title:  "历史记录",
						Layout: VBox{Margins: Margins{Left: 0, Top: 10, Right: 0, Bottom: 0}, Spacing: 8},
						Children: []Widget{
							TableView{
								AssignTo:         &a.historyView,
								AlternatingRowBG: true,
								ColumnsOrderable: false,
								Columns: []TableViewColumn{
									{Title: "源文件", Width: 210},
									{Title: "位置", Width: 42},
									{Title: "转换后文件", Width: 210},
									{Title: "位置", Width: 42},
									{Title: "状态", Width: 90},
									{Title: "完成时间", Width: 130},
									{Title: "详情", Width: 300},
								},
								OnMouseDown: func(x, y int, button walk.MouseButton) {
									if button == walk.LeftButton {
										a.rememberHistoryClickColumn(x)
									}
								},
								OnMouseUp: func(x, y int, button walk.MouseButton) {
									if button == walk.LeftButton {
										a.handleHistoryTableClick(x)
									}
								},
								StyleCell: func(style *walk.CellStyle) {
									item, ok := a.historyModel.itemAt(style.Row())
									if !ok {
										return
									}
									missing := !fileExists(item.Source) || (item.Output != "" && !fileExists(item.Output))
									if missing {
										style.Font = deletedFont
										style.TextColor = walk.RGB(136, 136, 136)
										return
									}
									if item.Status == statusDone {
										style.TextColor = walk.RGB(22, 128, 72)
									}
									if item.Status == statusFailed {
										style.TextColor = walk.RGB(176, 48, 48)
									}
								},
								Model: a.historyModel,
								OnItemActivated: func() {
									a.handleHistoryTableActivate()
								},
							},
							Composite{
								Layout: HBox{MarginsZero: true, Spacing: 8},
								Children: []Widget{
									PushButton{
										Text: "打开源文件",
										OnClicked: func() {
											a.openSelectedHistorySource()
										},
									},
									PushButton{
										Text: "打开转换后文件",
										OnClicked: func() {
											a.openSelectedHistoryOutput()
										},
									},
									HSpacer{},
									PushButton{
										AssignTo: &a.clearButton,
										Text:     "清理历史记录",
										OnClicked: func() {
											a.clearHistory()
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}).Create(); err != nil {
		return err
	}
	return a.applyWindowIcon()
}

func (a *App) applyWindowIcon() error {
	for _, id := range []string{appIconResourceID, appIconFallbackResourceID} {
		icon, err := walk.Resources.Icon(id)
		if err == nil {
			return a.mw.SetIcon(icon)
		}
	}
	return nil
}

func (a *App) chooseFiles() {
	dlg := new(walk.FileDialog)
	dlg.Title = "选择 Excel 工作簿"
	dlg.Filter = "Excel 工作簿 (*.xlsx)|*.xlsx"
	accepted, err := dlg.ShowOpenMultiple(a.mw)
	if err != nil {
		walk.MsgBox(a.mw, "选择文件失败", err.Error(), walk.MsgBoxIconError)
		return
	}
	if accepted {
		a.enqueueFiles(dlg.FilePaths)
	}
}

func (a *App) enqueueFiles(files []string) {
	files = filterExcelFiles(files)
	if len(files) == 0 {
		walk.MsgBox(a.mw, "没有可转换的 Excel 文件", "请选择 .xlsx 格式的文件。", walk.MsgBoxIconInformation)
		return
	}

	var rows []*taskRow
	for _, file := range files {
		row := &taskRow{
			ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
			Source:    file,
			Status:    statusQueued,
			Detail:    "等待中",
			StartedAt: time.Now(),
		}
		rows = append(rows, row)
	}
	a.runningModel.appendRows(rows)
	a.refreshRunning()
	a.setStatus(fmt.Sprintf("已加入 %d 个文件", len(files)))
	for _, row := range rows {
		go a.convert(row)
	}
}

func (a *App) convert(row *taskRow) {
	a.convertSlots <- struct{}{}
	defer func() {
		<-a.convertSlots
	}()

	a.updateTask(row, statusRunning, "正在打开工作簿", "")
	taskSettings := a.currentSettings()
	opts := converter.Options{
		KeepURL:         taskSettings.KeepURL,
		CellImageMode:   taskSettings.CellImageMode,
		DownloadWorkers: 4,
		Progress: func(event converter.ProgressEvent) {
			msg := displayProgressMessage(event.Message)
			if event.Sheet != "" && event.Cell != "" {
				msg = fmt.Sprintf("%s：%s!%s", msg, event.Sheet, event.Cell)
			}
			a.updateTask(row, statusRunning, msg, "")
		},
	}

	result, err := converter.ConvertFile(row.Source, opts)
	if err != nil {
		a.updateTask(row, statusFailed, displayErrorMessage(err.Error()), "")
		a.addHistory(row, result, err)
		return
	}

	status := statusDone
	detail := fmt.Sprintf("已转换 %d 张图片", result.Converted)
	if result.Converted == 0 && result.Failed > 0 {
		status = statusFailed
		detail = fmt.Sprintf("转换失败 %d 张图片", result.Failed)
	} else if result.Failed > 0 {
		detail = fmt.Sprintf("已转换 %d 张，失败 %d 张", result.Converted, result.Failed)
	}
	a.updateTask(row, status, detail, result.OutputPath)
	a.addHistory(row, result, nil)
}

func (a *App) updateTask(row *taskRow, status, detail, output string) {
	a.runningModel.update(row, status, detail, output)
	a.mw.Synchronize(func() {
		a.refreshRunning()
		a.setStatus(detail)
	})
}

func (a *App) addHistory(row *taskRow, result converter.Result, err error) {
	task := a.runningModel.snapshot(row)
	entry := history.Entry{
		ID:        task.ID,
		Source:    task.Source,
		Output:    result.OutputPath,
		Status:    task.Status,
		Converted: result.Converted,
		Failed:    result.Failed,
		StartedAt: task.StartedAt,
		EndedAt:   time.Now(),
	}
	if err != nil {
		entry.Status = statusFailed
		entry.Error = err.Error()
	}

	a.mu.Lock()
	a.history = append([]history.Entry{entry}, a.history...)
	if len(a.history) > 500 {
		a.history = a.history[:500]
	}
	snapshot := append([]history.Entry(nil), a.history...)
	saveErr := a.store.Save(snapshot)
	a.historyModel.setItems(snapshot)
	a.mu.Unlock()

	a.mw.Synchronize(func() {
		a.refreshHistory()
		if saveErr != nil {
			a.setStatus("历史记录保存失败：" + displayErrorMessage(saveErr.Error()))
		}
	})
}

func (a *App) refreshRunning() {
	a.runningModel.PublishRowsReset()
}

func (a *App) refreshHistory() {
	a.historyModel.PublishRowsReset()
}

func (a *App) setStatus(text string) {
	if a.statusLabel != nil {
		_ = a.statusLabel.SetText(text)
	}
}

func (a *App) applySettings() {
	if a.settings.CellImageMode == converter.CellImageModeWPS {
		if a.compatWPS != nil {
			a.compatWPS.SetChecked(true)
		}
	} else if a.compatExcel != nil {
		a.compatExcel.SetChecked(true)
	}
}

func (a *App) saveCurrentSettings() {
	current := settings.Settings{
		KeepURL:       false,
		CellImageMode: converter.DefaultCellImageMode(),
	}
	if a.compatExcel != nil && a.compatExcel.Checked() {
		current.CellImageMode = converter.CellImageModeExcel
	} else if a.compatWPS != nil && a.compatWPS.Checked() {
		current.CellImageMode = converter.CellImageModeWPS
	}

	a.mu.Lock()
	a.settings = current
	a.mu.Unlock()

	if a.settingsStore != nil {
		if err := a.settingsStore.Save(current); err != nil {
			a.setStatus("设置保存失败：" + displayErrorMessage(err.Error()))
		}
	}
}

func (a *App) currentSettings() settings.Settings {
	a.mu.Lock()
	defer a.mu.Unlock()
	current := a.settings
	current.KeepURL = false
	return current
}

func (a *App) clearHistory() {
	if walk.MsgBox(a.mw, "清理历史记录", "确定要清理所有转换历史记录吗？", walk.MsgBoxYesNo|walk.MsgBoxIconQuestion) != win.IDYES {
		return
	}
	a.mu.Lock()
	if err := a.store.Clear(); err != nil {
		a.mu.Unlock()
		walk.MsgBox(a.mw, "清理历史记录失败", displayErrorMessage(err.Error()), walk.MsgBoxIconError)
		return
	}
	a.history = nil
	a.historyModel.clear()
	a.mu.Unlock()
	a.refreshHistory()
	a.setStatus("历史记录已清理")
}

func (a *App) selectedTaskPaths() (source, output string, ok bool) {
	if a.runningView == nil {
		return "", "", false
	}
	idx := a.runningView.CurrentIndex()
	return a.runningModel.selectedPaths(idx)
}

func (a *App) selectedHistory() (history.Entry, bool) {
	if a.historyView == nil {
		return history.Entry{}, false
	}
	idx := a.historyView.CurrentIndex()
	return a.historyModel.itemAt(idx)
}

func (a *App) openSelectedTaskSource() {
	if source, _, ok := a.selectedTaskPaths(); ok {
		openIfExists(source)
	}
}

func (a *App) openSelectedTaskOutput() {
	if _, output, ok := a.selectedTaskPaths(); ok {
		openIfExists(output)
	}
}

func (a *App) openSelectedHistorySource() {
	if row, ok := a.selectedHistory(); ok {
		openIfExists(row.Source)
	}
}

func (a *App) openSelectedHistoryOutput() {
	if row, ok := a.selectedHistory(); ok {
		openIfExists(row.Output)
	}
}

func (a *App) rememberRunningClickColumn(x int) {
	a.runningClickColumn = columnAt(a.runningView, x)
	a.runningClickAt = time.Now()
}

func (a *App) rememberHistoryClickColumn(x int) {
	a.historyClickColumn = columnAt(a.historyView, x)
	a.historyClickAt = time.Now()
}

func (a *App) handleTaskTableClick(x int) {
	col := columnAt(a.runningView, x)
	a.runningClickColumn = col
	a.runningClickAt = time.Now()
	a.openSelectedTaskByColumn(col, false)
}

func (a *App) handleTaskTableActivate() {
	a.openSelectedTaskByColumn(a.recentRunningClickColumn(), true)
}

func (a *App) handleHistoryTableClick(x int) {
	col := columnAt(a.historyView, x)
	a.historyClickColumn = col
	a.historyClickAt = time.Now()
	a.openSelectedHistoryByColumn(col, false)
}

func (a *App) handleHistoryTableActivate() {
	a.openSelectedHistoryByColumn(a.recentHistoryClickColumn(), true)
}

func (a *App) recentRunningClickColumn() int {
	if time.Since(a.runningClickAt) > 2*time.Second {
		return -1
	}
	return a.runningClickColumn
}

func (a *App) recentHistoryClickColumn() int {
	if time.Since(a.historyClickAt) > 2*time.Second {
		return -1
	}
	return a.historyClickColumn
}

func (a *App) openSelectedTaskByColumn(col int, fromActivate bool) {
	source, output, ok := a.selectedTaskPaths()
	if !ok {
		return
	}
	switch col {
	case sourceFolderColumn:
		a.openFolderOnce(source)
	case outputFolderColumn:
		a.openFolderOnce(output)
	case sourceFileColumn:
		if fromActivate {
			a.openFileOnce(source)
		}
	case outputFileColumn:
		if fromActivate {
			a.openFileOnce(output)
		}
	}
}

func (a *App) openSelectedHistoryByColumn(col int, fromActivate bool) {
	row, ok := a.selectedHistory()
	if !ok {
		return
	}
	switch col {
	case sourceFolderColumn:
		a.openFolderOnce(row.Source)
	case outputFolderColumn:
		a.openFolderOnce(row.Output)
	case sourceFileColumn:
		if fromActivate {
			a.openFileOnce(row.Source)
		}
	case outputFileColumn:
		if fromActivate {
			a.openFileOnce(row.Output)
		}
	}
}

func (a *App) openFileOnce(path string) {
	a.openOnce("file:"+path, func() {
		openIfExists(path)
	})
}

func (a *App) openFolderOnce(path string) {
	a.openOnce("folder:"+path, func() {
		openFolderForFile(path)
	})
}

func (a *App) openOnce(key string, open func()) {
	if key == "" || strings.HasSuffix(key, ":") {
		return
	}
	now := time.Now()
	if a.lastOpenKey == key && now.Sub(a.lastOpenAt) < 700*time.Millisecond {
		return
	}
	a.lastOpenKey = key
	a.lastOpenAt = now
	open()
}

func columnAt(view *walk.TableView, x int) int {
	if view == nil || view.Columns() == nil {
		return -1
	}
	px := 0
	for i := 0; i < view.Columns().Len(); i++ {
		col := view.Columns().At(i)
		if col == nil || !col.Visible() {
			continue
		}
		px += view.IntFrom96DPI(col.Width())
		if x < px {
			return i
		}
	}
	return -1
}

func openIfExists(path string) {
	if path == "" || !fileExists(path) {
		return
	}
	_ = exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", path).Start()
}

func openFolderForFile(path string) {
	if path == "" || !fileExists(path) {
		return
	}
	_ = exec.Command("explorer.exe", "/select,", path).Start()
}

func filterExcelFiles(files []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, file := range files {
		file = strings.Trim(file, `"`)
		if strings.EqualFold(filepath.Ext(file), ".xlsx") && !seen[file] {
			out = append(out, file)
			seen[file] = true
		}
	}
	return out
}

func fileExists(path string) bool {
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

func displayStatus(status string) string {
	switch status {
	case statusQueued:
		return "等待中"
	case statusRunning:
		return "转换中"
	case statusDone:
		return "已完成"
	case statusFailed:
		return "失败"
	default:
		return status
	}
}

func displayProgressMessage(message string) string {
	switch message {
	case "Downloading image":
		return "正在下载图片"
	case "Inserting picture":
		return "正在插入图片"
	case "Embedding pictures in cells":
		return "正在嵌入单元格图片"
	case "Saving workbook":
		return "正在保存工作簿"
	default:
		return message
	}
}

func ErrToMessage(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, os.ErrNotExist) {
		return "文件已不存在"
	}
	return displayErrorMessage(err.Error())
}

func displayErrorMessage(message string) string {
	switch {
	case strings.Contains(message, "no IMAGE(url) formulas or plain image links found"):
		return "未找到可转换的 IMAGE(URL) 公式或图片链接"
	case strings.Contains(message, "no literal IMAGE(url) formulas found"):
		return "未找到可转换的 IMAGE(URL) 公式"
	case strings.HasPrefix(message, "only .xlsx is supported"):
		return "仅支持 .xlsx 格式的文件"
	case strings.HasPrefix(message, "http status "):
		return "图片下载失败，HTTP 状态码 " + strings.TrimPrefix(message, "http status ")
	case strings.HasPrefix(message, "image exceeds "):
		return "图片文件过大，超过当前大小限制"
	case strings.Contains(message, "unsupported image type"):
		return "不支持的图片类型"
	default:
		return message
	}
}
