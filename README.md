# TBD

把 X/Twitter 书签同步到本地，并下载书签里的媒体文件。

原始 repo：[0x1b2c/twitter-bookmarks-downloader](https://github.com/0x1b2c/twitter-bookmarks-downloader)

## 工作原理

TBD 由两部分组成：

| 组件                    | 作用                                                                                               |
| ----------------------- | -------------------------------------------------------------------------------------------------- |
| 本地 Go 服务（`./tbd`） | 保存书签到 `bookmarks.db`，下载媒体到 `media/`，并在 <http://localhost:41008> 提供浏览与统计仪表盘 |
| Tampermonkey 用户脚本   | 在 `x.com/i/history` 页面捕获浏览器收到的书签数据并发送给本地服务                                  |

默认下载视频和图片，均可在页面控制面板里开关。仪表盘只监听 `127.0.0.1`，不会暴露给局域网。

## 快速开始

### 1. 编译并启动本地服务

先安装 [Go](https://go.dev/dl/)，然后在项目目录运行：

```bash
go build -o tbd ./cmd/tbd
./tbd
```

服务会监听 `http://localhost:41008`。使用时保持这个终端窗口运行。

### 2. 安装 Tampermonkey 脚本

1. 安装 Tampermonkey 浏览器扩展
2. 在 Tampermonkey 设置页启用 **Allow User Scripts**（必须打开，否则脚本不会运行）
3. 新建脚本，把 [web/sync-bookmarks.user.js](./web/sync-bookmarks.user.js) 的完整内容复制进去并保存
4. 确认脚本处于启用状态

### 3. 开始同步

保持 `./tbd` 运行，打开：

```text
https://x.com/i/history
```

页面左下角会出现 TBD 控制面板：

| 控件                       | 作用                                                                                         |
| -------------------------- | -------------------------------------------------------------------------------------------- |
| `Start sync` / `Stop sync` | 开始或停止自动向下滚动并同步书签                                                             |
| `Auto-stop` 开关           | 开启：连续几批都是重复书签时自动停止，适合日常增量更新。关闭：忽略重复继续滚动，适合深度重扫 |
| `Download videos` 开关     | 是否下载视频                                                                                 |
| `Download images` 开关     | 是否下载图片                                                                                 |

面板会自动跟随 X 的浅色/深色主题。同步结果保存在：

- 数据库：`bookmarks.db`
- 媒体文件：`media/`

## 浏览仪表盘

服务运行时，打开 <http://localhost:41008> 即可浏览整个书签库：

- **概览**：总数、作者数、含媒体比例、媒体下载进度
- **图表**：按月趋势、按小时/星期分布、Top 作者、媒体类型（每个图表都可切换为表格视图）
- **浏览**：全文/作者搜索、按作者筛选（点击 Top 作者条目）、本地媒体预览、跳转原推

## 命令行工具

除了默认的服务模式，`./tbd` 还支持以下一次性命令：

| 命令                        | 作用                                                                                                              |
| --------------------------- | ----------------------------------------------------------------------------------------------------------------- |
| `./tbd --export-handles`    | 导出已保存书签里的唯一作者用户名到 `handles.json`（用 `--handles-output <路径>` 指定输出文件）                    |
| `./tbd --repair-media`      | 根据数据库里保存的原始推文 JSON 修复视频媒体链接                                                                  |
| `./tbd --fix-deleted-media` | 以 `media/` 文件夹为准，删除媒体文件已被全部手动删除的书签记录                                                    |
| `./tbd --reset`             | 确认后删除 `bookmarks.db`（含 WAL 文件）。`media/` 里已下载的媒体**永远不会被删除**。执行前请先停止正在运行的服务 |

## 设置

首次运行服务时会生成 `config.json`：

```json
{
  "media_dir": "media",
  "download_videos": true,
  "download_images": true
}
```

通常不需要手动编辑——视频和图片下载可以直接在页面控制面板里开关。

## 项目结构

```text
cmd/tbd/            程序入口
internal/config/    设置文件读写
internal/logx/      终端输出
internal/store/     SQLite 与数据模型
internal/twitter/   X API 响应解析
internal/syncer/    书签同步逻辑
internal/download/  媒体下载 worker
internal/server/    HTTP API 与仪表盘
internal/export/    导出 @handles
web/                Tampermonkey 用户脚本
```

## 常见问题

**页面左下角没有控制面板？**

依次检查：Tampermonkey 扩展已启用、脚本已启用、Tampermonkey 设置里的 **Allow User Scripts** 已开启、当前页面是 `https://x.com/i/history`（旧地址 `https://x.com/i/bookmarks` 也支持）。

**有待下载数量，但暂时看不到新文件？**

大视频下载需要时间。服务会在终端显示开始下载和进度；如果没有这些日志，重启 `./tbd`。

**更新脚本后没有生效？**

回到 Tampermonkey，把 [web/sync-bookmarks.user.js](./web/sync-bookmarks.user.js) 的最新内容重新复制进去并保存。

## 许可证

MIT。仅用于个人归档，请自行遵守 X/Twitter 的服务条款和内容版权要求。
