# Kobashi2

Route Claude Code, OpenAI Codex, and the GitHub Copilot app through your GitHub Copilot subscription.

Kobashi2 is a local bridge that lets [Claude Code](https://www.anthropic.com/claude-code), [OpenAI Codex](https://openai.com/index/introducing-gpt-5-3-codex/), and the [GitHub Copilot app](https://github.com/github/app) use your existing GitHub Copilot subscription instead of separate Anthropic or OpenAI API keys. The Copilot app can keep its own GitHub login while using the account connected to Kobashi2 for model inference.

## Running the app

Use the [native build commands](#building) to generate `dist/Kobashi2.exe` on
Windows or `dist/Kobashi2.app` on macOS. Run the Windows executable directly.
On macOS, copy `Kobashi2.app` to Applications and launch it there, not from the
synced checkout.
Kobashi2 itself needs no additional runtime installation.

The desktop app is now named **Kobashi2**. Existing login, settings, ports, and
client integration identities stay unchanged: state remains in `~/.kobashi`,
and the Codex/Copilot custom provider is still named **Kobashi**. The macOS bundle
identifier and startup registration keys also retain their existing values, so
the rename does not create a second set of preferences or providers. When
replacing an older installation, keep only the renamed app at its installed
location and update an enabled login item's app path without launching both.

> **macOS first launch — "unidentified developer" warning**  
> Kobashi2 isn't signed with an Apple Developer ID ($99/yr — not worth it for a free tool), so macOS Gatekeeper blocks the first launch. Drag `Kobashi2.app` to `/Applications`, then run this **once** in Terminal to remove the quarantine flag:
> ```bash
> xattr -dr com.apple.quarantine /Applications/Kobashi2.app
> ```
> After that, double-click opens it normally — no warning, no trip through System Settings. (Alternative: right-click the app → **Open** → **Open** in the dialog — works but the `xattr` command is faster and sticks.)
>
> **After connecting:** open a **new terminal window** before running `codex` or `claude` so the injected env vars are picked up.

## How It Works

1. **Double-click** the app — a native Win32 window opens on Windows, or a Cocoa window on macOS
2. **Click Not Connected** — authorize via GitHub device flow; the status turns
   green and reads **Connected**. Click it to disconnect.
3. **Toggle the clients** — enable Claude, Codex, and/or Copilot
4. **Connect your clients** — Kobashi2 auto-configures `~/.claude/settings.json` and `~/.codex/`. Enabling Copilot registers its provider when the app is available and automatically connects eligible idle local sessions; no preselected session is required.

The Go bridge intercepts API requests on localhost and forwards them to the GitHub Copilot API using your Copilot token. It manages token refresh, config injection, cleanup, and Anthropic↔OpenAI format translation automatically. The application uses only native code and operating-system libraries. GitHub device authorization opens your default browser; token sign-in stays in the native window.

### Startup and window behavior

The window shows **Starting...** until the local bridge has restored its saved
login and client configuration. It does not offer sign-in while that state is
still loading. On macOS, initial connection refusals are retried for up to
10 seconds while the bridge process starts; a stopped bridge, port conflict,
or persistent failure still produces an error. Polling errors clear when the
connection recovers without clearing errors from user actions.

Open **Settings** from the main window to change these options:

- **Start at login** starts Kobashi2 when you sign in to your computer. Windows
  registers the current executable in your account's startup entries; macOS
  registers the app bundle as a user LaunchAgent. Keep the executable or app at
  its installed location. This option is off by default for new installations.
  Existing saved choices are preserved. Startup registration changes only when
  you explicitly change this setting, not when Kobashi2 launches.
- **Close to system tray** keeps Kobashi2 and its enabled bridges running when
  you close the window and is enabled by default. Use **Show Kobashi2** from the
  Windows system tray or macOS menu bar to reopen it. Turning this option off
  makes closing the window quit Kobashi2 and restore client configurations.
- **Appearance** offers **Auto**, **Light**, and **Dark**. Auto follows the system
  appearance, including later system changes; Light and Dark override it. The
  selection is saved across launches. Appearance lives only in Settings, not
  in the main header. The login and tray options use label-only rows.

Click the Settings icon again or **Done** to return to the main screen.

Login startup runs in the tray when close to tray is enabled; otherwise it opens
the window. Settings are saved in `~/.kobashi/settings.json` independently of your
GitHub connection and are preserved when you disconnect. The main window is
400 logical pixels wide. Main and Settings share a fixed 400 × 370 content area
on Windows and 400 × 390 on macOS, with a plain background and no decorative
footer animation. Header-to-account, account-to-client, and client-to-version
gaps are all 16 logical pixels. Switching pages or updating status never resizes
the window or moves the cards. Main-screen errors temporarily replace the header
title, beside the Settings action, rather than reserving empty space.
Mouse selection adds no thick row border; keyboard focus remains visible around
the relevant control. The app icon uses two solid circles joined by a line.
The macOS menu bar uses the same connection mark as a template icon, so macOS
controls its color for light, dark, and highlighted menu-bar states.
Both themes use purple accents; active/connected indicators stay green, errors
stay red, and paused/disconnected indicators stay neutral gray.
Windows scales the entire window,
including its frame, with display DPI and keeps all controls visible without scrolling.
Bridge toggles show **Working...** in place of their row status without moving
the layout. Successful actions show no extra notice; errors remain visible.
Settings uses label-only login/tray rows and a native Appearance selector.

To stop Kobashi2 while close to tray is enabled, select **Quit Kobashi2** in the
tray/menu-bar menu. Settings has no quit link. macOS also supports **Cmd-Q**,
**Kobashi2 > Quit Kobashi2**, and **Dock > Quit**. Quitting stops the bridge process
and runs configuration cleanup; closing to tray does not restart the bridges.
These desktop settings are available in both native applications.
`--no-open` remains a headless launch and does not create a Windows
tray icon or register startup.

## Requirements

- **macOS 13+** (Apple Silicon or Intel) or **Windows 10/11** (x64)
- **GitHub Copilot subscription** (Individual, Business, or Enterprise)
- **Claude Code**, **OpenAI Codex** CLI / VS Code extension, or **GitHub Copilot app** with BYOK support

## Features

- One-click GitHub OAuth device flow authentication
- Automatic Copilot token acquisition and refresh
- **Claude Bridge** — exposes an Anthropic-compatible API; remaps Claude Code's model IDs (e.g. `claude-opus-4-7`, `claude-sonnet-4-6[1m]`) to whatever Copilot actually supports; translates streaming + tool-use between Anthropic and OpenAI formats
- **Codex Bridge** — transparent passthrough proxy with an account-aware live OpenAI model picker, online capability checks on Codex startup, and safe 1M context defaults for models that support it
- **Copilot Bridge** — independent OpenAI-compatible endpoint with persistent enablement, deferred provider registration, and automatic connection of eligible idle local sessions, preserving the app's GitHub login
- Auto-injects configs (`~/.claude/settings.json`, `~/.codex/config.toml`) and restores them on disconnect without replacing Codex account login
- Auto-detects system HTTP(S) proxy — routes only Bridge's upstream traffic through it, leaving other apps untouched
- Consistent purple light/dark themes with system preference detection
- Opt-in login startup and close to system tray enabled by default
- Compact fixed-size window with a plain background
- Native Win32 and Cocoa clients with no third-party runtime
<!-- - Optional token-authenticated Server, remote Client, and local-network Server discovery -->

### GitHub Copilot app

Keep the Copilot app signed in with account **A**, and connect Kobashi2 to an
authorized Copilot account **B**, then enable **Copilot** in Kobashi2.
No selected session is required. If the app is closed, Kobashi2 keeps the bridge
enabled and configures it automatically when the app starts. Enablement is
preserved across Kobashi2 launches.
The bridge is off by default and does not require either of the other bridges.
When off, its status reads **Paused**; when on, it reads **Active**.

Enabling the bridge uses the running app's authenticated loopback control API to
add or update the **Kobashi** custom provider and register a compatible model.
It reads the provider and model back to verify that the app saved the
configuration and credential.
Its base URL is `http://127.0.0.1:18925/v1` (`18925 + N` with `--port-offset N`).
The app stores the placeholder API key `PROXY_MANAGED` in its own credential
store; Kobashi2 never sends its GitHub token to the app.

Kobashi2 keeps the session's current model if account B offers it; otherwise it
selects the first compatible chat model from B's catalog. It uses the model's
advertised Responses or Chat Completions protocol, including streaming and tool
calls. Provider-qualified model IDs saved by the app are matched to their wire
model IDs, preserving the selected model and supported reasoning effort across
bridge re-enables. A background watcher connects the selected local session
and newly observed local sessions when they are idle. Existing background
sessions, remote/cloud sessions, other providers, and the app's global
new-session defaults are not changed.

Provider registration does not depend on an open session: it also works while
viewing Settings, when no session is selected, or while a session is running.
In those cases the bridge stays enabled, and local sessions connect automatically
once eligible and idle. There is no need to turn the bridge off and on again.
If you manually change a session's provider, model, reasoning effort, or context
tier after it has been observed, Kobashi2 does not switch it back while enabled.
New sessions explicitly using another custom provider are also left alone.
Provisional warmup sessions are left untouched until the app finishes preparing
them. Immediately before a switch or restoration, Kobashi2 also asks the app for
live runtime activity with recovery disabled; a database idle flag alone is not
enough. Busy or unresponsive runtimes wait without being restarted.
An explicit `idle_without_runtime` result with `is_running: false` is also safe:
it lets a dormant session connect before its first message instead of waiting
for an already-running CLI. No liveness probe starts or recovers that runtime.
Registration is idempotent: unchanged provider credentials and model metadata
are verified without being rewritten on every watcher tick. Unready runtimes
are checked at most once every five seconds and do not block other eligible
sessions; individual models keep their own wire-protocol overrides without
changing the shared provider's default.

The installed app does not expose an external API for setting the global
new-session provider. Automatic connection uses supported per-session commands,
not database writes. A session that starts work immediately may complete its
first turn using its original provider; Kobashi2 waits until it is idle rather
than interrupting it. **Active** means the bridge is enabled, not that every
session is already routed through it.

An unavailable control API, an incompatible app version, or a registration
that cannot be verified produces an explicit error. Kobashi2 reads the app's
selection database read-only; provider and model changes go through the app.
A failed or unconfirmed session switch is reported without disabling an
already registered bridge, so a late switch can still use its endpoint.
If Copilot exits or disconnects before acknowledging a switch, Kobashi2 does not
retry that switch automatically or claim it succeeded. Wait until the session
is idle, then select **Kobashi** in its model picker to finish connecting. The
original selection remains saved for safe restoration; detailed transport
diagnostics are kept in `~/.kobashi/kobashi.log`.
If the app's runtime stops responding before a provider switch, Kobashi2 leaves
it untouched. A successful provider registration cannot repair an unresponsive
Copilot runtime or confirm that a first message reached it.

Check that **Kobashi** is selected in the session's model picker, then send a
message to verify inference. On Windows, you can also check catalog access at
the default endpoint below; adjust the port if using `--port-offset`:

```powershell
curl.exe --fail --silent --show-error -H "Authorization: Bearer PROXY_MANAGED" http://127.0.0.1:18925/v1/models
```

A model list confirms connectivity and catalog access, not inference. Connection
refusal means no listener is available at that address; `401` means the API key
is wrong, the bridge is disabled, or Kobashi2 is not connected.

Only requests routed through this provider use account B's model access and
quota. GitHub repository operations, MCP connections, and other app features
retain their existing authentication; cloud sessions do not use this local
endpoint. Selecting a built-in GitHub-hosted model still uses the app's normal
Copilot account. Kobashi2 does not alter the app's GitHub login or environment
variables. Before automatically switching a session, Kobashi2 saves its original
provider, model, reasoning effort, and context tier in
`~/.kobashi/copilot-restore.json`. Disabling Copilot Bridge or disconnecting
Kobashi2 first restores the local sessions that still use its provider, then
stops accepting requests. User-selected replacements are left unchanged.
If a dependent session is running, restoration fails, or its original selection
was not saved by an older version, the bridge stays enabled and reports an
error instead of breaking inference. Wait for the session to become idle, or
select a non-Kobashi provider in that session, then try again.
The provider remains registered; remove it in the app yourself if no longer
needed. Restarting Kobashi2 resumes an enabled bridge and re-registers its provider
as needed. Saved original selections survive the restart, and previously managed
sessions that the user moved to another provider are not switched back.

The model catalog comes from Kobashi2's connected account, independently of
Codex's model checks. Account B must be authorized for this use, and any code sent through it
must comply with applicable organization policies and subscription terms.
See [GitHub's BYOK setup documentation](https://docs.github.com/en/copilot/how-tos/github-copilot-app/use-byok-models).

<!--
### Server and Client

Kobashi2 defaults to the local GitHub-backed bridge. Both native applications also
support this connection:

```text
Codex / Claude / Copilot app -> local Kobashi2 Client -> remote Kobashi2 Server -> GitHub Copilot
```

On the **Server**, connect to GitHub and enable the bridges you want to share.
Open **Settings > Share this Kobashi2**, set the listen IP and port, and select
**Enable Server**. The default is `0.0.0.0:18924`. A blank access token generates
a private token on first use and keeps the existing token afterward. Use
**Copy token** to give it to trusted clients. To change the token or listen
address, disable Server, edit the fields, and enable it again.

On the **Client**, open **Settings > Connect to Server**. Use **Find Servers**
to choose a Server on the same IPv4 LAN, or enter its URL manually, such as
`http://192.168.1.10:18924`. Enter its access token and select **Connect**.
Disconnect an existing source first. Client mode does not sign in to GitHub or
send GitHub credentials; it automatically configures the local Codex and Claude
bridges as usual. Codex's separate ChatGPT login is preserved.

The Client gets its Codex model catalog and verified capabilities from the
Server, without running its own inference probes. A full Codex launch refreshes
the remote catalog; native picker changes take effect on the next full launch,
as in local mode. Streaming responses, encrypted message payloads, upstream
errors, and request cancellation pass through without replacing the selected
model or replaying started requests.

Server mode opens a separate inference-only listener. The control port `18922`
and local bridge ports `18921` / `18923` / `18925` remain loopback-only. Each remote
request requires the Server token; the native management token is separate.
Enabled bridge switches also govern remote access. **Disable Server** stops
sharing without disconnecting the local GitHub account. Disconnecting GitHub
also disables sharing.

HTTP exposes prompts, outputs, and the access token to the network, so use it
only on a trusted LAN. Clients accept plain HTTP only for literal private,
link-local, or loopback IP addresses. For hostnames or public access, use
**HTTPS** with a trusted certificate and a reverse proxy such as Caddy in front
of a loopback-bound Server. URL path prefixes are supported. Kobashi2 does not
disable certificate verification, follow remote redirects, or expose its
management endpoints through the Server.

Discovery uses mDNS/DNS-SD (`_kobashi._tcp.local`, UDP `5353`) on active IPv4 LAN
interfaces. It does not broadcast credentials, connect automatically, or work
across routers, VPN boundaries, or networks that block multicast. If no Server
is found, use a manual URL. Only open the configured Server TCP port to trusted
clients; do not forward the control port.

Server settings are stored privately in `~/.kobashi/server.json`. The selected
Client connection and token are stored privately in `~/.kobashi/session.json`.
Both survive an application restart. All remote requests use the Server's
Copilot account and quota; a Client token is not a GitHub identity or a separate
Copilot subscription. Check the applicable subscription terms and organization
policy before sharing access.
-->

### Codex login and plugin store

Sign in to Codex with ChatGPT to use its plugin store. This account is separate
from Kobashi2's GitHub connection. Kobashi2 preserves `~/.codex/auth.json` and uses
a non-secret provider-level bearer for inference, so ChatGPT credentials are not
sent to Kobashi2 or GitHub Copilot. It does not set global OpenAI API credentials.
The provider keeps `requires_openai_auth = true` so Codex can display the saved
ChatGPT account; the explicit provider bearer still controls inference requests.

Kobashi2 recovers a managed placeholder login from its saved `auth.json.bak`
without replacing a newer login. If no usable backup exists, the app asks you to
sign in to Codex again. User API keys, `CODEX_API_KEY`, and organization login
policies are preserved; the plugin store requires Codex's active account to be
ChatGPT rather than API key auth. An expired or revoked login may also require
sign-in. Fully quit and reopen Codex yourself to load changed configuration.

### Codex model compatibility

Kobashi2 refreshes the account's Copilot model directory and performs one online
validation round when a Codex client starts. Fully quit Codex and reopen it to
force a fresh directory read and validation, even when the previous capability
cache is unchanged. Closing a window alone may leave the client process running.
Repeated picker reads and normal conversation turns reuse that startup snapshot;
there is no timed remote discovery, inference recheck, or background retry loop.

macOS and Windows use the same discovery, validation, caching, and request-adaptation
code. A read-only local process watcher checks client process IDs and creation
times every five seconds (`ps` on macOS, Win32 process APIs on Windows); these
checks do not contact a model or consume tokens. GUI helpers and child servers are
counted with their parent, and diagnostic queries such as `codex debug models`
never count as a startup. The first bridge request can bootstrap discovery before
the watcher sees the client, without causing a second validation round.

On first setup (or an account switch with no verified cache), Kobashi2 prepares the
verified directory before injecting the Codex configuration. Open Codex after
this preparation completes. On subsequent launches, the previous verified
snapshot remains available while the new startup check runs; a network timeout
does not revoke an earlier successful ordinary-inference check.

The native Codex app server reads `model_catalog_json` at process startup. It does
not reload its picker when that file or `config.toml` is rewritten. Therefore,
new models, removed models, and new effort options found by a startup round are
saved for the **next full Codex launch**; `/v1/models` reflects them as checks
finish. If Codex was already open during first setup, fully quit and reopen it
after preparation. Kobashi2 never restarts the client or interrupts a live turn.

New models are checked with a small, real streaming tool-call request
before being published to the directory or `/v1/models`. HTTP 200 alone is
insufficient: the check must finish successfully with a valid tool call. There is
no model-name allowlist, compatibility exception list, or silent model substitution.

The same refresh reads the installed Codex client's native catalog. Ultra is a
client delegation mode: its exact native model metadata and underlying API effort
must both be supported. Models declaring Ultra or Multi-Agent V2 also require a
real agent-message roundtrip: the API must generate a message and return its
complete original payload when replayed. Until that check succeeds, Kobashi2
publishes V1 delegation without Ultra and retains the model's normal API efforts.
A newly added native model or Ultra mode is discovered and verified automatically.
A Copilot model absent from the native catalog can still
appear with its advertised API efforts; Kobashi2 cannot invent a client mode that
the installed Codex does not declare.

Parameter compatibility is learned from explicit upstream validation responses.
Supported alternatives for reasoning context, effort, summary, and text verbosity
are applied without changing the selected model, conversation, or tools.
If the upstream API explicitly rejects `service_tier`, Kobashi2 omits that field
and caches the rejection for that model and account; it never substitutes another
service tier. HTTP validation errors can be retried before returning the response.
Streaming failures remain visible and update the rules for subsequent requests,
without replaying an already-started response. Multiple optional-field changes
can be negotiated with at most three compatibility retries,
separate from the existing single authentication retry. Ambiguous errors and
changes to user data, tools, model IDs, or token budgets are never guessed.

Streaming failures preserve the upstream error code and message, including when
Copilot sends those details after an empty `response.failed` event. Failures are
held across SSE keepalives so a following error still reaches the client.
If the provider supplies no message, the bridge reports that explicitly rather
than forwarding an unexplained failure. It never turns a failed stream into a
successful completion or replays an already-started response. Failures are
logged in `~/.kobashi/kobashi.log`. Encrypted agent content is forwarded unchanged.
If Copilot cannot decrypt it, Kobashi2 reports the actual failure rather than
discarding the message or presenting ciphertext as readable conversation text.
An agent decryption failure also invalidates that model's cached Ultra/V2
capability without removing ordinary inference. Explicit Codex feature overrides
can still force V2; Kobashi2 does not change those overrides.
Use a new V1 conversation without Ultra when this happens. Changing modes does
not remove encrypted messages from an existing conversation's history.

Verified capabilities and learned parameter rules are cached privately in
`~/.kobashi/codex-model-capabilities.json`, scoped to the account and a fingerprint
of upstream and native capabilities. A client restart explicitly revalidates all
discovered models and can remove old learned limitations when a provider adds
support. The running session keeps its verified list without a time-based expiry.
Real model-access failures still remove a model from the published directory and
`/v1/models` immediately (the already-open native picker updates on its next launch).
Real request validation errors still teach compatible parameters.

Each startup round uses at most two concurrent checks, a 45-second timeout per
request, and a 1,024-token output limit. These are real API requests and consume
some usage, including bounded parameter-negotiation retries. A transient failure
does not schedule further checks in the background; restart Codex to retry.
Exiting the clients stops queued validation work. Account changes and newer
startups invalidate late results from older rounds.

## Building

Install Go 1.27 or later to build from source. Windows builds need only Go;
macOS builds also need Xcode Command Line Tools.
No third-party Go modules are required.

On Windows, run from PowerShell:

```powershell
go run .\cmd\build -target windows
```

On macOS:

```bash
go run ./cmd/build -target darwin
```

Both commands copy the build sources into a unique local temporary directory
outside the checkout and OneDrive. Compilation, icon generation, and packaging
happen only in that copy, which is removed after success or failure.
The app version comes only from `package.json`, which contains product metadata,
and is embedded in the binaries
and platform metadata without modifying source files. macOS packaging builds
both Apple Silicon and Intel bridge binaries and a universal Cocoa executable.

After a successful build, only `dist/Kobashi2.app` (macOS) or `dist/Kobashi2.exe`
(Windows) and `dist/SHA256SUMS.txt` are published back to the checkout. The existing
release for the other platform is preserved. `SHA256SUMS.txt` lists the Windows
executable and each file inside the macOS app bundle. The complete bundle is
copied and verified before replacing an existing app. If artifact or checksum
publication fails, the previous release is restored; if rollback itself fails,
the error identifies the retained recovery directory. This is not a crash-atomic
transaction across both files. No intermediate binaries or dated build
directories are left in `dist`.
If an existing executable is in use, leave it running and use `-output <directory>`
to save the finished package elsewhere. The build never stops a running app.

The build uses the OS temporary directory by default. To use a different local
disk, set `KOBASHI_BUILD_TMPDIR` to a directory outside OneDrive and the checkout.
Cloud-backed temporary paths, including symlinks into OneDrive, are rejected.
Go compiler caches and intermediate files stay in the temporary workspace.
The final app path is printed when the build completes. macOS builds produce
the app bundle directly and require no archive tool or extractor.

## Development checks

Run `go test ./...` for native bridge, compatibility, configuration, transport,
and packaging checks. On Windows with a graphical login session, this also
exercises native controls, fixed DPI-scaled sizing, persisted settings, and
close-to-tray/quit behavior using an isolated home and different loopback ports.
Run `go run ./cmd/kobashi --no-open` for headless development. `--home <directory>` and
`--port-offset <offset>` isolate client configuration and ports;
`--no-startup-registration` leaves OS startup registration unchanged.

Windows native stream/catalog checks use `KOBASHI_TEST_CODEX_EXE` and
`go test .\internal\bridge -run '^TestInstalledCodex(StreamingFailureDetails|CopilotDelegation)$'`.
Live delegation checks also require `KOBASHI_TEST_COPILOT_SESSION` (the existing
session file) and `KOBASHI_TEST_COPILOT_MODEL`. They consume Copilot usage and use
a temporary home; the Codex installation must include `codex-code-mode-host.exe`.

The bridge checks include the compatibility fixtures in
`internal/bridge/testdata/parity.json`. On macOS with Xcode Command Line
Tools and a graphical login session, run `go run ./cmd/build -test-mac` for the native window
lifecycle checks. These compile the real app delegate with an isolated test entry
point and use a disposable child process instead of the bridge; they do not use
your credentials, modify your client configuration, or bind the bridge ports.
The runner selects the Xcode default Swift toolchain. Set
`KOBASHI_TEST_SCREEN_ID` to the target numeric macOS display ID to place every
test window on that screen before showing it, without interrupting a fullscreen
app on another display. A missing target display fails the check rather than
falling back to another screen.

Run `go run ./cmd/build -test-mac-artwork` for offscreen brand-artwork checks
without opening any windows. These compare tinted and original image orientation
in light and dark appearances, using both flipped and unflipped drawing coordinates.
The full native suite includes these checks before opening its test windows.

Native window checks first cover startup connection retries, saved-login readiness,
polling error recovery, and visible real failures using an isolated URL session.
They then send mouse events through an isolated app event loop to
verify that all three bridge cards toggle off and back on when clicked on the switch
or card body. Toggle requests use a fake service without changing client settings.
The lifecycle checks exercise the fixed window size, close-to-tray setting, menu-bar
restore entry, repeated restoration, minimization, and the termination callback.
Actual menu-bar/Dock activation, Cmd-Q/Dock Quit delivery, and live
inference continuity still need a hands-on macOS smoke test. Never stop a running
Kobashi2 instance from an automated test.

The opt-in `TestInstalledCopilotLiveChat` checks the real app's first-message
path. Set `KOBASHI_TEST_COPILOT_LIVE_SESSION` to a newly created, unused,
unselected disposable local chat ID. The check requires a saved authorized
Kobashi2 account, an unused default Copilot port, and no existing sessions
dependent on Kobashi2. It connects only that chat, sends `hi` through the app,
requires a real reply through the bridge, and restores the original model.
The first inference request must start within 15 seconds of sending `hi`;
an eventual reply after a stalled send does not pass. Set
`KOBASHI_TEST_COPILOT_DORMANT=1` to additionally require a confirmed
`idle_without_runtime` starting state, separating cold startup from a live
provider replacement.
It never stops a running Kobashi2 or switches the user's selected chat. Archive
the disposable chat after the check.

If dispatch stalls, compare a separate unused chat using the app's built-in
provider with Kobashi2 disabled. Distinguish time spent inside the app before
`session.send` from bridge request and upstream response time. A slow built-in
control does not waive the 15-second bridge check or establish a specific
extension as the cause.

For a safe hands-on check while your real bridge stays running, run
`go run ./cmd/build -test-mac -smoke`. This opens **Kobashi2 Lifecycle Test (no bridge)** with a
different bundle identifier, a counter, and a disposable child's PID. Close its
window, wait, and select **Show Kobashi2** from its menu-bar icon: the counter
should advance and the PID should stay the same. Repeat after minimizing and
hiding. Quit **only the test
app** with Cmd-Q; rerun to check Dock > Quit. The terminal confirms its child has
stopped. This checks native event delivery, not real inference or client-config
cleanup. The temporary test bundle is removed when the test exits.

## License

MIT
