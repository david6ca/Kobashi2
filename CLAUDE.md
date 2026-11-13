# ModelBridge

Local bridge that proxies inference for Codex and Claude Desktop.

The app's display name and release artifacts are `ModelBridge`, `ModelBridge.app`, and
`ModelBridge.exe`. The macOS bundle ID is `com.pomegram.modelbridge`.
Preserve existing state identifiers: `~/.kobashi`, startup registration keys, ports, and the `Kobashi`
client provider. Do not rename or duplicate saved logins or providers.

## UI feedback

Successful operations are silent: no provider-registration explanations or saved
notices. Bridge toggles show `Working...` in the affected row's existing status
slot, then `Active` or `Paused` for every client; never insert a progress banner
or shift the layout. Keep actionable errors visible. Settings uses label-only
Start at login and Close to system tray rows, without description text.
Appearance belongs in Settings with Auto, Light, and Dark; Auto follows the
system. Never put a theme toggle back in the main header.
Both light and dark themes use Pomegram berry pink (#E94F76) and coral orange
(#FF9166), with warm tinted neutrals and deeper shades for readable small text.
Functional colors stay semantic: green for active/connected, red for errors,
neutral gray for paused/disconnected. Never recolor status indicators to brand colors.
Preserve original brand artwork and system high-contrast colors.
The connected main screen fits its content; Settings and setup use a larger
fixed window. Changing a view preserves the window's top edge.
The macOS main window has a solid black upper half for the account and usage,
including the titlebar, with no orange tint. Its lower half is one edge-to-edge
native glass surface for the three client toggles. There is no inset panel.
Use one NSVisualEffectView (.sidebar, .behindWindow) for this region, with a
transparent window background. Do not stack NSGlassEffectView on it or force
the entire window to darkAqua. Paint black only in the account/header region.
NSPopover supplies the account menu material; its content stays transparent.
The app title, logo, and product version are absent from macOS content.
Settings is a borderless 32-point gear in a native unified toolbar with top
padding, on the same row as the native window controls. Body content uses
28-point side margins aligned with the native chrome's visible icon edges.
The connected main screen uses 400×259 content; Settings use 400×220 and local model setup uses 400×212; GitHub device setup uses 400×232.
The compact account area ends at 91 points. Client rows are 44 points tall,
with 20-point visible switch gaps. Glass top and bottom padding is 24 points
for icon tiles and 28 points for switches; client icons and the account icon
share a 28-point left margin.
The account dropdown arrow aligns with the body right edge; Connected sits
about 6 points from the arrow, The entire Connected label and chevron are one account-menu button; clicking
either opens the account menu. Hover padding encloses the arrow without moving
the label or chevron from their aligned positions.
Back uses the same 16-point regular SF Symbol
styling. Account information has no border. Native chrome does not mirror in RTL.
The account popover, Settings, and privacy window also use native materials,
respecting Reduce Transparency and Increase Contrast. Privacy Statement is
only in the macOS app menu. Main-screen errors temporarily replace usage details
without moving account information or toggles. Windows retains its existing layout.
Mouse selection has no thick row border; keyboard focus remains visible around
the relevant control.
Settings has no quit link; keep quitting available through the tray/menu bar.
The ModelBridge app and menu-bar marks use two outlined circles joined by a thin line. The menu-bar mark is a monochrome system template, with 1.5-point strokes for small-size legibility.
Client rows are ordered `Codex`, `Claude Code`, and `GitHub Copilot`, without `Bridge`.
Copilot preserves its enabled preference across launches. Its background watcher
registers the provider when the app becomes available and connects eligible
local sessions only while idle. Never interrupt a running session.
Startup shows `Starting...` until `/api/status` reports `ready` after saved
login and client configuration have loaded. Keep polling errors separate from
action errors: clear recovered transport failures, not actionable failures.

## NEVER kill the running Kobashi process

**ModelBridge (and the legacy Kobashi2/Kobashi apps) is the user's live connection to the AI they are talking to right now — including this session.** Killing it cuts the conversation mid-flight. There is no "quick restart"; the user is left with a dead client and no way to tell you so.

Do not run any of these, in any form:

- `pkill -f kobashi` / `pkill -f Kobashi.app` / `pkill -f kobashi-arm64`
- `killall Kobashi`
- `kill <pid>` against a kobashi or Kobashi.app process
- `launchctl kickstart -k` on anything kobashi-related

This holds even when it seems obviously necessary — installing a new build, freeing port 18921/18923, "just for a second". It is never worth it. **Ask the user to restart it themselves**, then wait.

The same applies to Claude Desktop (`/Applications/Claude.app`) when it is the surface the user is talking through.

### If a task seems to require a restart

Do the part that doesn't, then stop and hand off:

> The new build has been copied to `/Applications/ModelBridge.app`. Quit ModelBridge (or the legacy Kobashi app) from its menu, then open the installed ModelBridge app when you are ready.

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
go run ./cmd/build -target darwin   # Go + Cocoa → dist/ModelBridge.app
go run ./cmd/build -target windows  # Go + Win32 → dist/ModelBridge.exe
```

Version comes from `package.json` only. `cmd/build` stages sources, compiler caches, and intermediate files in a unique local temporary directory outside OneDrive. Versions are embedded through Go linker flags and platform metadata without changing source files. The temporary workspace is removed on success or failure. Go 1.27+ is required for builds; macOS also requires Xcode Command Line Tools. Released apps require no third-party runtime. macOS targets 13+.

Only the finished `ModelBridge.app` / `ModelBridge.exe` and `SHA256SUMS.txt` go back into `dist`. Keep all compiler output, temporary files, intermediate binaries, and unfinished app bundles outside the synced checkout. `KOBASHI_BUILD_TMPDIR` can select another local scratch directory; it must not be inside OneDrive or the checkout. Do not bypass the staging wrapper. macOS builds produce the app bundle directly without an archive tool. `SHA256SUMS.txt` lists each file in the app bundle and the Windows executable.

Copy the finished app to Applications before launching it: `open -a ModelBridge` matches by bundle id and can launch a build copy instead of the installed app.

Approved home spacing: b=20, e=20, f=24; h=28, g=8, r=20, s=24, t=28 at both top and bottom; j=12, y=24, z=26 and q=20. Other established 28-point outer margins remain unchanged.

Settings and Add account use matching 13-point regular navigation titles at the
same y=8 as the main account label. Their body layout uses 24-point side and
bottom margins; navigation glyphs align at x=24. Body labels, pickers and
main-screen client names/status use 12-point type. Client status is directly
below its name with a compact gap; use color rather than larger/bolder text.

The account menu has separate Add GitHub account and Add a local model entries.
GitHub setup only offers device authorization, without manual token input or
sign-in entry points. Code generation must not launch a browser. Ignore late
responses after the user cancels this flow. Keep saved accounts intact. Local setup only loads model choices and never starts GitHub auth.
The signed-out connect control opens the same menu. Both flows preserve the
active account until setup succeeds, and cancel returns to the saved source.

GitHub setup uses 400×232 content: the y=8 navigation header and a device-code
card with 24-point side margins and 16-point inner padding; code, Copy and
waiting status share the card. The card has a 24-point bottom margin; the GitHub URL is left-aligned. All GitHub auth states use the back chevron, never X.
Settings has no Done control; Back (including Escape) returns to the main page.
