#!/bin/sh
set -eu

if [ "$(uname -s)" != "Darwin" ]; then
  echo "error: signed macOS builds must run on macOS" >&2
  exit 1
fi

SIGNING_IDENTITY="${FEIDEX_CODESIGN_IDENTITY:-}"
SIGNING_KEYCHAIN="${FEIDEX_CODESIGN_KEYCHAIN:-}"
SIGNING_IDENTIFIER="${FEIDEX_CODESIGN_IDENTIFIER:-com.yuhong.feidex}"

if [ -z "$SIGNING_IDENTITY" ]; then
  echo "error: set FEIDEX_CODESIGN_IDENTITY to a stable code-signing identity" >&2
  echo "example: FEIDEX_CODESIGN_IDENTITY='Feidex Local Code Signing' $0" >&2
  exit 1
fi

if [ -n "$SIGNING_KEYCHAIN" ]; then
  IDENTITIES=$(/usr/bin/security find-identity -v -p codesigning "$SIGNING_KEYCHAIN")
else
  IDENTITIES=$(/usr/bin/security find-identity -v -p codesigning)
fi

case "$IDENTITIES" in
  *"$SIGNING_IDENTITY"*) ;;
  *)
    echo "error: code-signing identity not found: $SIGNING_IDENTITY" >&2
    echo "$IDENTITIES" >&2
    exit 1
    ;;
esac

SCRIPT_DIR=$(CDPATH= cd "$(dirname "$0")" && pwd)
REPO_DIR=$(dirname "$SCRIPT_DIR")
OUTPUT_PATH="${1:-bin/feidex}"

case "$OUTPUT_PATH" in
  /*) ;;
  *) OUTPUT_PATH="$REPO_DIR/$OUTPUT_PATH" ;;
esac

mkdir -p "$(dirname "$OUTPUT_PATH")"

cd "$REPO_DIR"
go build -o "$OUTPUT_PATH" ./cmd/feidex

if [ -n "$SIGNING_KEYCHAIN" ]; then
  /usr/bin/codesign \
    --force \
    --sign "$SIGNING_IDENTITY" \
    --keychain "$SIGNING_KEYCHAIN" \
    --identifier "$SIGNING_IDENTIFIER" \
    --timestamp=none \
    "$OUTPUT_PATH"
else
  /usr/bin/codesign \
    --force \
    --sign "$SIGNING_IDENTITY" \
    --identifier "$SIGNING_IDENTIFIER" \
    --timestamp=none \
    "$OUTPUT_PATH"
fi

/usr/bin/codesign --verify --strict --verbose=2 "$OUTPUT_PATH"
echo "built and signed $OUTPUT_PATH"
/usr/bin/codesign -dr - "$OUTPUT_PATH" 2>&1
