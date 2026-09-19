#!/bin/sh
# DevTether uninstall script.
# Usage:
#   curl -sSfL https://raw.githubusercontent.com/ivin-titus/devtether/main/scripts/uninstall.sh | sh
#   curl -sSfL ... | sh -s -- -b /usr/local/bin
#
# Removes the DevTether binary from the system.

set -eu

INSTALL_DIR="${HOME}/.local/bin"

usage() {
    printf "Usage: %s [-b install_dir]\n" "$0"
    printf "  -b    Installation directory to remove from (default: %s)\n" "${INSTALL_DIR}"
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

BINARY="${INSTALL_DIR}/devtether"

if [ ! -f "$BINARY" ]; then
    printf "DevTether binary not found at %s\n" "$BINARY"
    printf "Nothing to uninstall.\n"
    exit 0
fi

printf "Removing %s...\n" "$BINARY"
rm -f "$BINARY"

# Reverse the system DNS configuration written by `devtether init`.
SUDO=""
if command -v sudo >/dev/null 2>&1; then
    SUDO="sudo"
fi

dns_removed=0

remove_dns_config() {
    config="$1"
    [ -e "$config" ] || return 0
    printf "Removing %s...\n" "$config"
    if ! $SUDO rm -f "$config"; then
        printf "  Could not remove %s (requires sudo).\n" "$config"
        return 0
    fi
    dns_removed=1
}

remove_dns_config /etc/systemd/resolved.conf.d/devtether.conf
remove_dns_config /etc/dnsmasq.d/devtether.conf
remove_dns_config /etc/resolver/localhost

if [ "$dns_removed" -eq 1 ] && command -v systemctl >/dev/null 2>&1; then
    $SUDO systemctl restart systemd-resolved 2>/dev/null || true
    $SUDO systemctl restart dnsmasq 2>/dev/null || true
fi

printf "\nDevTether has been uninstalled successfully.\n"
printf "\nNote: Your configuration files (e.g. devtether.yaml) and logs (.logs/) were not removed.\n"
