# Kobashi

Route Claude Code, OpenAI Codex, and the GitHub Copilot app through your GitHub Copilot subscription.

Kobashi is a local bridge that lets [Claude Code](https://www.anthropic.com/claude-code), [OpenAI Codex](https://openai.com/index/introducing-gpt-5-3-codex/), and the [GitHub Copilot app](https://github.com/github/app) use your existing GitHub Copilot subscription instead of separate Anthropic or OpenAI API keys. The Copilot app can keep its own GitHub login while using the account connected to Kobashi for model inference.

## Running the app

Use the [native build commands](#building) to generate `dist/Kobashi.exe` on
Windows or `dist/Kobashi.app` on macOS. Run the Windows executable directly.
On macOS, copy `Kobashi.app` to Applications and launch it there, not from the
synced checkout.
Kobashi itself needs no additional runtime installation.

> **macOS first launch — "unidentified developer" warning**  
> Kobashi isn't signed with an Apple Developer ID ($99/yr — not worth it for a free tool), so macOS Gatekeeper blocks the first launch. Drag `Kobashi.app` to `/Applications`, then run this **once** in Terminal to remove the quarantine flag:
> ```bash
> xattr -dr com.apple.quarantine /Applications/Kobashi.app
> ```
> After that, double-click opens it normally — no warning, no trip through System Settings. (Alternative: right-click the app → **Open** → **Open** in the dialog — works but the `xattr` command is faster and sticks.)
>
> **After connecting:** open a **new terminal window** before running `codex` or `claude` so the injected env vars are picked up.

## How It Works

1. **Double-click** the app — a native Win32 window opens on Windows, or a Cocoa window on macOS
2. **Click Not Connected** — authorize via GitHub device flow; the status turns
   green and reads **Connected**. Click it to disconnect.
3. **Toggle the bridges** — enable Claude Bridge, Codex Bridge, and/or Copilot Bridge
4. **Connect your clients** — Kobashi auto-configures `~/.claude/settings.json` and `~/.codex/`. Add Kobashi as a model provider in the Copilot app as described below.

The Go bridge intercepts API requests on localhost and forwards them to the GitHub Copilot API using your Copilot token. It manages token refresh, config injection, cleanup, and Anthropic↔OpenAI format translation automatically. The application uses only native code and operating-system libraries. GitHub device authorization opens your default browser; token sign-in stays in the native window.

### Startup and window behavior

Open **Settings** from the main window to change these options:

- **Start at login** starts Kobashi when you sign in to your computer. Windows
  registers the current executable in your account's startup entries; macOS
  registers the app bundle as a user LaunchAgent. Keep the executable or app at
  its installed location. This option is off by default for new installations.
  Existing saved choices are preserved. Startup registration changes only when
  you explicitly change this setting, not when Kobashi launches.
- **Close to system tray** keeps Kobashi and its enabled bridges running when
  you close the window and is enabled by default. Use **Show Kobashi** from the
  Windows system tray or macOS menu bar to reopen it. Turning this option off
  makes closing the window quit Kobashi and restore client configurations.

Login startup runs in the tray when close to tray is enabled; otherwise it opens
the window. Settings are saved in `~/.kobashi/settings.json` independently of your
GitHub connection and are preserved when you disconnect. The main window is
400 logical pixels wide and fits the connected homepage without reserving space
for empty notices. Other screens use a 400 × 400 content area on Windows and
400 × 480 on macOS, with a plain background and no decorative footer animation.
Windows scales the entire window,
including its frame, with display DPI and keeps all controls visible without scrolling.

To stop Kobashi while close to tray is enabled, select **Quit Kobashi** in Settings
window or **Quit Kobashi** in the tray/menu-bar menu. macOS also supports **Cmd-Q**,
**Kobashi > Quit Kobashi**, and **Dock > Quit**. Quitting stops the bridge process
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
- **Copilot Bridge** — independent OpenAI-compatible endpoint for the GitHub Copilot app, preserving its GitHub login while using Kobashi's account for inference
- Auto-injects configs (`~/.claude/settings.json`, `~/.codex/config.toml`) and restores them on disconnect without replacing Codex account login
- Auto-detects system HTTP(S) proxy — routes only Bridge's upstream traffic through it, leaving other apps untouched
- Light/dark mode with system preference detection
- Opt-in login startup and close to system tray enabled by default
- Compact fixed-size window with a plain background
- Native Win32 and Cocoa clients with no third-party runtime
<!-- - Optional token-authenticated Server, remote Client, and local-network Server discovery -->

### GitHub Copilot app

Keep the Copilot app signed in with account **A**, and connect Kobashi to an
authorized Copilot account **B**. Enable **Copilot Bridge** in Kobashi; it is off
by default and does not require either of the other bridges.

In the Copilot app, open **Settings > Model providers > Add provider** and select
an **OpenAI-compatible** provider. Use the display name **Kobashi**, base URL
`http://127.0.0.1:18925/v1`, and API key `PROXY_MANAGED`. With `--port-offset N`,
use port `18925 + N`. Use the provider's model catalog or add a model ID available to
account B, then select that **Kobashi** model in the session model picker.
The endpoint supports HTTP Responses and Chat Completions, including streaming
and tool calls; use Responses for GPT models that require it.

Enabling the bridge does not add the provider to the Copilot app automatically.
On Windows, check the default endpoint below; adjust the port if using `--port-offset`:

```powershell
curl.exe --fail --silent --show-error -H "Authorization: Bearer PROXY_MANAGED" http://127.0.0.1:18925/v1/models
```

A model list confirms connectivity and catalog access, not inference. Select a
model under the **Kobashi** provider and send a message in a local Copilot session
to verify inference. Connection refusal means no listener is available at that
address; `401` means the API key is wrong, the bridge is disabled, or Kobashi is
not connected. Neither case indicates a working provider.

Only requests routed through this provider use account B's model access and
quota. GitHub repository operations, MCP connections, and other app features
retain their existing authentication; cloud sessions do not use this local
endpoint. Selecting a built-in GitHub-hosted model still uses the app's normal
Copilot account. Kobashi does not alter the app's login, credential store,
provider settings, or environment variables. Disable Copilot Bridge to stop
accepting its requests; disconnecting Kobashi also stops access. Remove the
provider in the app yourself if no longer needed.

The model catalog comes from Kobashi's connected account, independently of
Codex's model checks. Account B must be authorized for this use, and any code sent through it
must comply with applicable organization policies and subscription terms.
See [GitHub's BYOK setup documentation](https://docs.github.com/en/copilot/how-tos/github-copilot-app/use-byok-models).

<!--
### Server and Client

Kobashi defaults to the local GitHub-backed bridge. Both native applications also
support this connection:

```text
Codex / Claude / Copilot app -> local Kobashi Client -> remote Kobashi Server -> GitHub Copilot
```

On the **Server**, connect to GitHub and enable the bridges you want to share.
Open **Settings > Share this Kobashi**, set the listen IP and port, and select
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
of a loopback-bound Server. URL path prefixes are supported. Kobashi does not
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
from Kobashi's GitHub connection. Kobashi preserves `~/.codex/auth.json` and uses
a non-secret provider-level bearer for inference, so ChatGPT credentials are not
sent to Kobashi or GitHub Copilot. It does not set global OpenAI API credentials.
The provider keeps `requires_openai_auth = true` so Codex can display the saved
ChatGPT account; the explicit provider bearer still controls inference requests.

Kobashi recovers a managed placeholder login from its saved `auth.json.bak`
without replacing a newer login. If no usable backup exists, the app asks you to
sign in to Codex again. User API keys, `CODEX_API_KEY`, and organization login
policies are preserved; the plugin store requires Codex's active account to be
ChatGPT rather than API key auth. An expired or revoked login may also require
sign-in. Fully quit and reopen Codex yourself to load changed configuration.

### Codex model compatibility

Kobashi refreshes the account's Copilot model directory and performs one online
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

On first setup (or an account switch with no verified cache), Kobashi prepares the
verified directory before injecting the Codex configuration. Open Codex after
this preparation completes. On subsequent launches, the previous verified
snapshot remains available while the new startup check runs; a network timeout
does not revoke an earlier successful ordinary-inference check.

The native Codex app server reads `model_catalog_json` at process startup. It does
not reload its picker when that file or `config.toml` is rewritten. Therefore,
new models, removed models, and new effort options found by a startup round are
saved for the **next full Codex launch**; `/v1/models` reflects them as checks
finish. If Codex was already open during first setup, fully quit and reopen it
after preparation. Kobashi never restarts the client or interrupts a live turn.

New models are checked with a small, real streaming tool-call request
before being published to the directory or `/v1/models`. HTTP 200 alone is
insufficient: the check must finish successfully with a valid tool call. There is
no model-name allowlist, compatibility exception list, or silent model substitution.

The same refresh reads the installed Codex client's native catalog. Ultra is a
client delegation mode: its exact native model metadata and underlying API effort
must both be supported. Models declaring Ultra or Multi-Agent V2 also require a
real agent-message roundtrip: the API must generate a message and return its
complete original payload when replayed. Until that check succeeds, Kobashi
publishes V1 delegation without Ultra and retains the model's normal API efforts.
A newly added native model or Ultra mode is discovered and verified automatically.
A Copilot model absent from the native catalog can still
appear with its advertised API efforts; Kobashi cannot invent a client mode that
the installed Codex does not declare.

Parameter compatibility is learned from explicit upstream validation responses.
Supported alternatives for reasoning context, effort, summary, and text verbosity
are applied without changing the selected model, conversation, or tools.
If the upstream API explicitly rejects `service_tier`, Kobashi omits that field
and caches the rejection for that model and account; it never substitutes another
service tier. HTTP validation errors can be retried before returning the response.
Streaming failures remain visible and update the rules for subsequent requests,
without replaying an already-started response. Multiple optional-field changes
can be negotiated with at most three compatibility retries,
separate from the existing single authentication retry. Ambiguous errors and
changes to user data, tools, model IDs, or token budgets are never guessed.

Streaming failures preserve the upstream error code and message, including when
Copilot sends those details after an empty `response.failed` event. Failures are
logged in `~/.kobashi/kobashi.log`. Encrypted agent content is forwarded unchanged.
If Copilot cannot decrypt it, Kobashi reports the actual failure rather than
discarding the message or presenting ciphertext as readable conversation text.
An agent decryption failure also invalidates that model's cached Ultra/V2
capability without removing ordinary inference. Explicit Codex feature overrides
can still force V2; Kobashi does not change those overrides.
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

After a successful build, only `dist/Kobashi.app` (macOS) or `dist/Kobashi.exe`
(Windows) and `dist/SHA256SUMS.txt` are published back to the checkout. The existing
release for the other platform is preserved. `SHA256SUMS.txt` lists the Windows
executable and each file inside the macOS app bundle. The complete bundle is
copied and verified before replacing an existing app; replacement failure
triggers rollback to the previous bundle. No intermediate binaries or dated build
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
The runner selects the Xcode default Swift toolchain.

Native checks first send mouse events through an isolated app event loop to
verify that both bridge cards toggle off and back on when clicked on the switch
or card body. Toggle requests use a fake service without changing client settings.
The lifecycle checks exercise the fixed window size, close-to-tray setting, menu-bar
restore entry, repeated restoration, minimization, and the termination callback.
Actual menu-bar/Dock activation, Cmd-Q/Dock Quit delivery, and live
inference continuity still need a hands-on macOS smoke test. Never stop a running
Kobashi instance from an automated test.

For a safe hands-on check while your real bridge stays running, run
`go run ./cmd/build -test-mac -smoke`. This opens **Kobashi Lifecycle Test (no bridge)** with a
different bundle identifier, a counter, and a disposable child's PID. Close its
window, wait, and select **Show Kobashi** from its menu-bar icon: the counter
should advance and the PID should stay the same. Repeat after minimizing and
hiding. Quit **only the test
app** with Cmd-Q; rerun to check Dock > Quit. The terminal confirms its child has
stopped. This checks native event delivery, not real inference or client-config
cleanup. The temporary test bundle is removed when the test exits.

## License

MIT
