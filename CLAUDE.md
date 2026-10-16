# Kobashi2

Local bridge that proxies inference for Codex and Claude Desktop.

The app's display name and release artifacts are `Kobashi2`, `Kobashi2.app`, and
`Kobashi2.exe`. Preserve compatibility identifiers: `~/.kobashi`, bundle ID
`com.xjin6.kobashi`, existing startup registration keys, ports, and the `Kobashi`
client provider. Do not rename or duplicate saved logins or providers.

## UI feedback

Successful operations are silent: no provider-registration explanations or saved
notices. Bridge toggles show `Working...` in the affected row's existing status
slot, then `Active` or `Paused` for every client; never insert a progress banner
or shift the layout. Keep actionable errors visible. Settings uses label-only
Start at login and Close to system tray rows, without description text.
Appearance belongs in Settings with Auto, Light, and Dark; Auto follows the
system. Never put a theme toggle back in the main header.
Both light and dark themes use purple accents and purple-tinted neutrals.
Functional colors stay semantic: green for active/connected, red for errors,
neutral gray for paused/disconnected. Never recolor status indicators purple.
Preserve original brand artwork and system high-contrast colors.
Main and Settings share one fixed window size; keep the version footer anchored.
Header-to-account, account-to-client, and client-to-version gaps are 16 logical
pixels. Main-screen errors replace the header title without moving the cards.
Mouse selection has no thick row border; keyboard focus remains visible around
the relevant control.
Settings has no quit link; keep quitting available through the tray/menu bar.
The Kobashi mark is two solid circles joined by a line, not a chevron.
Client labels are simply `Claude`, `Codex`, and `Copilot`, without `Bridge`.
Copilot preserves its enabled preference across launches. Its background watcher
registers the provider when the app becomes available and connects eligible
local sessions only while idle. Never interrupt a running session.
Startup shows `Starting...` until `/api/status` reports `ready` after saved
login and client configuration have loaded. Keep polling errors separate from
action errors: clear recovered transport failures, not actionable failures.

## NEVER kill the running Kobashi process

**Kobashi2 (and the legacy Kobashi app) is the user's live connection to the AI they are talking to right now — including this session.** Killing it cuts the conversation mid-flight. There is no "quick restart"; the user is left with a dead client and no way to tell you so.

Do not run any of these, in any form:

- `pkill -f kobashi` / `pkill -f Kobashi.app` / `pkill -f kobashi-arm64`
- `killall Kobashi`
- `kill <pid>` against a kobashi or Kobashi.app process
- `launchctl kickstart -k` on anything kobashi-related

This holds even when it seems obviously necessary — installing a new build, freeing port 18921/18923, "just for a second". It is never worth it. **Ask the user to restart it themselves**, then wait.

The same applies to Claude Desktop (`/Applications/Claude.app`) when it is the surface the user is talking through.

### If a task seems to require a restart

Do the part that doesn't, then stop and hand off:

> The new build has been copied to `/Applications/Kobashi2.app`. Quit Kobashi2 (or the legacy Kobashi app) from its menu, then open the installed Kobashi2 app when you are ready.

Editing files, building, and verifying logic against a copy are all fine — none of that needs the process down.

## Ports

- `18921` — Codex bridge (`~/.codex/config.toml` → `model_providers.kobashi`)
- `18923` — Claude gateway (`~/.claude/settings.json`, Claude Desktop managed-config)
- `18925` — Copilot app BYOK endpoint (OpenAI-compatible; provider API key `PROXY_MANAGED`)

Codex and Claude configurations are written by kobashi at startup and restored on exit. Enabling Copilot Bridge registers and verifies its dedicated provider through the running app's authenticated loopback API, independently of session selection. It selects the provider for the current local session only when idle. Only the app stores its placeholder provider credential; Kobashi never changes the app's GitHub login or environment. A port showing as busy is normal operation, not a stale process to clean up.

## Config injection

`App.injectCodex()` / `App.injectClaude()` inject; `App.restoreCodex()` / `App.restoreClaude()` undo.

`App.enableCopilot()` persists enablement independently of app/session availability.
Its watcher registers and verifies the provider when the app is available,
then connects newly observed local sessions and the selected local session
only while idle. Existing background sessions and explicit user provider/model
changes are left alone. There is no external app API for a global new-session
default: do not claim first-message routing is guaranteed when a session starts
immediately. Missing control files mean wait for the app, not disable the bridge.
Read the app database read-only and never interrupt a running session.
Skip provisional warmup sessions. Before a model switch or restoration, confirm
live runtime activity using `probe_session_liveness` with `recover: false`;
never use that probe to restart or recover a session. A database idle flag alone
is not sufficient. Verify unchanged provider and model registrations without
rewriting them on watcher ticks. Back off unready runtimes and continue checking
other eligible sessions. Explicit `idle_without_runtime` with `is_running: false`
is safe for dormant sessions; do not require a CLI to be running before their
first-message provider can be selected. Per-model wire protocols must not repeatedly change
the shared provider's default. Never replay an unconfirmed switch after a control disconnect.
Preserve unrelated providers, sessions, and user changes to the reserved
provider. Save original session selections in `~/.kobashi/copilot-restore.json`
before switching. Disabling or disconnecting must restore dependent idle local
sessions before stopping inference; if restoration is unsafe or unconfirmed,
keep the bridge enabled and report the error. Leave the provider registered
and preserve sessions the user has moved to another provider.

Codex inference uses `experimental_bearer_token = "PROXY_MANAGED"` on its
provider, with `requires_openai_auth = true` to keep the ChatGPT account visible
to Codex. Never replace a normal `auth.json` or inject global OpenAI API
credentials. Only recover an exact Kobashi-managed placeholder from its original
backup; preserve newer logins, user API keys, and organization login policies.

`restoreConfig()` exists because the old restore path deleted the user's entire `config.toml` when no `.bak` was present, taking their notify/marketplaces/plugins/mcp_servers config with it. Keep restore surgical — remove only the injected `model_provider` line and `[model_providers.*]` section, never the file.

## Session state

`~/.kobashi/session.json` (holds `github_token` — chmod 600).

Migrated from the legacy `~/.codex/ccb-session.json`, which was wrong twice over: kobashi's own state living in Codex's config dir, under a `ccb-` prefix left from when this was called "Copilot Bridge". `App.initialize()` handles the move; don't reintroduce writes under `~/.codex/`.

## Build

```bash
go run ./cmd/build -target darwin   # Go + Cocoa → dist/Kobashi2.app
go run ./cmd/build -target windows  # Go + Win32 → dist/Kobashi2.exe
```

Version comes from `package.json` only. `cmd/build` stages sources, compiler caches, and intermediate files in a unique local temporary directory outside OneDrive. Versions are embedded through Go linker flags and platform metadata without changing source files. The temporary workspace is removed on success or failure. Go 1.27+ is required for builds; macOS also requires Xcode Command Line Tools. Released apps require no third-party runtime. macOS targets 13+.

Only the finished `Kobashi2.app` / `Kobashi2.exe` and `SHA256SUMS.txt` go back into `dist`. Keep all compiler output, temporary files, intermediate binaries, and unfinished app bundles outside the synced checkout. `KOBASHI_BUILD_TMPDIR` can select another local scratch directory; it must not be inside OneDrive or the checkout. Do not bypass the staging wrapper. macOS builds produce the app bundle directly without an archive tool. `SHA256SUMS.txt` lists each file in the app bundle and the Windows executable.

Copy the finished app to Applications before launching it: `open -a Kobashi2` matches by bundle id and can launch a build copy instead of the installed app.
