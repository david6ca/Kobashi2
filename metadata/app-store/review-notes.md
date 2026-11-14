# ModelBridge App Review information draft

ModelBridge connects separately installed coding clients (Claude Code, Codex and GitHub Copilot) to a selected inference source. GitHub sign-in is an optional inference-source connection; Local inference on a supported Mac does not require a GitHub account or Copilot subscription. The product does not create its own hosted user account.

The current desktop candidate requires macOS 13+. Local inference requires an Apple Silicon Mac with macOS 15+, sufficient memory/storage and initial downloads. GitHub Copilot access and usage limits follow the connected account. Do not describe the bridge as providing unlimited or free Copilot access, or as an official client from GitHub, OpenAI or Anthropic.

## Required incoming network connections

`com.apple.security.network.server` is required for ModelBridge's core local HTTP bridge. The app's bundled Go helper listens for and responds to incoming connections from separately installed coding clients and the native UI on the same Mac:

| Loopback listener | Function |
| --- | --- |
| `127.0.0.1:18921` | Codex Responses API requests and streamed responses |
| `127.0.0.1:18922` | Authenticated native UI control/status API |
| `127.0.0.1:18923` | Claude Code Messages API requests and streamed responses |
| `127.0.0.1:18925` | Copilot client Chat Completions requests and streamed responses |

Local inference also starts the bundled llama.cpp HTTP server on an ephemeral `127.0.0.1` port, which receives requests from the bridge. All listeners bind explicitly to loopback, not a LAN or public interface. The Go/native-engine child processes inherit the parent app's sandbox, so the entitlement is declared on the parent. These are incoming server connections, in addition to the outgoing HTTPS connections used for GitHub and model downloads.

To exercise the server functionality, add a local model on a supported Mac, enable a client row and grant its configuration directory, then send a prompt from that coding client. The client calls the corresponding loopback listener and receives the model's streamed response. The UI also uses its local control listener throughout normal operation. Implementation: `internal/bridge/app.go` (`App.Start`) and `internal/bridge/local_runtime.go` (`startRuntime`).

## Review steps after the submission dependencies are resolved

1. Open ModelBridge. Open Settings and use Language to select a language or System Default. The selection applies immediately and survives relaunch.
2. Open Privacy Statement from the macOS app menu. The statement is bundled and readable offline.
3. On a supported Mac, choose Add account, select an available local model and Add. The App Store candidate bundles its native llama.cpp engine without Python; initial downloads contain SHA-256-verified GGUF model data. Verify real inference on the final candidate before submission.
4. Install a supported coding client, enable its row and send a simple test request in that client. The row's Active state means the integration is enabled, not that every existing client session has switched.
5. For the optional GitHub Copilot source, connect an eligible review account privately. Do not place a token, password or reviewer contact's phone/name in this repository. A local model review path does not by itself establish review access to the Copilot branch; provide a suitable demo or reviewer access when preparing the final submission.
6. In Settings, change Appearance, Start at login and Close to system tray. Verify the eligible sandboxed build's behavior independently of this desktop candidate.

## Data handling

Local model inference is local. Model-data downloads connect to download hosts; the App Store runtime is bundled. In GitHub Copilot mode, prompts, code, images, conversation and tool results included in requests are forwarded to GitHub Copilot and its model providers. Saved credentials/settings and diagnostic/setup logs are local files. Account removal removes saved ModelBridge credentials; it does not delete the provider account, provider-retained data, logs or model downloads.

Before submission, supply the existing App Store Connect review contact privately, verify export-compliance answers for the final bundled code, confirm content rights and real client-folder/provider access. Age rating and deployed HTTPS privacy/support URL fields are already saved. This file is preparation, not a declaration that these checks or App Review have passed.
