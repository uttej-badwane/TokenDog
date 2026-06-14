# TokenDog Tray — Windows & Linux

A system-tray companion that shows your **Claude API spend** — today, this
month, lifetime — with the tokens/$ TokenDog clawed back alongside. It is the
Windows/Linux counterpart to the native macOS menu-bar app in
[`../macos/TokenDogBar`](../macos/README.md).

```
TokenDog ▾                      (Linux shows "$3.20 today" next to the icon;
────────────────────────         Windows shows the icon + a hover tooltip)
Spent today: $3.20
This month: $48.10
Lifetime: $120.50
(via Claude usage logs)
────────────────────────
TD saved lifetime: $10.74
TD share of bill: 8.2%
────────────────────────
Refresh now
Open full report…
────────────────────────
Quit TokenDog Tray
```

## How it works

It shells out to the TokenDog CLI:

```
td spend --json
```

`td spend` prices Claude Code's local usage logs natively — **no ccusage, no
network** — and the tray renders the result, refreshing every 60 seconds. Same
data contract as the macOS app. Everything stays on your machine.

## Platform notes

- **Linux** uses the AppIndicator/StatusNotifier protocol, so the dollar amount
  shows as text next to the icon (GNOME needs the *AppIndicator* extension;
  KDE, XFCE, Cinnamon, etc. work out of the box).
- **Windows** trays only show an icon, so the spend lives in the **hover
  tooltip** and the menu — the icon itself is always visible.

## Requirements

- The `td` CLI ≥ 0.12 on your `PATH` (the release that adds `td spend`). The
  tray also probes `~/go/bin`, `~/.local/bin`, Scoop/WinGet shims, and Homebrew
  paths, or set `TOKENDOG_BIN=/path/to/td`.
- A C toolchain to build (the tray library uses cgo): GCC/Clang on Linux,
  [TDM-GCC](https://jmeubank.github.io/tdm-gcc/) or MSYS2 mingw-w64 on Windows.
- Linux build headers: `libgtk-3-dev` and `libayatana-appindicator3-dev`
  (Debian/Ubuntu) or the equivalent for your distro.

## Build & run

Build natively on the target OS (cgo + system-tray libraries don't cross-compile
cleanly):

```sh
cd tray
go build -o tokendog-tray .     # tokendog-tray.exe on Windows
./tokendog-tray
```

Verify your setup without opening a tray:

```sh
./tokendog-tray --selftest
```

This runs the exact data path the tray uses (locate `td` → `td spend --json` →
decode) and prints the result.

## Autostart

No code is needed — use the OS mechanism:

- **Linux:** drop a `.desktop` file in `~/.config/autostart/` pointing at the
  built binary.
- **Windows:** put a shortcut to `tokendog-tray.exe` in
  `shell:startup` (Win+R → `shell:startup`).

## Layout

| File              | Responsibility                                        |
|-------------------|-------------------------------------------------------|
| `main.go`         | Tray UI, 60s poller, menu actions, `--selftest`       |
| `td.go`           | Locate `td`, run `td spend --json`, open full report  |
| `report.go`       | `td spend --json` contract + money formatting         |
| `icon/`           | Embedded tray icon (`go generate` regenerates it)     |

This is a **separate Go module** (`tokendog-tray`) so its cgo system-tray
dependency never touches the main `td` build, which stays CGO-free.
