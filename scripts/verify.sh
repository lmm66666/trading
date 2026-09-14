#!/usr/bin/env bash
set -euo pipefail

repo_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
verify_dir="$(mktemp -d "${TMPDIR:-/tmp}/trading-verify.XXXXXX")"
trap 'rm -rf -- "$verify_dir"' EXIT
cd "$repo_dir"

coverage_percent() {
  local profile="$1"
  go tool cover -func="$profile" | awk '/^total:/ {gsub(/%/, "", $3); print $3}'
}

require_coverage() {
  local label="$1"
  local profile="$2"
  local minimum="$3"
  local actual
  actual="$(coverage_percent "$profile")"
  if ! awk -v actual="$actual" -v minimum="$minimum" 'BEGIN { exit !(actual >= minimum) }'; then
    echo "$label 覆盖率 ${actual}% 低于门槛 ${minimum}%" >&2
    exit 1
  fi
  echo "$label 覆盖率：${actual}%"
}

echo "[1/10] 前端测试、覆盖率与生产构建"
npm --prefix web ci
npm --prefix web run check

echo "[2/10] 文档契约"
go test . -run '^TestDocumentation' -count=1

echo "[3/10] 全量测试与总覆盖率"
go test ./... -count=1
tested_packages=()
while IFS= read -r package; do
  if [[ -n "$package" ]]; then
    tested_packages+=("$package")
  fi
done < <(go list -f '{{if or .TestGoFiles .XTestGoFiles}}{{.ImportPath}}{{end}}' ./...)
go test "${tested_packages[@]}" -coverprofile="$verify_dir/all.cover" -count=1
require_coverage "总计" "$verify_dir/all.cover" 80

echo "[4/10] 核心领域覆盖率"
for area in market indicator strategy backtest; do
  package="./internal/$area"
  if [[ "$area" == "strategy" ]]; then
    package="./internal/strategy/..."
  fi
  go test "$package" -coverprofile="$verify_dir/$area.cover" -count=1
  require_coverage "$area" "$verify_dir/$area.cover" 90
done

echo "[5/10] Race Detector"
go test -race ./... -count=1

echo "[6/10] 静态检查"
go vet ./...

echo "[7/10] 全市场性能门禁"
go test ./internal/application -run '^TestFullMarketScanPerformance$' -count=1 -v

echo "[8/10] 容器配置安全检查"
! grep -q 'COPY config-nas.yaml' Dockerfile
grep -q '^config\*\.yaml$' .dockerignore
grep -q '^!config\.example\.yaml$' .dockerignore
grep -q '^\*.tar$' .dockerignore

echo "[9/10] 远端 MySQL 8.4/x86_64 兼容性与隔离集成测试"
go test -tags=deployment ./internal/infrastructure/mysql -run '^TestDeploymentMySQLCompatibility$' -count=1
go test -tags=integration ./internal/infrastructure/mysql/... -count=1

echo "[10/10] linux/amd64 无本地配置镜像构建"
docker buildx build --platform linux/amd64 --load --no-cache -t trading:verify .

echo "验证全部通过"
