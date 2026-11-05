#!/bin/sh
# Install the latest public Marshal release on macOS.
set -eu

fail() {
  printf '%s\n' "$*" >&2
  exit 1
}

[ "$(uname -s)" = Darwin ] || fail "This installer is for macOS."

repo=khanblair/marshal
destination="${MARSHAL_INSTALL_DIR:-/Applications}"
case "$destination" in
  /*) ;;
  *) fail "MARSHAL_INSTALL_DIR must be an absolute path." ;;
esac
app="$destination/Marshal.app"

if [ -e "$app" ] || [ -L "$app" ]; then
  fail "Marshal already exists at $app. Open it and use Settings → Updates."
fi

work="$(mktemp -d)"
mount="$work/mount"
mounted=
stage=
cleanup() {
  if [ -n "$stage" ]; then rm -rf "$stage"; fi
  if [ -n "$mounted" ] && ! hdiutil detach "$mount" -quiet; then
    printf 'Could not detach the installer at %s.\n' "$mount" >&2
    return
  fi
  rm -rf "$work"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

echo "Looking up the latest Marshal release ..."
metadata="$work/release.json"
curl -q -fsSL --proto '=https' --proto-redir '=https' -o "$metadata" \
  "https://api.github.com/repos/$repo/releases/latest"
tag="$(plutil -extract tag_name raw -o - "$metadata")"
case "$tag" in
  v[0-9]*) ;;
  *) fail "The release has an invalid version tag." ;;
esac
case "$tag" in
  *[!a-zA-Z0-9.+-]*) fail "The release has an invalid version tag." ;;
esac
name="Marshal_${tag#v}_universal.dmg"
index=0
while asset="$(plutil -extract "assets.$index.name" raw -o - "$metadata" 2>/dev/null)"; do
  [ "$asset" != "$name" ] || break
  index=$((index + 1))
done
digest="$(plutil -extract "assets.$index.digest" raw -o - "$metadata" 2>/dev/null)" ||
  fail "The latest release has no checksum for $name."
case "$digest" in
  sha256:*) expected="${digest#sha256:}" ;;
  *) fail "The release does not provide a SHA-256 checksum for $name." ;;
esac
case "$expected" in
  *[!0-9a-f]*) fail "The release has an invalid SHA-256 checksum." ;;
esac
[ "${#expected}" -eq 64 ] || fail "The release has an invalid SHA-256 checksum."

echo "Downloading $name ..."
dmg="$work/$name"
curl -q -fL --proto '=https' --proto-redir '=https' --progress-bar -o "$dmg" \
  "https://github.com/$repo/releases/download/$tag/$name"
checksum="$(shasum -a 256 "$dmg")"
[ "${checksum%% *}" = "$expected" ] || fail "The DMG checksum did not match. Nothing was installed."

mkdir "$mount"
hdiutil attach "$dmg" -nobrowse -readonly -mountpoint "$mount" -quiet
mounted=yes
[ -d "$mount/Marshal.app" ] || fail "The disk image does not contain Marshal.app."
mkdir -p "$destination"
stage="$(mktemp -d "$destination/.marshal-install.XXXXXXXX")"
ditto "$mount/Marshal.app" "$stage/Marshal.app"
codesign --verify --deep --strict "$stage/Marshal.app"

# Moving into the parent with -n keeps an app installed by a concurrent run.
/bin/mv -n "$stage/Marshal.app" "$destination/"
[ ! -e "$stage/Marshal.app" ] || fail "Marshal already exists at $app."
printf 'Installed %s. Open Marshal from Applications or Spotlight.\n' "$app"
