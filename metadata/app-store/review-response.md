# App Review response draft — not sent

Use this draft after build 1.0 (7) finishes processing, selecting it for review and saving `review-information.txt` in App Review Information. Final reply and resubmission remain with the app owner.

Hello App Review,

ModelBridge requires `com.apple.security.network.server` for its core local HTTP inference bridge. Its bundled Go helper listens for incoming requests from separately installed coding clients on `127.0.0.1:18921` (Codex Responses API), `:18923` (Claude Code Messages API) and `:18925` (Copilot Chat Completions API). The native UI also connects to the helper's authenticated control/status server on `127.0.0.1:18922`. In Local mode, the bundled llama.cpp process runs a separate loopback HTTP inference server on an ephemeral port. The entitlement is declared on the parent app because the Go/native-engine child processes inherit its sandbox. All listeners bind to loopback only.

To exercise the functionality, add a local model on an Apple Silicon Mac with macOS 15+, enable a client row and grant its configuration directory, then send a prompt from that coding client. It connects to the corresponding incoming HTTP listener and receives streamed model output. The native UI uses the control server during normal operation. Outgoing HTTPS/model downloads use the separate network.client entitlement.

We traced the reported API references to the previous Python inference runtime and its third-party dependencies. The replacement candidate removes that entire runtime, including Tcl/Tk, PyArrow, SciPy, SDL/OpenCV and ONNX Runtime. Local inference now uses a pinned native llama.cpp engine with Metal acceleration. Only GGUF model data is downloaded after installation; Python, pip and executable-code downloads are no longer part of the application.

The native candidate's archive passes the complete 52-name API audit across all 14 Mach-O binaries. Real on-device Qwen3 4B tests passed streaming tool-call and tool-result round trips through the Codex, Claude and Copilot protocols, plus idle context replacement and restoration after a settings-write failure. App Store boundary tests passed. This draft must be paired with the final signed/uploaded replacement build; no new reply or resubmission has been sent.

Thank you.
