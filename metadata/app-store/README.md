# ModelBridge · App Store GTM

Prepared 2026-11-11. Product name: **ModelBridge**. No publisher-brand prefix or suffix.

## Deliverables

- `metadata.json`: 13 localized Mac listing drafts, category recommendation, concise captions and screenshot paths. All name/subtitle/promotional-text/description/keyword length limits checked. App Store Connect locale IDs map to the product's 13 locale IDs.
- `screenshots/<product-locale>/`: two 2560 × 1600 PNGs per locale, captured from the actual native candidate controls with synthetic account/usage data. They show implemented client controls and Settings. They are candidate previews, not proof of App Store eligibility or a live authenticated account.
- `../site/`: 13 localized privacy pages and 13 support pages. These are local static files; public HTTPS pages were deployed to the independent `modelbridge` Cloudflare Pages project at `https://modelbridge-ev3.pages.dev/`; App Store URL fields are being updated.
- `review-notes.md`: truthful review handoff for the current candidate, including the reasons it is not ready to submit.
- `../../internal/localization/catalog.json`: shared native UI/privacy text for all 13 locales. macOS includes the catalog and declares the locales in Info.plist; Windows embeds it.

Product locales: `en`, `zh-Hans`, `zh-Hant`, `ja`, `ko`, `de`, `fr`, `pt-BR`, `es`, `ar`, `hi`, `ru`, `vi`. European Portuguese (`pt-PT`) is outside the agreed scope.

## App Store Connect — 2026-11-13

[ModelBridge](https://appstoreconnect.apple.com/apps/6833990021/distribution/info): Apple ID `6833990021`, Bundle ID `com.pomegram.modelbridge`, SKU `modelbridge-macos`, macOS version `1.0`, primary language English (US), Developer Tools category, limited user access.

All 13 localized names/subtitles, promotional text, descriptions, keywords and support URLs are saved. Each locale has two custom screenshots named `804539a-<locale>-*.png`; these replace the old pink GitHub mark/account-text gallery. Capture uses the actual APP_STORE native controls with synthetic data, preserved native chrome and aspect ratio. They do not prove authenticated provider access.

Privacy/support pages are live at `https://modelbridge-ev3.pages.dev/<product-locale>/privacy/` and `/support/`. The public privacy policy URL is saved. The user-approved App Privacy label is published: linked User ID for App Functionality; linked Photos/Videos and Other User Content for App Functionality and Other Purposes; no tracking. The age questionnaire is saved as 4+, pricing is free, and 174 territories are selected for availability after release, with France excluded at the user's request. Review contact and preparation notes are saved privately in Apple. Release mode is manual. No App Review submission has been made.

## App Store candidate

The user explicitly authorized an independent App Store App Sandbox variant. The ordinary running Applications app is unchanged and is not restarted by this workflow. App Store UI uses white GitHub marks and white header account text; the ordinary variant retains its existing colors.

`scripts/build-app-store.py` snapshots source outside the checkout, bundles standalone Python 3.12.12/oMLX 0.7.3 and dependencies, and uses Xcode CLI archive/export with the existing Apple account. Build `1.0 (5)` was archived from source commit `94c3ef5d9ba869447a133c9d1be45194d1b7ce33`, verified pushed to `origin/main` before upload. Xcode archive/export, package signature validation and the isolated bundled-runtime/bridge self-test passed. Final distribution signatures for the app and both helpers have matching designated requirements. Xcode CLI altool uploaded the signed package successfully on 2026-11-13 at 18:32:14 (Asia/Shanghai); Apple buildUploads reported PROCESSING with no errors or warnings. Build verification stopped immediately. See `build5-upload.json` for source and delivery details. Earlier 2.2.1 builds 1–4 are superseded; no App Review submission was made.

App Store clients initially remain Paused until the user grants access to the relevant configuration folder through NSOpenPanel/security-scoped bookmarks. Start at login uses SMAppService. Python/oMLX runtime code is bundled; only model data is downloaded after installation. App Store boundary tests and the prior candidate runtime/isolated bridge self-test passed. Native folder grants, real Qwen3 4B inference and all three client protocols passed as described below.

## User publication handoff — 2026-11-13

A fresh Apple submission check found missing localized privacy-policy URLs. All 13 locale URLs were then saved and read back after reloading the server-backed page. Re-running Add for Review validation now reports only “A build must be selected.” The published App Privacy data declarations were unchanged.

Apple reported 1.0 (5) COMPLETE / VALID / APP_STORE_ELIGIBLE. The correct build was attached and saved to the 1.0 draft. The user explicitly chose to exclude France from initial availability; the saved availability count is 174, with French UI/listing localization retained. Export compliance was answered accurately as standard encryption outside Apple's OS implementation and no France distribution. Apple accepted the response and removed the missing-compliance status; no French documentation was required for this selected launch scope.

Add for Review validation passed. Version 1.0 now displays “Ready for Review” and is included as the sole item in a draft submission with build 1.0 (5). The embedded browser is left at the enabled final “Submit for Review” button. It was not clicked. The user owns final App Review submission; after approval, public release is manual. No further source build or upload is needed for these saved metadata/availability changes. Revisit export compliance before adding France in a future release.

## Focused validation

The shared catalog, placeholders, system-language negotiation, persistence/rollback, Arabic RTL, native navigation and cancelled authorization races passed. Ordinary Go tests and three App Store boundary tests passed. Current APP_STORE screenshot capture compiled successfully for all 13 locales, and white header text was visually checked in the English main view. Existing unchanged passing checks are reused.

Capture with `python3 scripts/capture-store-screenshots.py --output /absolute/output/path`; native localization check: `go run ./cmd/build -test-mac-localization`.

Apple references: [App Review Guidelines](https://developer.apple.com/app-store/review/guidelines/) (2.4.5, 2.5.2, 5.2.2), [Mac screenshot sizes](https://developer.apple.com/help/app-store-connect/reference/app-information/screenshot-specifications), [App Privacy details](https://developer.apple.com/app-store/app-privacy-details/).

## Native acceptance and follow-up candidate — 2026-11-13

An isolated App Store acceptance copy reused build 3's backend/runtime, with its own bundle identifier/container and ports 21121/21122/21123/21125. Qwen3 4B downloaded and completed real on-device inference. Actual NSOpenPanel grants exposed a signing mismatch: the helper's designated requirement used `kobashi-arm64`, so it could not resolve app-scoped bookmarks created by its parent. Matching the helper signing identifier to the parent resolved it. Codex, Claude Code and Copilot directory grants/configuration succeeded; Codex authorization also survived helper restart. All three client protocols passed streaming tool-call and tool-result round trips. Disabling integrations restored the temporary configurations' contents; Codex TOML field order was normalized. The ordinary app and real client configurations were untouched.

The follow-up source also adds a chip icon for local accounts and 20pt below the local context slider, preserving client-row bottom spacing. The affected native context checks passed. The current 1.0 (5) candidate includes the helper signing fix and local-model controls introduced in build 4.

The pinned [Qwen3 4B](https://huggingface.co/mlx-community/Qwen3-4B-Instruct-2507-4bit), [Qwen3 8B](https://huggingface.co/mlx-community/Qwen3-8B-4bit) and [Qwen3-Coder 30B](https://huggingface.co/mlx-community/Qwen3-Coder-30B-A3B-Instruct-4bit) model repositories all declare Apache-2.0. Open-source licensing grants usage rights without transferring copyright ownership; it does not require separate author approval for permitted use. Model weights are downloaded from upstream and are not shipped in the app package. The user explicitly approved the complete content-rights statement; it is saved in App Store Connect as third-party content with the necessary rights.

After web sign-in was restored and the user explicitly confirmed rights for the models, client icons and GitHub Copilot service access, the content-rights statement was saved. Reloading App Information confirmed “Yes, this app has the necessary rights to its third-party content.” This is a rights-to-use statement, not exclusive ownership of the models.

## First App Store release version — 2026-11-13

The user selected 1.0 for the first App Store release. The existing App Store draft was changed from 2.2.1 to 1.0 and read back with manual release still selected. The matching 1.0 (5) binary is now uploaded and Apple reported PROCESSING. The earlier 2.2.1 (4) package was preserved as historical evidence; uploaded packages cannot be relabeled. Store builds now read their marketing version from `metadata.json`, while the ordinary build keeps using its existing package version. Existing 13-locale text and screenshot assignments are retained.
