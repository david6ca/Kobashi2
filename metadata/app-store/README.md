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

[ModelBridge](https://appstoreconnect.apple.com/apps/6833990021/distribution/info): Apple ID `6833990021`, Bundle ID `com.pomegram.modelbridge`, SKU `modelbridge-macos`, macOS version `2.2.1`, primary language English (US), Developer Tools category, limited user access.

All 13 localized names/subtitles, promotional text, descriptions, keywords and support URLs are saved. Each locale has two custom screenshots named `804539a-<locale>-*.png`; these replace the old pink GitHub mark/account-text gallery. Capture uses the actual APP_STORE native controls with synthetic data, preserved native chrome and aspect ratio. They do not prove authenticated provider access.

Privacy/support pages are live at `https://modelbridge-ev3.pages.dev/<product-locale>/privacy/` and `/support/`. The public privacy policy URL is saved. The user-approved App Privacy label is published: linked User ID for App Functionality; linked Photos/Videos and Other User Content for App Functionality and Other Purposes; no tracking. The age questionnaire is saved as 4+, pricing is free, and 175 territories are selected for availability after release. Review contact and preparation notes are saved privately in Apple. Release mode is manual. No App Review submission has been made.

## App Store candidate

The user explicitly authorized an independent App Store App Sandbox variant. The ordinary running Applications app is unchanged and is not restarted by this workflow. App Store UI uses white GitHub marks and white header account text; the ordinary variant retains its existing colors.

`scripts/build-app-store.py` snapshots source outside the checkout, bundles standalone Python 3.12.12/oMLX 0.7.3 and dependencies, and uses Xcode CLI archive/export with the existing Apple account. Build `2.2.1 (3)` was archived from source commit `804539ad40414ada04a67317340420097567bfdd`, verified pushed to `origin/main` before build preparation. Signature verification and independent runtime/isolated bridge self-test passed; export is in progress. No build is uploaded. Earlier exported builds 1 and 2 are superseded and must not be uploaded.

App Store clients initially remain Paused until the user grants access to the relevant configuration folder through NSOpenPanel/security-scoped bookmarks. Start at login uses SMAppService. Python/oMLX runtime code is bundled; only model data is downloaded after installation. App Store boundary tests and the prior candidate runtime/isolated bridge self-test passed. Real client-folder grants and provider inference remain unverified.

## Remaining submission dependencies

- Complete the content-rights declaration against the app's third-party service/content permissions. Official GitHub white marks replace the previously modified color.
- Finish the matching build export/self-test; any authorized upload must record its exact pushed source commit. Stop build verification when Apple reports processing.
- Accept real client-folder grants and provider inference on the final candidate, and verify export-compliance answers for its bundled dependencies.
- App Review submission and public release require their own authorization.

## Focused validation

The shared catalog, placeholders, system-language negotiation, persistence/rollback, Arabic RTL, native navigation and cancelled authorization races passed. Ordinary Go tests and three App Store boundary tests passed. Current APP_STORE screenshot capture compiled successfully for all 13 locales, and white header text was visually checked in the English main view. Existing unchanged passing checks are reused.

Capture with `python3 scripts/capture-store-screenshots.py --output /absolute/output/path`; native localization check: `go run ./cmd/build -test-mac-localization`.

Apple references: [App Review Guidelines](https://developer.apple.com/app-store/review/guidelines/) (2.4.5, 2.5.2, 5.2.2), [Mac screenshot sizes](https://developer.apple.com/help/app-store-connect/reference/app-information/screenshot-specifications), [App Privacy details](https://developer.apple.com/app-store/app-privacy-details/).
