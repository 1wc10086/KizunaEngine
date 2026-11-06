import Cocoa
import FlutterMacOS

class MainFlutterWindow: NSWindow {
  override func awakeFromNib() {
    let flutterViewController = FlutterViewController()
    let windowFrame = self.frame
    self.contentViewController = flutterViewController
    self.setFrame(windowFrame, display: true)
    // 游戏画布是 4:3，默认窗口按 1024x768 给一个不夸张的起步尺寸；
    // 最小尺寸限制住 640x480，避免拖到极小后 Flutter 布局溢出。
    self.setContentSize(NSSize(width: 1024, height: 768))
    self.contentMinSize = NSSize(width: 640, height: 480)

    RegisterGeneratedPlugins(registry: flutterViewController)

    super.awakeFromNib()
  }
}
