#!/usr/bin/env bash
set -euo pipefail
IFS=$'\n\t'

FORGE_GITHUB_OWNER="${FORGE_GITHUB_OWNER:-tOgg1}"
FORGE_GITHUB_REPO="${FORGE_GITHUB_REPO:-forge}"
FORGE_VERSION="${FORGE_VERSION:-latest}"
FORGE_INSTALL_DIR="${FORGE_INSTALL_DIR:-/usr/local/bin}"
FORGE_BINARIES="${FORGE_BINARIES:-forge forged forge-agent-runner fmail}"

log() {
  printf '%s\n' "[forge-install] $*"
}

warn() {
  printf '%s\n' "[forge-install] WARN: $*" >&2
}

fail() {
  printf '%s\n' "[forge-install] ERROR: $*" >&2
  exit 1
}

command_exists() {
  command -v "$1" >/dev/null 2>&1
}

fetch_url() {
  local url="$1"
  if command_exists curl; then
    curl -fsSL "$url"
    return 0
  fi
  if command_exists wget; then
    wget -qO- "$url"
    return 0
  fi
  fail "curl or wget is required"
}

download() {
  local url="$1"
  local dest="$2"
  if command_exists curl; then
    curl -fsSL "$url" -o "$dest"
    return 0
  fi
  if command_exists wget; then
    wget -qO "$dest" "$url"
    return 0
  fi
  fail "curl or wget is required"
}

detect_arch() {
  local arch
  arch="$(uname -m)"
  case "$arch" in
    x86_64|amd64)
      printf '%s' "amd64"
      ;;
    aarch64|arm64)
      printf '%s' "arm64"
      ;;
    *)
      fail "unsupported architecture: $arch"
      ;;
  esac
}

resolve_tag() {
  local version="$1"
  if [ "$version" = "latest" ]; then
    local tag
    tag="$(fetch_url "https://api.github.com/repos/${FORGE_GITHUB_OWNER}/${FORGE_GITHUB_REPO}/releases/latest" | tr -d '\r' | awk -F\" '/tag_name/{print $4; exit}')"
    if [ -z "$tag" ]; then
      fail "failed to resolve latest release tag"
    fi
    printf '%s' "$tag"
    return
  fi
  case "$version" in
    v*)
      printf '%s' "$version"
      ;;
    *)
      printf 'v%s' "$version"
      ;;
  esac
}

install_binary() {
  local src="$1"
  local dest="$2"
  local dest_dir

  dest_dir="$(dirname "$dest")"
  if [ ! -d "$dest_dir" ]; then
    mkdir -p "$dest_dir"
  fi

  if [ -w "$dest_dir" ]; then
    if command_exists install; then
      install -m 0755 "$src" "$dest"
    else
      cp "$src" "$dest"
      chmod 0755 "$dest"
    fi
    return
  fi

  if command_exists sudo; then
    if command_exists install; then
      sudo install -m 0755 "$src" "$dest"
    else
      sudo cp "$src" "$dest"
      sudo chmod 0755 "$dest"
    fi
    return
  fi

  fail "no permission to write to $dest_dir (try running with sudo or set FORGE_INSTALL_DIR)"
}

main() {
  if [ "$(uname -s)" != "Linux" ]; then
    fail "this installer is Linux-only"
  fi

  if ! command_exists tar; then
    fail "tar is required to extract the release archive"
  fi

  local arch tag version asset url tmpdir
  arch="$(detect_arch)"
  tag="$(resolve_tag "$FORGE_VERSION")"
  version="${tag#v}"
  asset="forge_${version}_linux_${arch}.tar.gz"
  url="https://github.com/${FORGE_GITHUB_OWNER}/${FORGE_GITHUB_REPO}/releases/download/${tag}/${asset}"

  tmpdir="$(mktemp -d)"
  trap 'rm -rf "$tmpdir"' EXIT

  log "downloading ${asset}"
  download "$url" "$tmpdir/$asset"

  tar -xzf "$tmpdir/$asset" -C "$tmpdir"

  local installed_any=0
  local bin
  for bin in $FORGE_BINARIES; do
    if [ -f "$tmpdir/$bin" ]; then
      log "installing $bin to $FORGE_INSTALL_DIR"
      install_binary "$tmpdir/$bin" "$FORGE_INSTALL_DIR/$bin"
      installed_any=1
    else
      warn "binary not found in archive: $bin"
    fi
  done

  if [ "$installed_any" -eq 0 ]; then
    fail "no binaries were installed"
  fi

  log "done"
}

main "$@"
