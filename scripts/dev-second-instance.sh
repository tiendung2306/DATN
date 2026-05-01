#!/usr/bin/env bash
set -euo pipefail

# Run exactly one second instance on Linux.
# Assumes node 1 is already running (for example: cd app && wails dev -appargs "-write-bootstrap .local/dev-bootstrap.txt")
#
# Defaults:
# - db:   app/.local/dev-wails-sibling.db
# - port: 4002
#
# Bootstrap resolution order:
# 1) --bootstrap / --bootstrap-file
# 2) app/.local/dev-bootstrap.txt
# 3) DATN_BOOTSTRAP env
#
# Examples:
#   ./scripts/dev-second-instance.sh
#   ./scripts/dev-second-instance.sh --headless
#   ./scripts/dev-second-instance.sh --use-go-run
#   ./scripts/dev-second-instance.sh --no-auto-build --use-go-run

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
APP_DIR="${REPO_ROOT}/app"
LOCAL_DIR="${APP_DIR}/.local"
DB_PATH="${LOCAL_DIR}/dev-wails-sibling.db"
PORT="4002"

BOOTSTRAP=""
BOOTSTRAP_FILE=""
HEADLESS="false"
USE_GO_RUN="false"
AUTO_BUILD="true"

EXE_DEFAULT_LINUX="${APP_DIR}/build/bin/SecureP2P"
EXE_DEFAULT_WIN="${APP_DIR}/build/bin/SecureP2P.exe"
EXE=""

print_help() {
  cat <<'EOF'
Usage: dev-second-instance.sh [options]

Options:
  --bootstrap <multiaddr>       Explicit bootstrap multiaddr
  --bootstrap-file <path>       Read first line as bootstrap (relative to app/ if not absolute)
  --headless                    Add --headless when running app
  --exe <path>                  Explicit executable path
  --use-go-run                  Use "go run . -- ..." instead of executable
  --auto-build                  Build with "wails build" before launch (default)
  --no-auto-build               Skip auto build
  -h, --help                    Show this help
EOF
}

resolve_from_app_dir() {
  local path_like="$1"
  if [[ -z "${path_like}" ]]; then
    echo ""
    return 0
  fi
  if [[ "${path_like}" = /* ]]; then
    echo "${path_like}"
  else
    local trimmed="${path_like#./}"
    echo "${APP_DIR}/${trimmed}"
  fi
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --bootstrap)
      BOOTSTRAP="${2:-}"
      shift 2
      ;;
    --bootstrap-file)
      BOOTSTRAP_FILE="${2:-}"
      shift 2
      ;;
    --headless)
      HEADLESS="true"
      shift
      ;;
    --exe)
      EXE="${2:-}"
      shift 2
      ;;
    --use-go-run)
      USE_GO_RUN="true"
      shift
      ;;
    --auto-build)
      AUTO_BUILD="true"
      shift
      ;;
    --no-auto-build)
      AUTO_BUILD="false"
      shift
      ;;
    -h|--help)
      print_help
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      print_help
      exit 1
      ;;
  esac
done

mkdir -p "${LOCAL_DIR}"

if [[ -n "${BOOTSTRAP_FILE}" ]]; then
  BOOTSTRAP_FILE="$(resolve_from_app_dir "${BOOTSTRAP_FILE}")"
fi
if [[ -n "${BOOTSTRAP_FILE}" && -f "${BOOTSTRAP_FILE}" ]]; then
  BOOTSTRAP="$(sed -n '1p' "${BOOTSTRAP_FILE}" | tr -d '\r' | xargs || true)"
fi

DEFAULT_BOOTSTRAP_FILE="${LOCAL_DIR}/dev-bootstrap.txt"
if [[ -z "${BOOTSTRAP}" && -f "${DEFAULT_BOOTSTRAP_FILE}" ]]; then
  BOOTSTRAP="$(sed -n '1p' "${DEFAULT_BOOTSTRAP_FILE}" | tr -d '\r' | xargs || true)"
fi

if [[ -z "${BOOTSTRAP}" && -n "${DATN_BOOTSTRAP:-}" ]]; then
  BOOTSTRAP="$(echo "${DATN_BOOTSTRAP}" | xargs)"
fi

RUN_ARGS=(--db "${DB_PATH}" --p2p-port "${PORT}")
if [[ -n "${BOOTSTRAP}" ]]; then
  RUN_ARGS+=(--bootstrap "${BOOTSTRAP}")
fi
if [[ "${HEADLESS}" == "true" ]]; then
  RUN_ARGS+=(--headless)
fi

if [[ -z "${EXE}" ]]; then
  if [[ -x "${EXE_DEFAULT_LINUX}" ]]; then
    EXE="${EXE_DEFAULT_LINUX}"
  else
    EXE="${EXE_DEFAULT_WIN}"
  fi
fi

HAS_EXE="false"
if [[ -f "${EXE}" ]]; then
  HAS_EXE="true"
fi

if [[ "${USE_GO_RUN}" == "false" && "${AUTO_BUILD}" == "true" ]]; then
  echo "Auto build: wails build (instance 2)"
  (
    cd "${APP_DIR}"
    wails build
  )
  if [[ -f "${EXE}" ]]; then
    HAS_EXE="true"
  fi
fi

if [[ "${HAS_EXE}" == "false" && "${USE_GO_RUN}" == "false" ]]; then
  echo "Executable not found: ${EXE}"
  echo "Falling back to go run (or pass --exe / --use-go-run)."
  USE_GO_RUN="true"
fi

echo "Second instance: db=${DB_PATH} port=${PORT} headless=${HEADLESS}"
if [[ -n "${BOOTSTRAP}" ]]; then
  echo "Bootstrap: ${BOOTSTRAP}"
fi

if [[ "${HAS_EXE}" == "true" && "${USE_GO_RUN}" == "false" ]]; then
  (
    cd "${APP_DIR}"
    "${EXE}" "${RUN_ARGS[@]}"
  )
  exit 0
fi

(
  cd "${APP_DIR}"
  go run . -- "${RUN_ARGS[@]}"
)
