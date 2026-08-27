# shellcheck shell=bash
# Shared shell helpers for scripts/deploy-aws.sh and scripts/destroy-aws.sh.
# Sourced, never executed directly (no shebang, no `set -e` here — the
# caller owns its own `set -euo pipefail`).
#
# Deliberately dependency-free beyond `aws`/`curl`/`go`/`openssl`, which the
# callers already require: no jq (see scripts/smoke.sh's own precedent),
# no third-party bash frameworks.

# log/warn print a tagged, timestamped line to stdout/stderr respectively.
# err additionally exits the whole script with status 1 — every caller uses
# it as the single "fail fast with an actionable message" primitive instead
# of letting a raw AWS CLI stack trace reach the user.
log()  { printf '[%s] [%s] %s\n' "$SCRIPT_NAME" "$(date -u +%H:%M:%S)" "$1"; }
warn() { printf '[%s] [%s] WARNING: %s\n' "$SCRIPT_NAME" "$(date -u +%H:%M:%S)" "$1" >&2; }
err()  { printf '[%s] [%s] ERROR: %s\n' "$SCRIPT_NAME" "$(date -u +%H:%M:%S)" "$1" >&2; exit 1; }

# require_cmd fails fast, naming exactly which tool is missing and why it is
# needed, instead of letting the script die later on an obscure
# "command not found".
require_cmd() {
  local cmd="$1" reason="$2"
  command -v "$cmd" >/dev/null 2>&1 || err "required tool '$cmd' not found on PATH ($reason)."
}

# validate_name enforces the conservative charset every AWS resource name
# this script creates (Lambda function, IAM role, DynamoDB table) accepts,
# so a typo'd --function-name/--table-name/--role-name fails locally with a
# clear message instead of as a confusing AWS API error mid-deploy.
validate_name() {
  local label="$1" value="$2"
  [[ "$value" =~ ^[A-Za-z0-9_.-]{1,140}$ ]] || err "$label '$value' is invalid: only letters, digits, '-', '_', '.' are allowed."
}

# resolve_region fills REGION (already exported by the caller) from, in
# order: an explicit --region flag/REGION env var (left untouched if
# already set), the AWS CLI's own configured default region, or the
# project's documented default (design.md: AWS_REGION default us-east-1).
resolve_region() {
  if [ -n "${REGION:-}" ]; then
    return
  fi
  REGION="$(aws configure get region 2>/dev/null || true)"
  if [ -z "$REGION" ]; then
    REGION="us-east-1"
  fi
}

# preflight_aws_credentials verifies the aws CLI is present and that
# credentials actually resolve (aws sts get-caller-identity), printing the
# resolved account/ARN so the operator can confirm which AWS account is
# about to be mutated before anything else runs.
preflight_aws_credentials() {
  require_cmd aws "install the AWS CLI v2: https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html"
  require_cmd curl "used to poll the deployed API and by scripts/smoke.sh"

  local identity
  if ! identity=$(aws sts get-caller-identity --region "$REGION" --output json 2>&1); then
    err "AWS credentials do not resolve (aws sts get-caller-identity failed): $identity
Configure credentials first, e.g. 'aws configure' or exporting AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY/AWS_PROFILE."
  fi

  ACCOUNT_ID=$(printf '%s' "$identity" | grep -o '"Account"[[:space:]]*:[[:space:]]*"[^"]*"' | head -n1 | sed -E 's/.*"([0-9]+)"$/\1/')
  local caller_arn
  caller_arn=$(printf '%s' "$identity" | grep -o '"Arn"[[:space:]]*:[[:space:]]*"[^"]*"' | head -n1 | sed -E 's/.*"([^"]*)"$/\1/')
  [ -n "$ACCOUNT_ID" ] || err "could not parse an AWS account id out of sts get-caller-identity's response: $identity"

  log "AWS credentials resolved: account=$ACCOUNT_ID identity=$caller_arn region=$REGION"
}

# to_win_path converts a Git-Bash-style POSIX path (e.g. /d/foo/bar) into a
# native Windows path (D:\foo\bar) for tools, like a plain powershell.exe
# fallback, that do not understand MSYS path mangling. Prefers `cygpath`
# when present (exact); falls back to a best-effort single-drive-letter
# rewrite otherwise.
to_win_path() {
  local p="$1"
  if command -v cygpath >/dev/null 2>&1; then
    cygpath -w "$p"
    return
  fi
  case "$p" in
    /*/*)
      local drive="${p:1:1}"
      local rest="${p:2}"
      printf '%s:%s\n' "${drive^^}" "${rest//\//\\}"
      ;;
    *)
      printf '%s\n' "$p"
      ;;
  esac
}

# aws_uri renders a local path as one of the URIs the AWS CLI expects for a
# path-valued parameter: `file://` for text (--assume-role-policy-document,
# --policy-document, --environment) and `fileb://` for binary (--zip-file).
#
# Under Git Bash / MSYS the shell is POSIX but the AWS CLI is a NATIVE
# WINDOWS binary, so it cannot resolve a path like /tmp/x: it looks for
# \tmp\x on the current drive and fails with "No such file or directory".
# The conversion is deliberately gated on the platform rather than applied
# everywhere, because to_win_path's cygpath-less fallback would happily
# rewrite a genuine Linux /tmp/x into "T:mp\x".
#
# Both schemes go through this one function on purpose: they differ by four
# characters, which is exactly how the binary one gets missed when the text
# one is fixed on its own.
aws_uri() {
  local scheme="$1" p="$2"
  case "$(uname -s)" in
    MINGW* | MSYS* | CYGWIN*) printf '%s://%s\n' "$scheme" "$(to_win_path "$p")" ;;
    *) printf '%s://%s\n' "$scheme" "$p" ;;
  esac
}

# aws_file_uri is aws_uri for text payloads.
aws_file_uri() { aws_uri file "$1"; }

# aws_fileb_uri is aws_uri for binary payloads (the Lambda deployment zip).
aws_fileb_uri() { aws_uri fileb "$1"; }

# create_zip packages bin_path (a single file) into zip_path with the entry
# name bin_name at the archive root — the exact shape provided.al2023
# requires for the handler binary. Prefers the `zip` CLI; falls back to
# Windows PowerShell's Compress-Archive when `zip` is not installed (common
# on a bare Git Bash / Windows setup with no MSYS extras).
create_zip() {
  local zip_path="$1" bin_path="$2" bin_name="$3"

  rm -f "$zip_path"

  if command -v zip >/dev/null 2>&1; then
    ( cd "$(dirname "$bin_path")" && zip -q -X "$zip_path" "$bin_name" )
    return
  fi

  if command -v powershell.exe >/dev/null 2>&1; then
    local win_bin win_zip
    win_bin="$(to_win_path "$bin_path")"
    win_zip="$(to_win_path "$zip_path")"
    powershell.exe -NoProfile -NonInteractive -Command \
      "Compress-Archive -Path '${win_bin}' -DestinationPath '${win_zip}' -Force" \
      || err "powershell.exe Compress-Archive failed while building $zip_path"
    return
  fi

  err "Neither 'zip' nor 'powershell.exe' is available to build the Lambda deployment archive. Install zip (e.g. via scoop/choco 'zip') and re-run."
}
