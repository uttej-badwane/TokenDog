# TokenDog Bar — macOS menu bar

A tiny menu-bar app that shows your **Claude API spend** at a glance — today,
this month, lifetime — with the tokens/$ TokenDog clawed back shown alongside.
Native to TokenDog and dependency-free.

```
🐶 $3.20 today
────────────────────────
Spent today        $3.20
This month         $48.10
Lifetime           $120.50
(via Claude usage logs)
────────────────────────
TD saved today     $0.86
TD saved lifetime  $10.74
TD share of bill   8.2%
────────────────────────
Refresh now            ⌘R
Open full report…      ⌘O
Launch at login   ✓
Quit TokenDog Bar      ⌘Q
```

## How it works

The app shells out to the TokenDog CLI:

```
td spend --json
```

`td spend` reads Claude Code's local usage logs
(`~/.claude/projects/**/*.jsonl`) and prices them natively with TokenDog's
per-model rates — **no ccusage, no npx, no network access**. It then joins that
with TokenDog's own savings history. The menu bar just renders the JSON and
refreshes every 60 seconds (and whenever you open the menu).

Everything stays on your machine: the app reads only local files and runs only
the local `td` binary.

## Requirements

- macOS 13 (Ventura) or newer
- The `td` CLI on your `PATH` (`brew install uttej-badwane/tokendog/tokendog`).
  Needs TokenDog **≥ 0.12** (the release that adds `td spend`). The app also
  probes `/opt/homebrew/bin`, `/usr/local/bin`, and `~/go/bin`, or you can set
  `TOKENDOG_BIN=/path/to/td`.

## Build & install

```sh
cd macos/TokenDogBar
./build.sh --install     # builds TokenDogBar.app and copies it to /Applications
```

Then launch **TokenDog Bar** from Spotlight or `/Applications`, and toggle
**Launch at login** from the menu so it starts with your Mac.

Other `build.sh` modes:

```sh
./build.sh               # build into ./dist only
./build.sh --run         # build and launch
```

The build uses `swiftc` directly, so it works with just the Xcode **Command
Line Tools** (`xcode-select --install`) — full Xcode is not required.

### Verify your setup

```sh
./dist/TokenDogBar.app/Contents/MacOS/TokenDogBar --selftest
```

This runs the exact data path the menu bar uses (locate `td` → `td spend --json`
→ decode) and prints the result, without opening the UI. Use it to confirm your
`td` install is compatible.

## Development

With **full Xcode** installed you can use SwiftPM directly:

```sh
swift run        # build and launch from source
```

(On Command-Line-Tools-only machines SwiftPM's manifest runner can be broken;
use `./build.sh` there instead — it compiles the same sources with `swiftc`.)

Source layout (`Sources/TokenDogBar/`):

| File              | Responsibility                                            |
|-------------------|-----------------------------------------------------------|
| `main.swift`      | Entry point; `.accessory` activation; `--selftest`        |
| `AppDelegate.swift` | Status item, menu, 60s poller, actions                  |
| `TDClient.swift`  | Locate `td`, run `td spend --json`, open full report      |
| `SpendReport.swift` | Codable for the `td spend --json` contract + money fmt  |
| `LoginItem.swift` | Launch-at-login via `SMAppService` (macOS 13+)            |

## Graceful degradation

- **No Claude logs** (`spend.available == false`) → the bar shows lifetime
  savings (`🐶 $10.74 saved`) and the menu notes no logs were found.
- **`td` not found** → the bar shows `🐶 td?` and the menu links the install
  command.
