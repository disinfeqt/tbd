# TBD

TBD 用来把 X/Twitter 书签同步到本地，并下载书签里的媒体文件。

原始 repo：[0x1b2c/twitter-bookmarks-downloader](https://github.com/0x1b2c/twitter-bookmarks-downloader)

它由两部分组成：

- 本地 Go 服务：保存书签到 `bookmarks.db`，下载媒体到 `media/`
- Tampermonkey 用户脚本：在 `x.com/i/bookmarks` 页面捕获浏览器收到的书签数据并发送给本地服务

默认下载视频和图片。图片下载可以在页面左下角控制面板里关闭。

## 安装

### 1. 编译并启动本地服务

先安装 [Go](https://go.dev/dl/)，然后在项目目录运行：

```bash
go build -o tbd .
./tbd
```

服务会监听 `http://localhost:41008`。使用时保持这个终端窗口运行。

### 2. 安装 Tampermonkey 脚本

1. 安装 Tampermonkey 浏览器扩展。
2. 打开 Tampermonkey 的设置页，启用 **Allow User Scripts**。这一步必须打开，否则用户脚本不会正常运行。
3. 在 Tampermonkey 里创建新脚本。
4. 把 [sync-bookmarks.user.js](./sync-bookmarks.user.js) 的完整内容复制进去并保存。
5. 确认脚本处于启用状态。

## 使用

1. 先运行本地服务：

```bash
./tbd
```

2. 打开 X/Twitter 书签页：

```text
https://x.com/i/bookmarks
```

3. 页面左下角会出现 TBD 控制面板：

- `Start sync` / `Stop sync`：开始或停止自动向下滚动并同步书签
- `Auto-stop on`：遇到连续重复书签后自动停止
- `Auto-stop off`：忽略重复并继续向下滚动，适合深度重扫
- `Download videos: on/off`：控制是否下载视频
- `Download images: on/off`：控制是否下载图片

4. 下载结果：

- 数据库：`bookmarks.db`
- 媒体文件：`media/`
- 日志：`logs`

## 导出 @handles

导出已保存书签里的唯一作者用户名：

```bash
./tbd --export-handles
```

默认输出到 `handles.json`。如果想指定路径：

```bash
./tbd --export-handles --handles-output my-handles.json
```

## 设置

首次运行服务时会生成 `config.json`：

```json
{
  "media_dir": "media",
  "download_videos": true,
  "download_images": true
}
```

通常不需要手动编辑。视频和图片下载可以直接在页面控制面板里开关。

## 常见问题

### 页面左下角没有控制面板

检查：

- Tampermonkey 扩展已启用
- 脚本已启用
- Tampermonkey 设置里的 **Allow User Scripts** 已启用
- 当前页面是 `https://x.com/i/bookmarks`

### 有待下载数量，但暂时看不到新文件

大视频下载需要时间。新版服务会在终端显示开始下载和进度；如果没有这些日志，重启 `./tbd`。

### 更新脚本后没有生效

回到 Tampermonkey，把 [sync-bookmarks.user.js](./sync-bookmarks.user.js) 的最新内容重新复制进去并保存。

## 许可证

MIT。仅用于个人归档，请自行遵守 X/Twitter 的服务条款和内容版权要求。
