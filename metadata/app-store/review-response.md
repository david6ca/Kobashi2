# App Review response draft — not sent

Use this draft after selecting the new audited replacement build. Final reply and resubmission remain with the app owner.

Hello App Review,

ModelBridge requires `com.apple.security.network.server` for its core local HTTP inference bridge. Its bundled Go helper listens for incoming requests from separately installed coding clients on `127.0.0.1:18921` (Codex Responses API), `:18923` (Claude Code Messages API) and `:18925` (Copilot Chat Completions API). The native UI also connects to the helper's authenticated control/status server on `127.0.0.1:18922`. In Local mode, the bundled oMLX process runs a separate loopback HTTP inference server on an ephemeral port. The entitlement is declared on the parent app because the Go/Python child processes inherit its sandbox. All listeners bind to loopback only.

To exercise the functionality, add a local model on an Apple Silicon Mac with macOS 15+, enable a client row and grant its configuration directory, then send a prompt from that coding client. It connects to the corresponding incoming HTTP listener and receives streamed model output. The native UI uses the control server during normal operation. Outgoing HTTPS/model downloads use the separate network.client entitlement.

We traced the API references to bundled third-party components and prepared these changes:

- Removed unused Tcl/Tk/tkinter GUI libraries containing the non-public AppKit notification and selectors.
- Replaced the Accelerate-linked SciPy wheel with the same SciPy 1.18.1 release's official OpenBLAS wheel, eliminating the listed legacy BLAS/LAPACK references while retaining scientific computation.
- Removed the unused PyArrow S3 backend that imported the three listed CommonCrypto GCM functions. Local Arrow, Parquet and datasets functionality remains available; model downloads use Hugging Face HTTPS.
- Rebuilt SDL2 from verified official source without its unused Cocoa video/Metal renderer backends, removing an additional private menu selector while preserving OpenCV image and video processing.

The build pipeline now checks every Mach-O binary against the complete reported API list before export. Regression checks cover scientific computation, local datasets, SSL/oMLX imports, OpenCV image/video I/O and real on-device Qwen3 4B inference. The server-entitlement explanation is also included in the prepared App Review Information notes.

Thank you.
