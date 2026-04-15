#!/bin/bash

# AgentSmith-HUB Update Script
# Download latest GitHub release for current architecture, install, then restart.

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

print_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

print_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO="${AGENTSMITH_REPO:-EBWi11/AgentSmith-HUB}"
RELEASE_API="https://api.github.com/repos/${REPO}/releases/latest"
INCLUDE_CONFIG=false
KEEP_BACKUP=true
START_ARGS=""
CONFIG_DIR="${AGENTSMITH_CONFIG_DIR:-}"

TMP_DIR=""
BACKUP_DIR=""

cleanup() {
    if [ -n "${TMP_DIR}" ] && [ -d "${TMP_DIR}" ]; then
        rm -rf "${TMP_DIR}"
    fi
}
trap cleanup EXIT

detect_architecture() {
    local arch
    arch="$(uname -m)"
    case "$arch" in
        x86_64)
            echo "amd64"
            ;;
        aarch64|arm64)
            echo "arm64"
            ;;
        *)
            print_warn "Unknown architecture: ${arch}, defaulting to amd64"
            echo "amd64"
            ;;
    esac
}

show_help() {
    cat <<EOF
AgentSmith-HUB Update Script

Usage: $0 [OPTIONS]

Options:
  --help, -h              Show this help message
  --repo <owner/repo>     Override release repo (default: ${REPO})
  --include-config         Also replace local config directory
  --no-backup              Do not keep backup files
  --start-args "<args>"    Extra args passed to start script
  --config-dir "<path>"    Config directory to update when --include-config is set
  --dry-run                Only show detected release and assets

Environment:
  AGENTSMITH_REPO          Same as --repo
  AGENTSMITH_CONFIG_DIR    Same as --config-dir

Behavior:
  1) Detect latest release from GitHub
  2) Download matching archive for current architecture
  3) Verify checksum when available
  4) Stop local service
  5) Replace local files (config excluded by default)
  6) Start local service
EOF
}

require_command() {
    local cmd="$1"
    if ! command -v "$cmd" >/dev/null 2>&1; then
        print_error "Required command not found: ${cmd}"
        exit 1
    fi
}

sha256_file() {
    local file="$1"
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$file" | awk '{print $1}'
    elif command -v shasum >/dev/null 2>&1; then
        shasum -a 256 "$file" | awk '{print $1}'
    else
        print_warn "No sha256 tool found (sha256sum/shasum), skip checksum verification"
        echo ""
    fi
}

stop_service() {
    if [ -x "${SCRIPT_DIR}/stop.sh" ]; then
        print_info "Stopping AgentSmith-HUB via stop.sh ..."
        "${SCRIPT_DIR}/stop.sh"
    else
        print_warn "stop.sh not found, trying process kill fallback ..."
        local pids
        pids="$(pgrep -f agentsmith-hub 2>/dev/null || true)"
        if [ -n "${pids}" ]; then
            echo "${pids}" | xargs kill -TERM 2>/dev/null || true
            sleep 3
            pids="$(pgrep -f agentsmith-hub 2>/dev/null || true)"
            if [ -n "${pids}" ]; then
                echo "${pids}" | xargs kill -KILL 2>/dev/null || true
            fi
        fi
    fi
}

start_service() {
    local starter=""
    if [ -x "${SCRIPT_DIR}/start.sh" ]; then
        starter="${SCRIPT_DIR}/start.sh"
    elif [ -x "${SCRIPT_DIR}/run.sh" ]; then
        starter="${SCRIPT_DIR}/run.sh"
    fi

    if [ -z "${starter}" ]; then
        print_error "No start script found (start.sh or run.sh)"
        exit 1
    fi

    print_info "Starting AgentSmith-HUB via $(basename "${starter}") ..."
    if [ -n "${START_ARGS}" ]; then
        # shellcheck disable=SC2086
        "${starter}" ${START_ARGS}
    else
        "${starter}"
    fi
}

replace_path() {
    local src="$1"
    local dst="$2"

    if [ ! -e "${src}" ]; then
        return 0
    fi

    if [ -e "${dst}" ] && [ "${KEEP_BACKUP}" = true ] && [ -n "${BACKUP_DIR}" ]; then
        mkdir -p "${BACKUP_DIR}"
        cp -a "${dst}" "${BACKUP_DIR}/" || true
    fi

    rm -rf "${dst}"
    cp -a "${src}" "${dst}"
}

main() {
    require_command curl
    require_command tar

    local dry_run=false

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --help|-h)
                show_help
                exit 0
                ;;
            --repo)
                REPO="$2"
                RELEASE_API="https://api.github.com/repos/${REPO}/releases/latest"
                shift 2
                ;;
            --include-config)
                INCLUDE_CONFIG=true
                shift
                ;;
            --no-backup)
                KEEP_BACKUP=false
                shift
                ;;
            --start-args)
                START_ARGS="$2"
                shift 2
                ;;
            --config-dir)
                CONFIG_DIR="$2"
                shift 2
                ;;
            --dry-run)
                dry_run=true
                shift
                ;;
            *)
                print_error "Unknown option: $1"
                show_help
                exit 1
                ;;
        esac
    done

    local arch
    arch="$(detect_architecture)"
    local asset_name="agentsmith-hub-${arch}.tar.gz"
    local checksum_name="${asset_name}.sha256"

    print_info "Repository: ${REPO}"
    print_info "Detected architecture: ${arch}"
    print_info "Fetching latest release metadata ..."

    local release_json
    release_json="$(curl -fsSL -H "Accept: application/vnd.github+json" "${RELEASE_API}")"

    local tag
    tag="$(echo "${release_json}" | sed -n 's/.*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)"
    if [ -z "${tag}" ]; then
        print_error "Could not parse latest release tag from GitHub API"
        exit 1
    fi

    local asset_url
    asset_url="$(echo "${release_json}" | sed -n "s/.*\"browser_download_url\":[[:space:]]*\"\\([^\"]*${asset_name}\\)\".*/\\1/p" | head -n 1)"
    if [ -z "${asset_url}" ]; then
        print_error "Release ${tag} does not contain asset: ${asset_name}"
        exit 1
    fi

    local checksum_url
    checksum_url="$(echo "${release_json}" | sed -n "s/.*\"browser_download_url\":[[:space:]]*\"\\([^\"]*${checksum_name}\\)\".*/\\1/p" | head -n 1)"

    print_info "Latest release: ${tag}"
    print_info "Asset: ${asset_name}"
    print_info "Asset URL: ${asset_url}"
    if [ -n "${checksum_url}" ]; then
        print_info "Checksum URL: ${checksum_url}"
    else
        print_warn "Checksum asset not found, verification will be skipped"
    fi

    if [ "${dry_run}" = true ]; then
        exit 0
    fi

    TMP_DIR="$(mktemp -d)"
    local archive_path="${TMP_DIR}/${asset_name}"
    local checksum_path="${TMP_DIR}/${checksum_name}"
    local extracted_dir="${TMP_DIR}/agentsmith-hub"
    local effective_config_dir="${CONFIG_DIR}"

    print_info "Downloading release archive ..."
    curl -fL "${asset_url}" -o "${archive_path}"

    if [ -n "${checksum_url}" ]; then
        print_info "Downloading checksum ..."
        curl -fL "${checksum_url}" -o "${checksum_path}"

        local expected actual
        expected="$(awk '{print $1}' "${checksum_path}")"
        actual="$(sha256_file "${archive_path}")"
        if [ -n "${actual}" ] && [ "${expected}" != "${actual}" ]; then
            print_error "Checksum verification failed"
            print_error "Expected: ${expected}"
            print_error "Actual:   ${actual}"
            exit 1
        fi
        if [ -n "${actual}" ]; then
            print_info "Checksum verification passed"
        fi
    fi

    print_info "Extracting archive ..."
    tar -xzf "${archive_path}" -C "${TMP_DIR}"
    if [ ! -d "${extracted_dir}" ]; then
        print_error "Unexpected archive structure: missing directory 'agentsmith-hub'"
        exit 1
    fi

    if [ -z "${effective_config_dir}" ]; then
        # Match deployment docs and start.sh default behavior.
        if [ -d "/opt/hub_config" ]; then
            effective_config_dir="/opt/hub_config"
        else
            effective_config_dir="${SCRIPT_DIR}/config"
        fi
    fi

    BACKUP_DIR="${SCRIPT_DIR}/backup-${tag}-$(date +%Y%m%d%H%M%S)"
    if [ "${KEEP_BACKUP}" = true ]; then
        print_info "Backup directory: ${BACKUP_DIR}"
    fi

    # Stop before replacing binary/shared libs to avoid file-in-use issues.
    stop_service

    print_info "Replacing local files ..."
    replace_path "${extracted_dir}/agentsmith-hub" "${SCRIPT_DIR}/agentsmith-hub"
    replace_path "${extracted_dir}/web" "${SCRIPT_DIR}/web"
    replace_path "${extracted_dir}/lib" "${SCRIPT_DIR}/lib"
    replace_path "${extracted_dir}/nginx" "${SCRIPT_DIR}/nginx"
    replace_path "${extracted_dir}/VERSION" "${SCRIPT_DIR}/VERSION"
    replace_path "${extracted_dir}/LICENSE" "${SCRIPT_DIR}/LICENSE"
    replace_path "${extracted_dir}/README.md" "${SCRIPT_DIR}/README.md"
    replace_path "${extracted_dir}/start.sh" "${SCRIPT_DIR}/start.sh"
    replace_path "${extracted_dir}/stop.sh" "${SCRIPT_DIR}/stop.sh"
    replace_path "${extracted_dir}/update.sh" "${SCRIPT_DIR}/update.sh"

    if [ "${INCLUDE_CONFIG}" = true ]; then
        print_warn "Replacing config directory because --include-config is set"
        print_info "Config target: ${effective_config_dir}"
        replace_path "${extracted_dir}/config" "${effective_config_dir}"
    else
        print_info "Keeping existing config directory unchanged"
    fi

    chmod +x "${SCRIPT_DIR}/agentsmith-hub" 2>/dev/null || true
    chmod +x "${SCRIPT_DIR}/start.sh" 2>/dev/null || true
    chmod +x "${SCRIPT_DIR}/stop.sh" 2>/dev/null || true
    chmod +x "${SCRIPT_DIR}/update.sh" 2>/dev/null || true

    start_service

    print_info "Update completed: ${tag}"
    if [ "${KEEP_BACKUP}" = true ]; then
        print_info "Backup kept at: ${BACKUP_DIR}"
    fi
}

main "$@"
