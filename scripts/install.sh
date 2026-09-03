#!/bin/sh
# DevTether install script.
# Usage:
#   curl -sSfL https://raw.githubusercontent.com/ivin-titus/devtether/main/scripts/install.sh | sh
#   curl -sSfL ... | sh -s -- -b /usr/local/bin
#
# Detects OS and architecture, downloads the latest release, verifies
# the SHA256 checksum, and installs the binary.

set -eu

REPO="ivin-titus/devtether"
INSTALL_DIR="${HOME}/.local/bin"

usage() {
    printf "Usage: %s [-b install_dir]\n" "$0"
    printf "  -b    Installation directory (default: %s)\n" "${INSTALL_DIR}"
    exit 1
}

# Parse flags.
while getopts "b:h" opt; do
    case "$opt" in
        b) INSTALL_DIR="$OPTARG" ;;
        h) usage ;;
        *) usage ;;
    esac
done

# Detect OS.
detect_os() {
    os="$(uname -s)"
    case "$os" in
        Linux)  echo "linux" ;;
        Darwin) echo "darwin" ;;
        *)
            printf "Error: unsupported OS '%s'. DevTether supports Linux and macOS.\n" "$os" >&2
            printf "See: https://github.com/%s/blob/main/docs/adr/006-platform-support-and-cgo-policy.md\n" "$REPO" >&2
            exit 1
            ;;
    esac
}

# Detect architecture.
detect_arch() {
    arch="$(uname -m)"
    case "$arch" in
        x86_64|amd64)   echo "amd64" ;;
        aarch64|arm64)  echo "arm64" ;;
        *)
            printf "Error: unsupported architecture '%s'. DevTether supports amd64 and arm64.\n" "$arch" >&2
            exit 1
            ;;
    esac
}

# Fetch the latest release tag from GitHub API.
latest_version() {
    url="https://api.github.com/repos/${REPO}/releases/latest"
    if command -v curl >/dev/null 2>&1; then
        curl -sSfL "$url" | grep '"tag_name"' | head -1 | cut -d'"' -f4
    elif command -v wget >/dev/null 2>&1; then
        wget -qO- "$url" | grep '"tag_name"' | head -1 | cut -d'"' -f4
    else
        printf "Error: curl or wget is required.\n" >&2
        exit 1
    fi
}

# Download a file.
download() {
    url="$1"
    output="$2"
    if command -v curl >/dev/null 2>&1; then
        curl -sSfL -o "$output" "$url"
    elif command -v wget >/dev/null 2>&1; then
        wget -qO "$output" "$url"
    fi
}

# Verify SHA256 checksum.
verify_checksum() {
    archive="$1"
    checksums="$2"
    basename="$(basename "$archive")"

    expected="$(grep "${basename}" "$checksums" | awk '{print $1}')"
    if [ -z "$expected" ]; then
        printf "Error: no checksum found for '%s' in checksums.txt\n" "$basename" >&2
        exit 1
    fi

    if command -v sha256sum >/dev/null 2>&1; then
        actual="$(sha256sum "$archive" | awk '{print $1}')"
    elif command -v shasum >/dev/null 2>&1; then
        actual="$(shasum -a 256 "$archive" | awk '{print $1}')"
    else
        printf "Warning: neither sha256sum nor shasum found, skipping checksum verification.\n" >&2
        return 0
    fi

    if [ "$expected" != "$actual" ]; then
        printf "Error: checksum mismatch!\n" >&2
        printf "  expected: %s\n" "$expected" >&2
        printf "  actual:   %s\n" "$actual" >&2
        exit 1
    fi
    printf "  Checksum verified.\n"
}

main() {
    os="$(detect_os)"
    arch="$(detect_arch)"

    printf "Detecting platform... %s/%s\n" "$os" "$arch"

    tag="$(latest_version)"
    if [ -z "$tag" ]; then
        printf "Error: could not determine latest release. Check https://github.com/%s/releases\n" "$REPO" >&2
        exit 1
    fi
    # Strip leading 'v' for the archive filename.
    version="${tag#v}"

    printf "Latest release: %s\n" "$tag"

    # Build download URLs.
    archive="devtether_${version}_${os}_${arch}.tar.gz"
    base_url="https://github.com/${REPO}/releases/download/${tag}"
    archive_url="${base_url}/${archive}"
    checksums_url="${base_url}/checksums.txt"

    # Create a temporary directory for the download.
    tmpdir="$(mktemp -d)"
    trap 'rm -rf "$tmpdir"' EXIT

    printf "Downloading %s...\n" "$archive"
    download "$archive_url" "${tmpdir}/${archive}"
    download "$checksums_url" "${tmpdir}/checksums.txt"

    verify_checksum "${tmpdir}/${archive}" "${tmpdir}/checksums.txt"

    # Extract and install.
    mkdir -p "$INSTALL_DIR"
    tar -xzf "${tmpdir}/${archive}" -C "$tmpdir"
    mv "${tmpdir}/devtether" "${INSTALL_DIR}/devtether"
    chmod +x "${INSTALL_DIR}/devtether"

    printf "\nInstalled devtether %s to %s/devtether\n" "$tag" "$INSTALL_DIR"

    # Post-install hints.
    case ":$PATH:" in
        *":${INSTALL_DIR}:"*) ;;
        *)
            printf "\n  Add to your PATH:\n"
            printf "    export PATH=\"%s:\$PATH\"\n" "$INSTALL_DIR"
            ;;
    esac

    if [ "$os" = "linux" ]; then
        printf "\n  To bind to ports 80/53 without root:\n"
        printf "    sudo setcap cap_net_bind_service=+ep %s/devtether\n" "$INSTALL_DIR"
    fi

    printf "\n  Get started:\n"
    printf "    devtether init\n"
    printf "    devtether up\n"
}

main
