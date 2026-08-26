#!/usr/bin/env bash
# Deploy the World Cup 2026 Betting API to AWS Lambda (Function URL) + a
# single-table DynamoDB, staying inside the AWS Always-Free tier
# (design.md's "Local vs Lambda" and "DynamoDB Single Table" sections).
#
# What this script creates (idempotent — safe to re-run):
#   1. A DynamoDB table (PK/SK + EmailIndex GSI) — PROVISIONED 10/10 RCU/WCU
#      on the table + 5/5 on the index (15/15 total), inside the permanent
#      25/25 Always-Free allowance. On-demand billing is deliberately NOT
#      used: it has no free tier at all.
#   2. An IAM execution role: the AWS-managed AWSLambdaBasicExecutionRole
#      (CloudWatch Logs only) plus ONE inline policy scoped to exactly this
#      table's ARN and its EmailIndex ARN, granting only the actions the
#      deployed binary actually calls at runtime (verified against
#      internal/adapters/dynamo/*.go): GetItem, PutItem, UpdateItem, Query,
#      TransactWriteItems. Never dynamodb:* and never Resource: "*".
#   3. A Lambda function (runtime provided.al2023, arch arm64, handler
#      "bootstrap", cross-compiled from ./cmd/api) with a public Function
#      URL (auth-type NONE — the application enforces its own JWT on every
#      mutating route, so this is a deliberate, documented tradeoff, not an
#      oversight).
#   4. A 7-day retention policy on the function's CloudWatch log group, so
#      logs cannot silently accumulate cost past the 5GB Always-Free
#      allowance.
#
# Cost target: $0/month on permanent AWS Always-Free allowances (Lambda 1M
# requests + 400,000 GB-s, DynamoDB 25 RCU/WCU provisioned + 25GB storage,
# CloudWatch Logs 5GB). Hard cap: ~$5/month even if this demo's traffic
# occasionally spills past the free tier. This script never creates a NAT
# gateway, VPC, ElastiCache, or anything else outside that tier (design.md
# D14: no cache, no queues — a NAT gateway alone would already blow the cap
# roughly 6x over).
#
# JWT_SECRET handling (documented choice, not an oversight): the secret is
# NEVER hardcoded or committed. If the JWT_SECRET environment variable is
# already set when this script runs, that value is used as-is. Otherwise
# the script generates a strong random secret on first run and caches it at
# .deploy/jwt-secret (repo-local, gitignored, chmod 600) so every later
# re-run of this script reuses the SAME secret — regenerating it on every
# deploy would silently invalidate every token issued by the previous
# deployment. This is the simplest defensible option for a take-home demo;
# a production deployment would read it from AWS SSM Parameter Store or
# Secrets Manager instead.
#
# Seeding: rather than re-implementing "create table with this exact
# schema" a second time in bash/aws-cli JSON (risking drift from the
# already-tested Go schema), this script cross-compiles and runs the
# existing, already-tested cmd/seed binary NATIVELY (the deployer's own
# OS/arch, not arm64/linux) against the real table, using the deployer's
# own ambient AWS credentials (the same ones aws sts get-caller-identity
# just verified). cmd/seed's dynamo.EnsureTable call is exactly step 1
# above (idempotent create-if-absent + wait ACTIVE + enable TTL), and its
# seeding is idempotent by construction (PutUserIfAbsent — SEED_RESET is
# left at its default "false", so a re-run never clobbers a played-with
# balance).
#
# Usage:
#   scripts/deploy-aws.sh [options]
#
# Options (all optional; every one also reads an environment variable of
# the same name in SCREAMING_SNAKE_CASE):
#   --function-name NAME   Lambda function name (default: apuesta-total-api)
#   --table-name NAME      DynamoDB table name (default: apuesta-total, the
#                           same default internal/platform/config/config.go
#                           uses for DYNAMO_TABLE)
#   --role-name NAME       IAM execution role name
#                           (default: apuesta-total-lambda-role)
#   --region REGION        AWS region (default: the AWS CLI's configured
#                           default region, or us-east-1)
#   --memory MB            Lambda memory size in MB (default: 512)
#   --timeout SECONDS      Lambda timeout in seconds (default: 10)
#   -h, --help             Print this help and exit
#
# Requires: aws CLI v2 (with resolvable credentials), go, curl, and either
# `zip` or Windows `powershell.exe` to build the deployment archive.
#
# This script is NEVER executed by the SDD implementer — only authored and
# statically validated (bash -n / shellcheck). The user runs it themselves
# with their own AWS credentials (proposal.md / tasks.md Phase 17).
set -euo pipefail

SCRIPT_NAME="deploy-aws"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
# shellcheck source=lib/common.sh
source "$SCRIPT_DIR/lib/common.sh"

FUNCTION_NAME="${FUNCTION_NAME:-apuesta-total-api}"
TABLE_NAME="${TABLE_NAME:-apuesta-total}"
ROLE_NAME="${ROLE_NAME:-apuesta-total-lambda-role}"
REGION="${REGION:-}"
MEMORY_SIZE="${MEMORY_SIZE:-512}"
LAMBDA_TIMEOUT="${LAMBDA_TIMEOUT:-10}"
LOG_RETENTION_DAYS=7
JWT_SECRET_CACHE_FILE="$REPO_ROOT/.deploy/jwt-secret"

usage() {
  # Prints this script's own header comment (everything between the
  # shebang and the first blank line after "set -euo pipefail") as the
  # help text, so the usage message and the documentation can never drift
  # apart.
  sed -n '2,/^set -euo pipefail$/p' "$SCRIPT_DIR/deploy-aws.sh" | sed '$d' | sed 's/^# \{0,1\}//'
}

parse_args() {
  while [ $# -gt 0 ]; do
    case "$1" in
      --function-name) FUNCTION_NAME="$2"; shift 2 ;;
      --table-name)    TABLE_NAME="$2"; shift 2 ;;
      --role-name)     ROLE_NAME="$2"; shift 2 ;;
      --region)        REGION="$2"; shift 2 ;;
      --memory)        MEMORY_SIZE="$2"; shift 2 ;;
      --timeout)       LAMBDA_TIMEOUT="$2"; shift 2 ;;
      -h|--help)       usage; exit 0 ;;
      *) err "unknown argument: $1 (run with --help for usage)" ;;
    esac
  done
}

preflight() {
  validate_name "--function-name" "$FUNCTION_NAME"
  validate_name "--table-name" "$TABLE_NAME"
  validate_name "--role-name" "$ROLE_NAME"
  [[ "$MEMORY_SIZE" =~ ^[0-9]+$ ]] || err "--memory must be a positive integer (MB), got '$MEMORY_SIZE'"
  [[ "$LAMBDA_TIMEOUT" =~ ^[0-9]+$ ]] || err "--timeout must be a positive integer (seconds), got '$LAMBDA_TIMEOUT'"

  require_cmd go "needed to cross-compile the Lambda binary and the native seeder"
  resolve_region
  preflight_aws_credentials

  log "Target: function='$FUNCTION_NAME' table='$TABLE_NAME' role='$ROLE_NAME' region='$REGION' memory=${MEMORY_SIZE}MB timeout=${LAMBDA_TIMEOUT}s"
}

resolve_jwt_secret() {
  if [ -n "${JWT_SECRET:-}" ]; then
    log "Using JWT_SECRET from the environment (not written to disk by this script)."
    return
  fi

  if [ -f "$JWT_SECRET_CACHE_FILE" ]; then
    JWT_SECRET="$(sed -n '1p' "$JWT_SECRET_CACHE_FILE")"
    [ -n "$JWT_SECRET" ] || err "$JWT_SECRET_CACHE_FILE exists but is empty; delete it and re-run, or export JWT_SECRET yourself."
    log "Reusing the JWT_SECRET cached at $JWT_SECRET_CACHE_FILE from a previous run."
    return
  fi

  log "No JWT_SECRET set; generating a new one and caching it at $JWT_SECRET_CACHE_FILE for future re-runs."
  mkdir -p "$(dirname "$JWT_SECRET_CACHE_FILE")"
  if command -v openssl >/dev/null 2>&1; then
    JWT_SECRET="$(openssl rand -hex 32)"
  elif [ -r /dev/urandom ]; then
    JWT_SECRET="$(od -An -tx1 -N32 /dev/urandom | tr -d ' \n')"
  else
    err "cannot generate a JWT secret: neither 'openssl' nor /dev/urandom is available. Export JWT_SECRET yourself and re-run."
  fi
  printf '%s' "$JWT_SECRET" > "$JWT_SECRET_CACHE_FILE"
  chmod 600 "$JWT_SECRET_CACHE_FILE" 2>/dev/null || true
  warn "Generated a new JWT_SECRET and stored it at $JWT_SECRET_CACHE_FILE — keep it safe, it is gitignored on purpose. Losing it just means the next redeploy issues a fresh one and invalidates old tokens."
}

build_lambda_zip() {
  log "Cross-compiling the Lambda handler (GOOS=linux GOARCH=arm64, tag lambda.norpc)..."
  ( cd "$REPO_ROOT" && GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -tags lambda.norpc -o "$WORKDIR/bootstrap" ./cmd/api )
  log "Packaging $WORKDIR/function.zip (entry: bootstrap)..."
  create_zip "$WORKDIR/function.zip" "$WORKDIR/bootstrap" "bootstrap"
}

# run_seed cross-compiles cmd/seed for the deployer's own OS/arch (never
# arm64/linux — this runs locally, not on Lambda) and executes it once
# against the real table, ensuring the table exists (10/10 + 5/5, TTL
# enabled) and the two demo users are present, without ever touching a
# balance that has already been played with.
run_seed() {
  local goos ext seed_bin
  goos="$(go env GOOS)"
  ext=""
  [ "$goos" = "windows" ] && ext=".exe"
  seed_bin="$WORKDIR/seed-runner${ext}"

  log "Building the seeder natively ($goos/$(go env GOARCH))..."
  ( cd "$REPO_ROOT" && go build -o "$seed_bin" ./cmd/seed )

  log "Running the seeder against table '$TABLE_NAME' in $REGION (creates the table if absent, then seeds demo users idempotently)..."
  # DYNAMO_ENDPOINT is explicitly cleared here regardless of what the
  # calling shell has set (e.g. left over from docker-compose testing):
  # empty means "talk to real AWS" (config.go). AWS_ACCESS_KEY_ID/SECRET
  # are deliberately NOT set, so the SDK's default credential chain picks
  # up the same ambient credentials preflight_aws_credentials just
  # verified.
  env -u AWS_ACCESS_KEY_ID -u AWS_SECRET_ACCESS_KEY \
    AWS_REGION="$REGION" \
    DYNAMO_TABLE="$TABLE_NAME" \
    DYNAMO_ENDPOINT="" \
    APP_ENV=aws \
    LOG_LEVEL=info \
    JWT_SECRET="$JWT_SECRET" \
    JWT_TTL=1h \
    BETSLIP_MIN_STAKE_AMOUNT=1 \
    BETSLIP_MAX_STAKE_AMOUNT=10000 \
    BETSLIP_CURRENCY_CODE=PEN \
    BETSLIP_MAX_SELECTIONS=20 \
    RATE_LIMIT=60-M \
    IDEMPOTENCY_TTL=24h \
    DEMO_ACCOUNT_INITIAL_BALANCE=1000 \
    SEED_RESET=false \
    "$seed_bin"
}

get_table_arn() {
  TABLE_ARN="$(aws dynamodb describe-table --table-name "$TABLE_NAME" --region "$REGION" --query 'Table.TableArn' --output text)"
  [ -n "$TABLE_ARN" ] && [ "$TABLE_ARN" != "None" ] || err "could not resolve the ARN of table '$TABLE_NAME' after seeding/creation."
  log "Table ARN: $TABLE_ARN"
}

# ensure_iam_role creates (or reuses) the execution role, then
# unconditionally re-attaches the managed policy and re-puts the inline
# policy — both operations are naturally idempotent in IAM, so this always
# converges the role to exactly the policy this script expects, even if a
# previous run was interrupted partway through.
ensure_iam_role() {
  if aws iam get-role --role-name "$ROLE_NAME" >/dev/null 2>&1; then
    log "IAM role '$ROLE_NAME' already exists; reusing it."
  else
    log "Creating IAM execution role '$ROLE_NAME'..."
    cat > "$WORKDIR/trust-policy.json" <<'EOF'
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": { "Service": "lambda.amazonaws.com" },
      "Action": "sts:AssumeRole"
    }
  ]
}
EOF
    aws iam create-role \
      --role-name "$ROLE_NAME" \
      --assume-role-policy-document "file://$WORKDIR/trust-policy.json" \
      --description "Execution role for the $FUNCTION_NAME Lambda (managed by scripts/deploy-aws.sh)" \
      >/dev/null
    NEW_ROLE=true
  fi

  ROLE_ARN="$(aws iam get-role --role-name "$ROLE_NAME" --query 'Role.Arn' --output text)"

  log "Attaching AWSLambdaBasicExecutionRole (CloudWatch Logs only) to '$ROLE_NAME'..."
  aws iam attach-role-policy \
    --role-name "$ROLE_NAME" \
    --policy-arn arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole >/dev/null

  log "Writing the least-privilege inline policy, scoped to $TABLE_ARN and its EmailIndex..."
  cat > "$WORKDIR/table-policy.json" <<EOF
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "AppTableAccess",
      "Effect": "Allow",
      "Action": [
        "dynamodb:GetItem",
        "dynamodb:PutItem",
        "dynamodb:UpdateItem",
        "dynamodb:Query",
        "dynamodb:TransactWriteItems"
      ],
      "Resource": [
        "$TABLE_ARN",
        "$TABLE_ARN/index/EmailIndex"
      ]
    }
  ]
}
EOF
  aws iam put-role-policy \
    --role-name "$ROLE_NAME" \
    --policy-name "${ROLE_NAME}-dynamodb" \
    --policy-document "file://$WORKDIR/table-policy.json" >/dev/null
}

# build_env_json writes the Lambda function's environment variables.
# DYNAMO_ENDPOINT, AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY, and AWS_REGION
# are all deliberately ABSENT: the first three would tell the SDK to talk
# to a fake local endpoint / would silently break SigV4 (config.go's
# NewClient doc comment); the last is a Lambda-reserved variable the
# platform already injects and the API would refuse to let us set anyway.
build_env_json() {
  cat > "$WORKDIR/env.json" <<EOF
{
  "Variables": {
    "APP_ENV": "aws",
    "LOG_LEVEL": "info",
    "DYNAMO_TABLE": "$TABLE_NAME",
    "JWT_SECRET": "$JWT_SECRET",
    "JWT_TTL": "1h",
    "BETSLIP_MIN_STAKE_AMOUNT": "1",
    "BETSLIP_MAX_STAKE_AMOUNT": "10000",
    "BETSLIP_CURRENCY_CODE": "PEN",
    "BETSLIP_MAX_SELECTIONS": "20",
    "RATE_LIMIT": "60-M",
    "IDEMPOTENCY_TTL": "24h",
    "DEMO_ACCOUNT_INITIAL_BALANCE": "1000"
  }
}
EOF
}

# create_function_with_retry retries only on the specific, well-known IAM
# propagation race (a role created moments ago is not yet assumable
# everywhere) — never a blind retry of arbitrary failures.
create_function_with_retry() {
  local attempt=1 max=8 delay=5 output
  while true; do
    if output=$(aws lambda create-function \
        --function-name "$FUNCTION_NAME" \
        --runtime provided.al2023 \
        --architectures arm64 \
        --handler bootstrap \
        --role "$ROLE_ARN" \
        --zip-file "fileb://$WORKDIR/function.zip" \
        --memory-size "$MEMORY_SIZE" \
        --timeout "$LAMBDA_TIMEOUT" \
        --environment "file://$WORKDIR/env.json" \
        --region "$REGION" 2>&1); then
      return 0
    fi
    if [[ "$output" == *"cannot be assumed"* ]] && [ "$attempt" -lt "$max" ]; then
      log "IAM role not yet assumable by Lambda (propagation delay) — retrying in ${delay}s (attempt $attempt/$max)..."
      sleep "$delay"
      attempt=$((attempt + 1))
      continue
    fi
    err "aws lambda create-function failed: $output"
  done
}

update_function_code_and_config() {
  aws lambda update-function-code \
    --function-name "$FUNCTION_NAME" \
    --zip-file "fileb://$WORKDIR/function.zip" \
    --region "$REGION" >/dev/null
  aws lambda wait function-updated --function-name "$FUNCTION_NAME" --region "$REGION"

  aws lambda update-function-configuration \
    --function-name "$FUNCTION_NAME" \
    --role "$ROLE_ARN" \
    --runtime provided.al2023 \
    --memory-size "$MEMORY_SIZE" \
    --timeout "$LAMBDA_TIMEOUT" \
    --environment "file://$WORKDIR/env.json" \
    --region "$REGION" >/dev/null
  aws lambda wait function-updated --function-name "$FUNCTION_NAME" --region "$REGION"
}

deploy_lambda_function() {
  build_env_json

  if aws lambda get-function --function-name "$FUNCTION_NAME" --region "$REGION" >/dev/null 2>&1; then
    log "Lambda function '$FUNCTION_NAME' already exists; updating its code and configuration..."
    update_function_code_and_config
  else
    log "Creating Lambda function '$FUNCTION_NAME'..."
    create_function_with_retry
    aws lambda wait function-active --function-name "$FUNCTION_NAME" --region "$REGION"
  fi
}

# ensure_log_retention creates the function's log group up front (Lambda
# would otherwise create it lazily on first invocation, with NO retention
# limit — i.e. unbounded, if never touched) and pins retention at
# LOG_RETENTION_DAYS so CloudWatch Logs storage cannot silently grow past
# the 5GB Always-Free allowance.
ensure_log_retention() {
  local log_group="/aws/lambda/$FUNCTION_NAME" output
  log "Ensuring log group '$log_group' exists with a ${LOG_RETENTION_DAYS}-day retention..."
  if ! output=$(aws logs create-log-group --log-group-name "$log_group" --region "$REGION" 2>&1); then
    [[ "$output" == *"ResourceAlreadyExistsException"* ]] || err "aws logs create-log-group failed: $output"
  fi
  aws logs put-retention-policy \
    --log-group-name "$log_group" \
    --retention-in-days "$LOG_RETENTION_DAYS" \
    --region "$REGION" >/dev/null
}

# ensure_function_url creates (or reuses) a public Function URL and grants
# it the matching public-invoke resource policy. auth-type NONE is a
# deliberate, documented tradeoff: every mutating route still requires a
# valid application-level JWT (internal/adapters/http/middleware), so this
# only exposes what the spec already requires to be public (health,
# read-only catalog/docs endpoints, login).
ensure_function_url() {
  local url output
  if url=$(aws lambda get-function-url-config --function-name "$FUNCTION_NAME" --region "$REGION" --query 'FunctionUrl' --output text 2>/dev/null) \
      && [ -n "$url" ] && [ "$url" != "None" ]; then
    log "Function URL already configured: $url"
  else
    log "Creating a public Function URL (auth-type NONE)..."
    url=$(aws lambda create-function-url-config \
      --function-name "$FUNCTION_NAME" \
      --auth-type NONE \
      --region "$REGION" \
      --query 'FunctionUrl' --output text)
  fi

  if ! output=$(aws lambda add-permission \
      --function-name "$FUNCTION_NAME" \
      --statement-id FunctionURLAllowPublicAccess \
      --action lambda:InvokeFunctionUrl \
      --principal '*' \
      --function-url-auth-type NONE \
      --region "$REGION" 2>&1); then
    [[ "$output" == *"ResourceConflictException"* ]] || err "aws lambda add-permission failed: $output"
  fi

  FUNCTION_URL="$url"
}

wait_for_health() {
  log "Waiting for $FUNCTION_URL health to come up (cold start)..."
  local attempt=1 max=20 code
  while [ "$attempt" -le "$max" ]; do
    code="$(curl -sS -o /dev/null -w '%{http_code}' --max-time 5 "${FUNCTION_URL%/}/health" 2>/dev/null || true)"
    if [ "$code" = "200" ]; then
      log "Function URL is responding (attempt $attempt)."
      return 0
    fi
    sleep 3
    attempt=$((attempt + 1))
  done
  err "Function URL never answered 200 on /health after $((max * 3))s (last status: '${code:-none}'): $FUNCTION_URL"
}

run_smoke() {
  log "Running scripts/smoke.sh against the live deployment..."
  "$REPO_ROOT/scripts/smoke.sh" "${FUNCTION_URL%/}"
}

print_summary() {
  cat <<EOF

========================================================================
 Deployment summary
========================================================================
 Function URL  : $FUNCTION_URL
 Function name : $FUNCTION_NAME
 Table name    : $TABLE_NAME
 Role name     : $ROLE_NAME
 Region        : $REGION
 Log group     : /aws/lambda/$FUNCTION_NAME (retention: ${LOG_RETENTION_DAYS}d)

 Tear everything down with:
   scripts/destroy-aws.sh --yes --function-name "$FUNCTION_NAME" --table-name "$TABLE_NAME" --role-name "$ROLE_NAME" --region "$REGION"

 Cost target: \$0/month on the AWS Always-Free tier (Lambda 1M req +
 400k GB-s, DynamoDB 25 RCU/WCU provisioned, CloudWatch Logs 5GB, 7-day
 retention). Hard cap: ~\$5/month even outside the free tier at this
 workload's scale. No NAT gateway, VPC, or ElastiCache was created.
========================================================================
EOF
}

main() {
  parse_args "$@"
  preflight

  WORKDIR="$(mktemp -d)"
  trap 'rm -rf "$WORKDIR"' EXIT

  resolve_jwt_secret
  build_lambda_zip
  run_seed
  get_table_arn
  ensure_iam_role
  deploy_lambda_function
  ensure_log_retention
  ensure_function_url
  wait_for_health
  run_smoke
  print_summary
}

main "$@"
