#!/bin/zsh
set -euo pipefail
root="${SRCROOT}/../.."
bin="${TARGET_BUILD_DIR}/${UNLOCALIZED_RESOURCES_FOLDER_PATH}/bin"
mkdir -p "$bin"
for pair in arm64:arm64 amd64:x86_64; do
    arch="${pair%%:*}"
    target="${pair##*:}"
    name="kobashi-arm64"
    if [[ "$arch" == amd64 ]]; then name="kobashi-x64"; fi
    cd "$root"
    CGO_ENABLED=1 GOOS=darwin GOARCH="$arch" CC=/usr/bin/clang \
        CGO_CFLAGS="-target ${target}-apple-macos13.0" CGO_LDFLAGS="-target ${target}-apple-macos13.0" \
        GOCACHE="${DERIVED_FILE_DIR}/go-cache" \
        go build -tags appstore -trimpath -ldflags "-s -w -X main.Version=${MARKETING_VERSION}" -o "$bin/$name" ./cmd/kobashi
    # App-scoped bookmarks require the helper's designated requirement to match the parent app.
    /usr/bin/codesign --force --identifier "${PRODUCT_BUNDLE_IDENTIFIER}" --sign "${EXPANDED_CODE_SIGN_IDENTITY:--}" \
        --entitlements "${SRCROOT}/Helper.entitlements" "$bin/$name"
done
