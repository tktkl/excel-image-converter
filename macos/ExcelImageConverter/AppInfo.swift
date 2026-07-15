import Foundation

let appVersion = (Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String) ?? "1.0.16"
let appDisplayTitle = "Excel 图片转换器 v\(appVersion)"
