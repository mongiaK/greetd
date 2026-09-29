# mgreeter

用 Go + Bubble Tea 写的 greetd 登录界面：大号点阵时钟、头像卡片、会话胶囊、Catppuccin Mocha 配色，状态栏显示主机名、wifi 和电量。

## 文件

| 文件 | 作用 |
| --- | --- |
| `main.go` | Bubble Tea 界面：模型、按键处理、渲染、主题 |
| `greetd.go` | greetd IPC 客户端（只用标准库） |
| `sessions.go` | 扫描 `wayland-sessions/*.desktop`，记住上次的用户和会话 |
| `sysinfo.go` | 主机名、电量（`/sys/class/power_supply`）、wifi 名（`iwgetid` 或 `nmcli`） |

## 构建

需要 Go 1.24.2 及以上（`bubbles` v1.0.0 的要求）。

```bash
go mod tidy
go build -o mgreeter .
```

## 先在普通终端里试效果

没有 `GREETD_SOCK` 时自动进入演示模式：不会真的登录，密码输入 `demo` 算成功，`Ctrl+C` 退出。

```bash
./mgreeter --state /tmp/state.json
```

## 安装

```bash
sudo install -m755 mgreeter /usr/local/bin/
sudo install -d -o greeter -g greeter /var/cache/mgreeter
```

`/etc/greetd/config.toml`：

```toml
[terminal]
vt = 1

[default_session]
command = "cage -s -m last -- foot --config=/etc/greetd/foot.ini /usr/local/bin/mgreeter"
user = "greeter"
```

`/etc/greetd/foot.ini`：

```ini
[main]
font=JetBrainsMono Nerd Font:size=15, Noto Sans CJK SC:size=15
pad=0x0

[cursor]
style=beam
blink=yes

[colors]
background=1e1e2e
foreground=cdd6f4
```

依赖（Arch）：`greetd cage foot ttf-jetbrains-mono-nerd noto-fonts-cjk`。

## 按键

| 按键 | 作用 |
| --- | --- |
| `Enter` | 用户名 → 密码 → 登录 |
| `↑` `↓` | 在用户名和密码之间切换 |
| `Tab` `Shift+Tab` | 切换会话 |
| `F10` `F11` `F12` | 睡眠、重启、关机（调用 `systemctl`） |

## 说明

- 界面必须跑在会响应 OSC 11 / 光标位置查询的终端里（foot 会响应）。在裸 Linux 控制台上启动会因为依赖库的探测多等约 5 秒。
- 电量和 wifi 每 30 秒刷新一次。没有电池或读不到 wifi 名时对应项不显示。
- 会话只扫描 Wayland 的 `.desktop` 文件，找不到时回退到 `/bin/sh`。
- 登录成功后 greeter 会立刻退出，greetd 随后才会启动会话，这是正常流程。
- 不要在使用中的机器上直接重启 greetd 来测试，先切到别的 TTY，或者直接重启机器。
- `--state` 可以改记住用户和会话的文件位置，需要让 `greeter` 用户有写权限。
