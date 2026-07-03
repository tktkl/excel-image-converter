import AppKit

private enum TaskStatus: String, Codable {
    case queued = "Queued"
    case running = "Running"
    case done = "Done"
    case failed = "Failed"

    var title: String {
        switch self {
        case .queued: return "等待中"
        case .running: return "转换中"
        case .done: return "已完成"
        case .failed: return "失败"
        }
    }
}

private struct TaskRow {
    let id: String
    let source: String
    var output: String
    var status: TaskStatus
    var detail: String
    let startedAt: Date
}

private struct HistoryRow: Codable {
    let id: String
    let source: String
    let output: String
    let status: TaskStatus
    let detail: String
    let endedAt: Date
}

private struct AppSettings: Codable {
    static let currentSettingsVersion = 2

    var settingsVersion: Int
    var keepURL: Bool
    var cellImageMode: String

    static let `default` = AppSettings(settingsVersion: currentSettingsVersion, keepURL: false, cellImageMode: "wps")

    enum CodingKeys: String, CodingKey {
        case settingsVersion = "settings_version"
        case keepURL = "keep_url"
        case cellImageMode = "cell_image_mode"
    }

    var normalized: AppSettings {
        if settingsVersion < AppSettings.currentSettingsVersion {
            return AppSettings.default
        }
        return AppSettings(settingsVersion: AppSettings.currentSettingsVersion, keepURL: keepURL, cellImageMode: AppSettings.normalizedMode(cellImageMode))
    }

    private static func normalizedMode(_ mode: String) -> String {
        switch mode.trimmingCharacters(in: .whitespacesAndNewlines).lowercased() {
        case "excel", "native", "office":
            return "excel"
        case "", "wps", "feishu", "lark", "dispimg", "feishu-wps", "wps-feishu":
            return "wps"
        default:
            return AppSettings.default.cellImageMode
        }
    }
}

private final class ActionTableView: NSTableView {
    var folderClickHandler: ((Int, Int) -> Void)?

    override func mouseDown(with event: NSEvent) {
        let point = convert(event.locationInWindow, from: nil)
        let row = self.row(at: point)
        let column = self.column(at: point)
        super.mouseDown(with: event)
        if event.clickCount == 1, row >= 0, (column == 1 || column == 3) {
            folderClickHandler?(row, column)
        }
    }
}

final class ConverterView: NSView, NSTableViewDataSource, NSTableViewDelegate {
    private let chooseButton = NSButton(title: "选择 Excel 文件", target: nil, action: nil)
    private let compatibilityControl = NSSegmentedControl(labels: ["兼容Excel", "兼容飞书/WPS"], trackingMode: .selectOne, target: nil, action: nil)
    private let keepLinkControl = NSSegmentedControl(labels: ["保留链接：否", "保留链接：是"], trackingMode: .selectOne, target: nil, action: nil)
    private let statusLabel = NSTextField(labelWithString: "准备就绪")
    private let tabView = NSTabView()
    private let runningTable = ActionTableView()
    private let historyTable = ActionTableView()
    private let openSourceButton = NSButton(title: "打开源文件", target: nil, action: nil)
    private let openOutputButton = NSButton(title: "打开转换后文件", target: nil, action: nil)
    private let clearHistoryButton = NSButton(title: "清理历史记录", target: nil, action: nil)

    private var runningRows: [TaskRow] = []
    private var historyRows: [HistoryRow] = []
    private let historyURL: URL
    private let settingsURL: URL
    private let conversionQueue: OperationQueue = {
        let queue = OperationQueue()
        queue.name = "ExcelImageConverter.ConversionQueue"
        queue.maxConcurrentOperationCount = 2
        return queue
    }()

    override init(frame frameRect: NSRect) {
        let appSupport = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask).first!
        let dir = appSupport.appendingPathComponent("ExcelImageConverter", isDirectory: true)
        try? FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        historyURL = dir.appendingPathComponent("history.json")
        settingsURL = dir.appendingPathComponent("settings.json")
        super.init(frame: frameRect)
        setupUI()
        loadSettings()
        loadHistory()
    }

    required init?(coder: NSCoder) {
        fatalError("init(coder:) has not been implemented")
    }

    private func setupUI() {
        wantsLayer = true
        layer?.backgroundColor = NSColor(calibratedRed: 0.94, green: 0.97, blue: 0.94, alpha: 1).cgColor
        registerForDraggedTypes([.fileURL])

        let title = NSTextField(labelWithString: "Excel IMAGE(URL) 转图片对象")
        title.font = NSFont.systemFont(ofSize: 22, weight: .bold)
        title.textColor = NSColor(calibratedRed: 0.05, green: 0.25, blue: 0.20, alpha: 1)

        let subtitle = NSTextField(labelWithString: "选择或拖入 .xlsx 文件，自动把 IMAGE(URL) 和图片链接转换为 Excel 图片对象。")
        subtitle.textColor = .secondaryLabelColor

        let author = NSTextField(labelWithString: "v\(appVersion)\npower by Thunder.Wu")
        author.font = NSFont.systemFont(ofSize: 12, weight: .regular)
        author.textColor = .secondaryLabelColor
        author.alignment = .right
        author.maximumNumberOfLines = 2

        chooseButton.target = self
        chooseButton.action = #selector(chooseFiles)
        chooseButton.bezelStyle = .rounded
        chooseButton.controlSize = .large

        keepLinkControl.selectedSegment = 0
        keepLinkControl.controlSize = .large
        keepLinkControl.target = self
        keepLinkControl.action = #selector(saveSettingsFromControls)

        compatibilityControl.selectedSegment = 1
        compatibilityControl.controlSize = .large
        compatibilityControl.target = self
        compatibilityControl.action = #selector(saveSettingsFromControls)

        let titleBlock = NSStackView(views: [title, subtitle])
        titleBlock.orientation = .vertical
        titleBlock.alignment = .leading
        titleBlock.spacing = 4

        let header = NSStackView(views: [titleBlock, author])
        header.orientation = .horizontal
        header.alignment = .bottom
        header.distribution = .fill
        header.spacing = 12
        titleBlock.setContentHuggingPriority(.defaultLow, for: .horizontal)
        author.setContentHuggingPriority(.required, for: .horizontal)

        let toolbar = NSStackView(views: [chooseButton, compatibilityControl, keepLinkControl, statusLabel])
        toolbar.orientation = .horizontal
        toolbar.alignment = .centerY
        toolbar.spacing = 12

        configure(table: runningTable)
        configure(table: historyTable)
        runningTable.folderClickHandler = { [weak self] row, column in
            guard let self else { return }
            let paths = self.selectedPaths(table: self.runningTable, row: row)
            self.reveal(path: column == 1 ? paths.source : paths.output)
        }
        historyTable.folderClickHandler = { [weak self] row, column in
            guard let self else { return }
            let paths = self.selectedPaths(table: self.historyTable, row: row)
            self.reveal(path: column == 1 ? paths.source : paths.output)
        }

        let runningScroll = NSScrollView()
        runningScroll.documentView = runningTable
        runningScroll.hasVerticalScroller = true
        runningScroll.hasHorizontalScroller = true

        let historyScroll = NSScrollView()
        historyScroll.documentView = historyTable
        historyScroll.hasVerticalScroller = true
        historyScroll.hasHorizontalScroller = true

        tabView.addTabViewItem(NSTabViewItem(viewController: tableController(title: "正在转换", view: runningScroll)))
        tabView.addTabViewItem(NSTabViewItem(viewController: tableController(title: "历史记录", view: historyScroll)))

        openSourceButton.target = self
        openSourceButton.action = #selector(openSelectedSource)
        openOutputButton.target = self
        openOutputButton.action = #selector(openSelectedOutput)
        clearHistoryButton.target = self
        clearHistoryButton.action = #selector(clearHistory)

        let bottom = NSStackView(views: [openSourceButton, openOutputButton, NSView(), clearHistoryButton])
        bottom.orientation = .horizontal
        bottom.spacing = 8
        bottom.distribution = .fill

        let root = NSStackView(views: [header, toolbar, tabView, bottom])
        root.orientation = .vertical
        root.alignment = .leading
        root.spacing = 14
        root.translatesAutoresizingMaskIntoConstraints = false
        addSubview(root)

        NSLayoutConstraint.activate([
            root.leadingAnchor.constraint(equalTo: leadingAnchor, constant: 18),
            root.trailingAnchor.constraint(equalTo: trailingAnchor, constant: -18),
            root.topAnchor.constraint(equalTo: topAnchor, constant: 18),
            root.bottomAnchor.constraint(equalTo: bottomAnchor, constant: -18),
            header.widthAnchor.constraint(equalTo: root.widthAnchor),
            tabView.widthAnchor.constraint(equalTo: root.widthAnchor),
            tabView.heightAnchor.constraint(greaterThanOrEqualToConstant: 420)
        ])
    }

    private func tableController(title: String, view: NSView) -> NSViewController {
        let vc = NSViewController()
        vc.title = title
        vc.view = view
        return vc
    }

    private func configure(table: NSTableView) {
        table.dataSource = self
        table.delegate = self
        table.usesAlternatingRowBackgroundColors = true
        table.doubleAction = #selector(doubleClickTable(_:))
        table.target = self
        for column in [
            ("source", "源文件", 220),
            ("sourceFolder", "位置", 48),
            ("output", "转换后文件", 220),
            ("outputFolder", "位置", 48),
            ("status", "状态", 100),
            ("detail", "详情", 360)
        ] {
            let col = NSTableColumn(identifier: NSUserInterfaceItemIdentifier(column.0))
            col.title = column.1
            col.width = CGFloat(column.2)
            table.addTableColumn(col)
        }
    }

    func numberOfRows(in tableView: NSTableView) -> Int {
        tableView == runningTable ? runningRows.count : historyRows.count
    }

    func tableView(_ tableView: NSTableView, viewFor tableColumn: NSTableColumn?, row: Int) -> NSView? {
        guard let id = tableColumn?.identifier.rawValue else { return nil }
        let text = NSTextField(labelWithString: value(tableView: tableView, column: id, row: row))
        text.lineBreakMode = .byTruncatingMiddle
        text.alignment = id.contains("Folder") ? .center : .left
        if tableView == historyTable, isHistoryMissing(row: row) {
            text.attributedStringValue = NSAttributedString(
                string: text.stringValue,
                attributes: [.strikethroughStyle: NSUnderlineStyle.single.rawValue, .foregroundColor: NSColor.secondaryLabelColor]
            )
        }
        return text
    }

    private func value(tableView: NSTableView, column: String, row: Int) -> String {
        if tableView == runningTable {
            let item = runningRows[row]
            return value(source: item.source, output: item.output, status: item.status, detail: item.detail, column: column)
        }
        let item = historyRows[row]
        return value(source: item.source, output: item.output, status: item.status, detail: item.detail, column: column)
    }

    private func value(source: String, output: String, status: TaskStatus, detail: String, column: String) -> String {
        switch column {
        case "source": return URL(fileURLWithPath: source).lastPathComponent
        case "sourceFolder": return "📁"
        case "output": return output.isEmpty ? "-" : URL(fileURLWithPath: output).lastPathComponent
        case "outputFolder": return output.isEmpty ? "" : "📁"
        case "status": return status.title
        default: return detail
        }
    }

    @objc private func chooseFiles() {
        let panel = NSOpenPanel()
        panel.title = "选择 Excel 工作簿"
        panel.allowedContentTypes = [.init(filenameExtension: "xlsx")!]
        panel.allowsMultipleSelection = true
        panel.canChooseDirectories = false
        if panel.runModal() == .OK {
            enqueue(paths: panel.urls.map { $0.path })
        }
    }

    override func draggingEntered(_ sender: NSDraggingInfo) -> NSDragOperation {
        .copy
    }

    override func performDragOperation(_ sender: NSDraggingInfo) -> Bool {
        let urls = sender.draggingPasteboard.readObjects(forClasses: [NSURL.self], options: nil) as? [URL] ?? []
        enqueue(paths: urls.map { $0.path })
        return true
    }

    private func enqueue(paths: [String]) {
        let files = paths.filter { $0.lowercased().hasSuffix(".xlsx") }
        guard !files.isEmpty else {
            statusLabel.stringValue = "没有可转换的 Excel 文件"
            return
        }
        for file in files {
            let row = TaskRow(id: UUID().uuidString, source: file, output: "", status: .queued, detail: "等待中", startedAt: Date())
            runningRows.append(row)
            convert(taskID: row.id)
        }
        runningTable.reloadData()
        statusLabel.stringValue = "已加入 \(files.count) 个文件"
    }

    private func convert(taskID: String) {
        let settings = currentSettings()
        conversionQueue.addOperation {
            let source: String? = DispatchQueue.main.sync {
                guard let rowIndex = self.rowIndex(taskID: taskID) else { return nil }
                self.update(rowIndex: rowIndex, status: .running, detail: "正在转换", output: "")
                return self.runningRows[rowIndex].source
            }
            guard let source else { return }
            let result = Self.runConverter(source: source, settings: settings)
            DispatchQueue.main.async {
                guard let rowIndex = self.rowIndex(taskID: taskID) else { return }
                switch result {
                case .success(let output):
                    self.update(rowIndex: rowIndex, status: .done, detail: "转换完成", output: output)
                case .failure(let error):
                    self.update(rowIndex: rowIndex, status: .failed, detail: error.localizedDescription, output: "")
                }
                self.addHistory(from: self.runningRows[rowIndex])
            }
        }
    }

    private static func runConverter(source: String, settings: AppSettings) -> Result<String, Error> {
        let exeURL = Bundle.main.url(forResource: "excel-image-converter-cli", withExtension: nil)!
        let process = Process()
        process.executableURL = exeURL
        var arguments = ["-quiet", "-workers", "4", "-compat", settings.cellImageMode]
        if settings.keepURL {
            arguments.append("-keep-url")
        }
        arguments.append(source)
        process.arguments = arguments
        let pipe = Pipe()
        process.standardOutput = pipe
        process.standardError = pipe
        var output = Data()
        let outputLock = NSLock()
        pipe.fileHandleForReading.readabilityHandler = { handle in
            let data = handle.availableData
            guard !data.isEmpty else { return }
            outputLock.lock()
            output.append(data)
            outputLock.unlock()
        }
        do {
            try process.run()
            process.waitUntilExit()
            pipe.fileHandleForReading.readabilityHandler = nil
            let remaining = pipe.fileHandleForReading.readDataToEndOfFile()
            outputLock.lock()
            output.append(remaining)
            let text = String(data: output, encoding: .utf8) ?? ""
            outputLock.unlock()
            if process.terminationStatus != 0 {
                return .failure(NSError(domain: "ExcelImageConverter", code: Int(process.terminationStatus), userInfo: [NSLocalizedDescriptionKey: text.trimmingCharacters(in: .whitespacesAndNewlines)]))
            }
            return .success(parseOutputPath(text: text, source: source))
        } catch {
            return .failure(error)
        }
    }

    private func rowIndex(taskID: String) -> Int? {
        runningRows.firstIndex { $0.id == taskID }
    }

    private static func parseOutputPath(text: String, source: String) -> String {
        for line in text.split(separator: "\n") {
            let trimmed = line.trimmingCharacters(in: .whitespaces)
            if trimmed.hasPrefix("Output:") {
                return trimmed.replacingOccurrences(of: "Output:", with: "").trimmingCharacters(in: .whitespaces)
            }
        }
        let url = URL(fileURLWithPath: source)
        return url.deletingPathExtension().path + "_pictures." + url.pathExtension
    }

    private func update(rowIndex: Int, status: TaskStatus, detail: String, output: String) {
        guard runningRows.indices.contains(rowIndex) else { return }
        runningRows[rowIndex].status = status
        runningRows[rowIndex].detail = detail
        if !output.isEmpty {
            runningRows[rowIndex].output = output
        }
        runningTable.reloadData()
        statusLabel.stringValue = detail
    }

    private func addHistory(from row: TaskRow) {
        historyRows.insert(HistoryRow(id: row.id, source: row.source, output: row.output, status: row.status, detail: row.detail, endedAt: Date()), at: 0)
        if historyRows.count > 500 {
            historyRows = Array(historyRows.prefix(500))
        }
        saveHistory()
        historyTable.reloadData()
    }

    @objc private func openSelectedSource() {
        open(path: selectedPaths().source)
    }

    @objc private func openSelectedOutput() {
        open(path: selectedPaths().output)
    }

    @objc private func clearHistory() {
        historyRows.removeAll()
        saveHistory()
        historyTable.reloadData()
        statusLabel.stringValue = "历史记录已清理"
    }

    @objc private func saveSettingsFromControls() {
        save(settings: currentSettings())
    }

    private func currentSettings() -> AppSettings {
        AppSettings(
            settingsVersion: AppSettings.currentSettingsVersion,
            keepURL: keepLinkControl.selectedSegment == 1,
            cellImageMode: compatibilityControl.selectedSegment == 1 ? "wps" : "excel"
        )
    }

    @objc private func doubleClickTable(_ sender: NSTableView) {
        let row = sender.clickedRow
        let column = sender.clickedColumn
        guard row >= 0, column >= 0 else { return }
        let paths = selectedPaths(table: sender, row: row)
        switch column {
        case 0: open(path: paths.source)
        case 1: reveal(path: paths.source)
        case 2: open(path: paths.output)
        case 3: reveal(path: paths.output)
        default: break
        }
    }

    private func selectedPaths() -> (source: String, output: String) {
        let table = tabView.indexOfTabViewItem(tabView.selectedTabViewItem!) == 0 ? runningTable : historyTable
        let row = table.selectedRow
        guard row >= 0 else { return ("", "") }
        return selectedPaths(table: table, row: row)
    }

    private func selectedPaths(table: NSTableView, row: Int) -> (source: String, output: String) {
        if table == runningTable, runningRows.indices.contains(row) {
            let item = runningRows[row]
            return (item.source, item.output)
        }
        if table == historyTable, historyRows.indices.contains(row) {
            let item = historyRows[row]
            return (item.source, item.output)
        }
        return ("", "")
    }

    private func open(path: String) {
        guard !path.isEmpty, FileManager.default.fileExists(atPath: path) else { return }
        NSWorkspace.shared.open(URL(fileURLWithPath: path))
    }

    private func reveal(path: String) {
        guard !path.isEmpty, FileManager.default.fileExists(atPath: path) else { return }
        NSWorkspace.shared.activateFileViewerSelecting([URL(fileURLWithPath: path)])
    }

    private func isHistoryMissing(row: Int) -> Bool {
        guard historyRows.indices.contains(row) else { return false }
        let item = historyRows[row]
        return !FileManager.default.fileExists(atPath: item.source) || (!item.output.isEmpty && !FileManager.default.fileExists(atPath: item.output))
    }

    private func loadHistory() {
        guard let data = try? Data(contentsOf: historyURL) else { return }
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601
        historyRows = (try? decoder.decode([HistoryRow].self, from: data)) ?? []
        historyTable.reloadData()
    }

    private func loadSettings() {
        let settings: AppSettings
        if let data = try? Data(contentsOf: settingsURL),
           let decoded = try? JSONDecoder().decode(AppSettings.self, from: data) {
            settings = decoded.normalized
        } else {
            settings = .default
        }
        keepLinkControl.selectedSegment = settings.keepURL ? 1 : 0
        compatibilityControl.selectedSegment = settings.cellImageMode == "wps" ? 1 : 0
    }

    private func save(settings: AppSettings) {
        if let data = try? JSONEncoder().encode(settings) {
            try? data.write(to: settingsURL)
        }
    }

    private func saveHistory() {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        if let data = try? encoder.encode(historyRows) {
            try? data.write(to: historyURL)
        }
    }
}
