#!/usr/bin/env bash
# Tear down everything scripts/deploy-aws.sh created, so a demo deployment
# leaves zero cost behind: the Function URL, the Lambda function, its
# CloudWatch log group, the IAM execution role (managed policy detached +
# inline policy removed, then the role itself), and finally the DynamoDB
# table (which permanently deletes every seeded user and placed bet).
#
# This is destructive and requires the explicit --yes flag; without it, the
# script only prints what it WOULD delete and exits non-zero.
#
# Usage:
#   scripts/destroy-aws.sh --yes [options]
#
# Options (all optional; every one also reads an environment variable of
# the same name in SCREAMING_SNAKE_CASE) — pass the SAME names used at
# deploy time:
#   --function-name NAME   Lambda function name (default: apuesta-total-api)
#   --table-name NAME      DynamoDB table name (default: apuesta-total)
#   --role-name NAME       IAM execution role name
#                           (default: apuesta-total-lambda-role)
#   --region REGION        AWS region (default: the AWS CLI's configured
#                           default region, or us-east-1)
#   --yes                  Required. Confirms the destructive operation.
#   -h, --help              Print this help and exit
#
# Every AWS call below tolerates "already gone" (a resource missing because
# a previous run already deleted it, or it was never created) so this
# script is itself safe to re-run.
set -euo pipefail

SCRIPT_NAME="destroy-aws"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/common.sh
source "$SCRIPT_DIR/lib/common.sh"

FUNCTION_NAME="${FUNCTION_NAME:-apuesta-total-api}"
TABLE_NAME="${TABLE_NAME:-apuesta-total}"
ROLE_NAME="${ROLE_NAME:-apuesta-total-lambda-role}"
REGION="${REGION:-}"
CONFIRMED=false

usage() {
  sed -n '2,/^set -euo pipefail$/p' "$SCRIPT_DIR/destroy-aws.sh" | sed '$d' | sed 's/^# \{0,1\}//'
}

parse_args() {
  while [ $# -gt 0 ]; do
    case "$1" in
      --function-name) FUNCTION_NAME="$2"; shift 2 ;;
      --table-name)    TABLE_NAME="$2"; shift 2 ;;
      --role-name)     ROLE_NAME="$2"; shift 2 ;;
      --region)        REGION="$2"; shift 2 ;;
      --yes)           CONFIRMED=true; shift ;;
      -h|--help)       usage; exit 0 ;;
      *) err "unknown argument: $1 (run with --help for usage)" ;;
    esac
  done
}

preflight() {
  validate_name "--function-name" "$FUNCTION_NAME"
  validate_name "--table-name" "$TABLE_NAME"
  validate_name "--role-name" "$ROLE_NAME"
  resolve_region
  preflight_aws_credentials
}

confirm_or_exit() {
  cat <<EOF
This will PERMANENTLY delete, in region $REGION:
  - Lambda function       : $FUNCTION_NAME (and its Function URL)
  - CloudWatch log group  : /aws/lambda/$FUNCTION_NAME
  - IAM role              : $ROLE_NAME (managed + inline policies removed first)
  - DynamoDB table        : $TABLE_NAME (ALL seeded users and placed bets are lost)
EOF
  if [ "$CONFIRMED" != true ]; then
    err "refusing to proceed without --yes. Re-run with --yes once you have reviewed the list above."
  fi
  log "Confirmed via --yes. Proceeding with teardown."
}

delete_function_url() {
  local output
  log "Deleting the Function URL config (if any)..."
  if ! output=$(aws lambda delete-function-url-config --function-name "$FUNCTION_NAME" --region "$REGION" 2>&1); then
    [[ "$output" == *"ResourceNotFoundException"* ]] || err "aws lambda delete-function-url-config failed: $output"
    log "  no Function URL existed."
  fi
}

delete_lambda_function() {
  local output
  log "Deleting Lambda function '$FUNCTION_NAME' (if any)..."
  if ! output=$(aws lambda delete-function --function-name "$FUNCTION_NAME" --region "$REGION" 2>&1); then
    [[ "$output" == *"ResourceNotFoundException"* ]] || err "aws lambda delete-function failed: $output"
    log "  no such function."
  fi
}

delete_log_group() {
  local log_group="/aws/lambda/$FUNCTION_NAME" output
  log "Deleting log group '$log_group' (if any)..."
  # See ensure_log_retention in deploy-aws.sh: Git Bash rewrites a
  # leading-slash argument into a Windows path before a native binary sees
  # it, which would make this delete miss the real log group and leave it
  # behind — the one thing a teardown script must never do.
  if ! output=$(MSYS_NO_PATHCONV=1 aws logs delete-log-group --log-group-name "$log_group" --region "$REGION" 2>&1); then
    [[ "$output" == *"ResourceNotFoundException"* ]] || err "aws logs delete-log-group failed: $output"
    log "  no such log group."
  fi
}

delete_iam_role() {
  local output
  log "Detaching AWSLambdaBasicExecutionRole from '$ROLE_NAME' (if attached)..."
  if ! output=$(aws iam detach-role-policy \
      --role-name "$ROLE_NAME" \
      --policy-arn arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole 2>&1); then
    [[ "$output" == *"NoSuchEntity"* ]] || err "aws iam detach-role-policy failed: $output"
  fi

  log "Removing the inline dynamodb policy from '$ROLE_NAME' (if present)..."
  if ! output=$(aws iam delete-role-policy \
      --role-name "$ROLE_NAME" \
      --policy-name "${ROLE_NAME}-dynamodb" 2>&1); then
    [[ "$output" == *"NoSuchEntity"* ]] || err "aws iam delete-role-policy failed: $output"
  fi

  log "Deleting IAM role '$ROLE_NAME' (if any)..."
  if ! output=$(aws iam delete-role --role-name "$ROLE_NAME" 2>&1); then
    [[ "$output" == *"NoSuchEntity"* ]] || err "aws iam delete-role failed: $output"
    log "  no such role."
  fi
}

delete_table() {
  local output
  log "Deleting DynamoDB table '$TABLE_NAME' (if any)..."
  if ! output=$(aws dynamodb delete-table --table-name "$TABLE_NAME" --region "$REGION" 2>&1); then
    [[ "$output" == *"ResourceNotFoundException"* ]] || err "aws dynamodb delete-table failed: $output"
    log "  no such table."
  fi
}

print_summary() {
  cat <<EOF

========================================================================
 Teardown complete for region $REGION:
   - Function URL / Lambda function : $FUNCTION_NAME
   - Log group                      : /aws/lambda/$FUNCTION_NAME
   - IAM role                       : $ROLE_NAME
   - DynamoDB table                 : $TABLE_NAME
 Nothing billable from this deployment should remain in this account.
========================================================================
EOF
}

main() {
  parse_args "$@"
  preflight
  confirm_or_exit

  delete_function_url
  delete_lambda_function
  delete_log_group
  delete_iam_role
  delete_table

  print_summary
}

main "$@"
