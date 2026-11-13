# ModelBridge review handoff — not submitted

ModelBridge connects separately installed coding clients (Claude Code, Codex and GitHub Copilot) to a selected inference source. GitHub sign-in is an optional inference-source connection; Local inference on a supported Mac does not require a GitHub account or Copilot subscription. The product does not create its own hosted user account.

The current desktop candidate requires macOS 13+. Local inference requires an Apple Silicon Mac with macOS 15+, sufficient memory/storage and initial downloads. GitHub Copilot access and usage limits follow the connected account. Do not describe the bridge as providing unlimited or free Copilot access, or as an official client from GitHub, OpenAI or Anthropic.

## Review steps after the submission dependencies are resolved

1. Open ModelBridge. Open Settings and use Language to select a language or System Default. The selection applies immediately and survives relaunch.
2. Open Privacy Statement from Settings. The statement is bundled and readable offline.
3. On a supported Mac, choose Add account, select an available local model and Add. Verify the final store candidate's actual setup flow before sending these steps to Apple; the current downloaded executable runtime is not a compliant submission route.
4. Install a supported coding client, enable its row and send a simple test request in that client. The row's Active state means the integration is enabled, not that every existing client session has switched.
5. For the optional GitHub Copilot source, connect an eligible review account privately. Do not place a token, password or reviewer contact's phone/name in this repository. A local model review path does not by itself establish review access to the Copilot branch; provide a suitable demo or reviewer access when preparing the final submission.
6. In Settings, change Appearance, Start at login and Close to system tray. Verify the eligible sandboxed build's behavior independently of this desktop candidate.

## Data handling

Local model inference is local. Runtime/model installation connects to download hosts. In GitHub Copilot mode, prompts, code, images, conversation and tool results included in requests are forwarded to GitHub Copilot and its model providers. Saved credentials/settings and diagnostic/setup logs are local files. Account removal removes saved ModelBridge credentials; it does not delete the provider account, provider-retained data, logs or model downloads.

Before submission, supply the existing App Store Connect review contact privately, verify export-compliance answers for the final bundled code, complete the age-rating questionnaire against actual functionality, and replace the unverified privacy/support URL fields with deployed HTTPS URLs. This file is preparation, not a declaration that these checks or App Review have passed.
