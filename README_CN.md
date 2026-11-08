# Rove

[English](README.md) | 简体中文

Rove 是一个面向终端和编程代理的桌面工作空间，使用 Go 和 [MyGo](https://github.com/egoist/mygo) 构建。它通过 [herdr-core](https://github.com/herdrdev/herdr-core) 在后台运行终端，因此只要计算机保持运行，关闭窗口不会中断会话。

## 功能

- 使用空间、标签页和分屏组织终端，支持调整面板大小、切换焦点和缩放面板。
- 通过 SSH 连接计算机，使用远程终端、文件和 Git 更改。
- 查看代理状态和后台任务；代理完成任务或需要处理时，可通过系统通知和提示音提醒。
- 搜索终端输出和历史记录，浏览文件、预览更改并创建 Git 提交。
- 自定义主题、字体、外观和快捷键，通过可搜索的命令面板执行操作。
- 界面支持英语、简体中文、繁体中文和日语，默认跟随系统语言。

## 安装

从 [GitHub Releases](https://github.com/TengYangTuoHai/rove/releases) 下载适合系统的安装包：

| 平台 | 支持的架构 | 发布文件 |
| --- | --- | --- |
| macOS | Apple 芯片、Intel | `.dmg` |
| Windows | x86-64 | 安装程序 `.exe` |
| Linux | x86-64、ARM64 | `.deb` |

发布包包含应用、`herdr-core` 和所需原生库。其他 Linux 发行版可从源码构建便携包或 Arch 软件包。

### macOS

当前 macOS 构建使用临时签名，尚未经过 Apple 公证。安装后，macOS 可能阻止首次启动。如果你信任下载的应用，请先尝试打开，再进入 **系统设置 → 隐私与安全性 → 仍要打开** 并确认。具体操作可参阅 [Apple 说明](https://support.apple.com/en-us/102445)。

### 通知

macOS 的系统通知需要打包后的应用或通过 `go tool mygo dev` 运行，并在系统提示时授权。Linux 通过 `notify-send` 发送通知，需要图形桌面会话和对应软件包（Debian/Ubuntu 为 `libnotify-bin`，Fedora/Arch 为 `libnotify`）。Windows 使用原生通知；如有需要，请在 **设置 → 系统 → 通知** 中允许 Rove 发送通知。

## 开发

安装 Go 1.27.3 或更高版本以及 [herdr-core](https://github.com/herdrdev/herdr-core#install)，然后运行：

```sh
git clone https://github.com/TengYangTuoHai/rove.git
cd rove
go run .
```

Rove 会先查找应用自带的核心，再查找 `PATH` 中的 `hc` 或 `herdr-core`，最后查找核心的默认安装位置。除非已准备开发用应用包，否则 `go run .` 和 `go tool mygo dev` 需要预先安装核心。

运行检查：

```sh
go test ./...
go tool mygo i18n check
```

### 构建安装包

为指定平台构建：

```sh
go tool mygo build -platform darwin/arm64
```

按需将平台替换为 `darwin/amd64`、`windows/amd64`、`linux/arm64` 或 `linux/amd64`。构建全部五个目标：

```sh
./scripts/build-all.sh
```

Windows 上可使用 Bash 或 Git Bash。构建结果位于 `build/<os>-<arch>/`。构建钩子会下载固定版本的核心、校验 SHA-256，并将其加入应用包。构建需要网络；下载或校验失败时会停止打包。核心版本及校验值见 [cmd/prepare-hc/release.json](cmd/prepare-hc/release.json)。

MyGo 可构建 macOS `.app`、Windows 应用目录和 Linux 软件包。生成 DMG 需要 macOS；生成 Windows 安装程序需要 NSIS（MyGo 可在 Windows 上自动安装）。在其他系统上生成 Windows 安装程序时，请先安装 NSIS。分发时请保留完整应用目录或使用安装包，包括 `bin/` 目录及原生库；单独的 Windows `.exe` 无法满足运行要求。自带核心的发布版本不支持 Windows ARM64。

### 配置

Rove 默认将 `settings.json`、`keys.json` 和 `view.json` 保存在 `~/.config/rove`。设置 `XDG_CONFIG_HOME` 后，配置位于其 `rove` 子目录；设置 `ROVE_HOME` 可直接指定配置目录。设置和快捷键可在应用内修改。

| 操作 | macOS | Linux / Windows |
| --- | --- | --- |
| 命令面板 | `Cmd+Shift+P` | `Ctrl+Shift+P` |
| 新建空间 | `Cmd+N` | `Ctrl+Shift+N` |
| 新建标签页 | `Cmd+T` | `Ctrl+Shift+T` |
| 向右分屏 | `Cmd+D` | `Ctrl+Shift+O` |
| 向下分屏 | `Cmd+Shift+D` | `Ctrl+Shift+E` |
| 搜索终端 | `Cmd+F` | `Ctrl+Shift+F` |
| 设置 | `Cmd+,` | `Ctrl+,` |

### 翻译

翻译文件位于 `internal/i18n/locales`。修改后，重新生成 Go 访问代码并检查翻译：

```sh
go tool mygo generate
go tool mygo i18n check
go test ./...
```

提交翻译修改时，请一并提交更新后的 `catalog_gen.go`。

## 发布

[构建工作流](.github/workflows/build.yml) 会测试并构建五个受支持的目标。推送到 `master` 时发布开发预览版；推送与 `mygo.json` 中版本一致的 `v*` 标签时发布对应版本。手动运行工作流只上传构建产物，不发布 Release。

计划发布的版本保存在 `mygo.json`，可使用以下命令查看和更新：

```sh
go run ./cmd/version
go run ./cmd/version bump patch
go run ./cmd/version set 0.2.0-rc.1
```

## 许可证

Rove 使用 [MIT 许可证](LICENSE)。第三方资源和依赖保留各自的许可证及声明。
