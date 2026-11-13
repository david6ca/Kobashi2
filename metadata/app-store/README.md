# ModelBridge · App Store GTM

Prepared 2026-11-11. Product name: **ModelBridge**. No publisher-brand prefix or suffix.

## Deliverables

- `metadata.json`: 13 localized Mac listing drafts, category recommendation, concise captions and screenshot paths. All name/subtitle/promotional-text/description/keyword length limits checked. App Store Connect locale IDs map to the product's 13 locale IDs.
- `screenshots/<product-locale>/`: two 2560 × 1600 PNGs per locale, captured from the actual native candidate controls with synthetic account/usage data. They show implemented client controls and Settings. They are candidate previews, not proof of App Store eligibility or a live authenticated account.
- `../site/`: 13 localized privacy pages and 13 support pages. These are local static files; public HTTPS pages were deployed to the independent `modelbridge` Cloudflare Pages project at `https://modelbridge-ev3.pages.dev/`; App Store URL fields are being updated.
- `review-notes.md`: truthful review handoff for the current candidate, including the reasons it is not ready to submit.
- `../../internal/localization/catalog.json`: shared native UI/privacy text for all 13 locales. macOS includes the catalog and declares the locales in Info.plist; Windows embeds it.

Product locales: `en`, `zh-Hans`, `zh-Hant`, `ja`, `ko`, `de`, `fr`, `pt-BR`, `es`, `ar`, `hi`, `ru`, `vi`. European Portuguese (`pt-PT`) is outside the agreed scope.

## App Store Connect draft — 2026-11-12

Created [ModelBridge](https://appstoreconnect.apple.com/apps/6833990021/distribution/info), Apple ID `6833990021`, Bundle ID `com.pomegram.modelbridge`, SKU `modelbridge-macos`, macOS version `2.2.1`, primary language English (US), with limited user access. Saved the Developer Tools category and all 13 localized names/subtitles plus promotional text, descriptions and keywords; reloaded and read back every locale. Review contact and preparation notes were saved privately in Apple; release mode is manual.

Historical pause: screenshot work was paused while UI changes continued. At pause, English had two assigned screenshots and simplified Chinese had four (including duplicate assignments after an upload retry). The user resumed screenshot capture/replacement on 2026-11-13 after confirming the UI is ready. Local screenshots are preparation artifacts, not the final accepted UI.

No build was uploaded and no review was submitted. Public privacy/support URLs, App Privacy, content-rights and age-rating answers still need completion. The ordinary installed app remains non-sandbox; the independent App Store App Sandbox candidate was explicitly authorized and prepared as described below. Source must be committed and pushed to `main` before any new build upload, with its commit recorded in the receipt.

## App Store candidate — 2026-11-13

The user authorized an independent App Store App Sandbox build. `scripts/build-app-store.py` now snapshots source outside the checkout, bundles standalone Python 3.12.12 / oMLX 0.7.3 and its dependencies, and uses Xcode CLI archive/export with the existing Apple account. ModelBridge `2.2.1 (1)` archived and exported successfully. The 367 MiB package signature was validated on the host through Apple Root CA. App Sandbox self-test passed: bundled Python/oMLX imports and an isolated-port bridge startup/exit without reading existing accounts. The ordinary Applications app was not replaced and screenshot updates remain paused.

App Store mode keeps initial client rows Paused until a user selects the client configuration folder through NSOpenPanel; security-scoped bookmarks are resolved by the native Go helper. Start at login uses SMAppService. Running the App Store mode does not download/install runtime code; model-data downloads remain available. Three affected App Store boundary tests and affected ordinary account/environment tests passed. Real external client-folder grants and real provider inference still need acceptance; the smoke test does not establish them.

The exported package is a candidate, not an uploaded or reviewed build. Its source snapshot contains uncommitted changes; before uploading, commit the corresponding source, push to main and verify the committed inputs, then rebuild and record the commit in the upload receipt. Build output and machine-specific receipts stay outside the repository.

## Submission dependencies

| Dependency | Current evidence and next action |
| --- | --- |
| Sandboxed Mac candidate | The ordinary `cmd/build/main.go` route remains non-sandbox. The independent `macos/app-store/ModelBridge.xcodeproj` now creates the authorized App Sandbox candidate; native folder grants still need acceptance. Original integration evidence: `internal/bridge/config.go` and `claude.go` change other clients' home-directory configuration; `platform_darwin.go` writes a LaunchAgent and user environment. Design a sandbox-compatible access/integration route and verify it before submission. This is a product/architecture change, not a metadata correction. |
| Self-contained local inference | The ordinary route in `internal/bridge/local_runtime.go` downloads and installs runtime code. The App Store build-tag route now uses the bundled, verified runtime and does not perform that installation. Review Guidelines 2.4.5(ii)/(iv) and 2.5.2 are directly relevant. Bundle a compliant runtime and verify its subprocess/sandbox behavior. Downloadable weights are distinct from executable runtime code. Do not silently drop Local or assume a developer-tool exemption. |
| Privacy and service disclosure | The selected GitHub Copilot source sends request content to an external service. Credentials currently live in local files, not a demonstrated Keychain store. Confirm the selected providers' data retention and complete App Privacy, including User Content/User ID/Diagnostics where required; do not preselect “Data Not Collected.” Review the service integration under its actual terms and Apple's 5.2.2 before submission. |
| Store record and public URLs | The ModelBridge record and 13 text localizations have been created and verified as described above; pricing/territory settings, age rating, content rights, App Privacy and build upload/review are not complete. Confirm the legal operator for public policy attribution, publish the prepared privacy/support pages at owned HTTPS URLs, then fill those fields. Reuse the existing Apple account and signing/release tooling when producing the eligible candidate. Review contact must be entered privately, not committed here. |

These dependencies block submission, not the completed localization and listing preparation. Do not substitute Developer ID distribution or remove core product features without a user decision. No App Review submission or TestFlight upload was performed.

## Focused validation

- Shared catalog coverage, placeholder integrity and ordered system-language negotiation.
- Language API persistence across reinitialization, unchanged account/client/settings state, invalid choices and failed-write rollback.
- Offscreen native menu/language/privacy/RTL checks; no live bridge or desktop window is opened. Screenshot capture uses the same native controls and all 13 locales.
- Windows desktop test binary cross-compilation is a compile check, not Windows visual acceptance.

Run the native check with `go run ./cmd/build -test-mac-localization`. To regenerate PNGs, set `KOBASHI_SCREENSHOT_OUTPUT` to an explicit output directory. UI wording and store text must continue to match the final eligible build; regenerate affected screenshots after relevant changes.

Apple references checked for this preparation: [App Review Guidelines](https://developer.apple.com/app-store/review/guidelines/) (2.4.5, 2.5.2, 5.2.2), [Mac screenshot sizes](https://developer.apple.com/help/app-store-connect/reference/app-information/screenshot-specifications), [App Privacy details](https://developer.apple.com/app-store/app-privacy-details/).

## UI-ready GTM batch — 2026-11-13

The user confirmed the UI is ready and requested screenshot capture, App Store GTM preparation, commit and push. The updated capture pipeline uses an independent no-bridge preview and WindowServer screenshots of only its windows, preserves native chrome and aspect ratio, and produces 26 RGB 2560×1600 PNGs for 13 locales. Privacy/support pages are live at the independent Pages site; no DNS was changed. The ordinary running app is not restarted.

## Current store preparation — 2026-11-13

All 13 listing localizations and support URLs are saved in App Store Connect. The public privacy policy URL is saved. The user-approved App Privacy label is published: linked User ID for App Functionality, linked Photos/Videos and Other User Content for App Functionality and Other Purposes, with no tracking. The age questionnaire is saved as 4+, pricing is free, and availability covers 175 territories after release. Manual release remains selected; no App Review submission has been made.

The user selected official white GitHub marks for the App Store variant and white header account text. The 26 local screenshots have been regenerated using APP_STORE controls and synthetic data; they supersede the previously saved pink-mark gallery. Remote gallery replacement and the matching build upload remain pending. The ordinary running app is unchanged. Earlier build 2 exported successfully but is superseded by this UI change and must not be uploaded. Final content-rights answers and real client-folder/provider acceptance remain outstanding before submission.
