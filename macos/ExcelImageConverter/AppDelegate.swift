import AppKit

final class AppDelegate: NSObject, NSApplicationDelegate {
    private var window: NSWindow!
    private var rootView: ConverterView!

    static func installMainMenu() {
        let mainMenu = NSMenu()
        let appMenuItem = NSMenuItem()
        let appMenu = NSMenu()
        appMenu.addItem(NSMenuItem(
            title: "退出 Excel 图片转换器",
            action: #selector(NSApplication.terminate(_:)),
            keyEquivalent: "q"
        ))
        appMenuItem.submenu = appMenu
        mainMenu.addItem(appMenuItem)
        NSApp.mainMenu = mainMenu
    }

    func applicationDidFinishLaunching(_ notification: Notification) {
        showMainWindow()
        DispatchQueue.main.async { [weak self] in
            self?.showMainWindow()
        }
    }

    func applicationDidBecomeActive(_ notification: Notification) {
        if window == nil || !window.isVisible {
            showMainWindow()
        }
    }

    func applicationShouldHandleReopen(_ sender: NSApplication, hasVisibleWindows flag: Bool) -> Bool {
        showMainWindow()
        return true
    }

    private func showMainWindow() {
        NSApp.setActivationPolicy(.regular)
        NSApplication.shared.applicationIconImage = NSImage(named: "AppIcon")

        if window == nil {
            rootView = ConverterView()
            window = NSWindow(
                contentRect: NSRect(x: 0, y: 0, width: 1120, height: 720),
                styleMask: [.titled, .closable, .miniaturizable, .resizable],
                backing: .buffered,
                defer: false
            )
            window.title = appDisplayTitle
            window.isReleasedWhenClosed = false
            window.center()
            window.minSize = NSSize(width: 980, height: 640)
            window.contentView = rootView
        }

        window.deminiaturize(nil)
        window.setIsVisible(true)
        window.makeKeyAndOrderFront(nil)
        window.orderFrontRegardless()
        NSApp.activate(ignoringOtherApps: true)
    }

    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool {
        true
    }
}
