#!/usr/bin/env bash
set -euo pipefail

verify_local=false
verify_full=false
verify_mysql=false
image_target=""
if [[ $# -eq 0 ]]; then verify_local=true; fi
for option in "$@"; do
  case "$option" in
    --mysql) verify_mysql=true ;;
    --image) image_target=all ;;
    --image=updater|--image=workbench|--image=updater-configured) image_target="${option#--image=}" ;;
    --full) verify_full=true ;;
    --help|-h)
      echo "用法: bash scripts/verify.sh [--mysql] [--image[=updater|workbench|updater-configured]] [--full]"
      echo "无参数：快速本地检查。--mysql/--image：仅所选检查，可组合。--full：覆盖率、Race、MySQL、全部镜像。"
      exit 0 ;;
    *) echo "未知参数: $option（使用 --help 查看用法）" >&2; exit 2 ;;
  esac
done
if "$verify_full"; then
  verify_local=true
  verify_mysql=true
  image_target=all
fi

repo_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
verify_dir="$(mktemp -d "${TMPDIR:-/tmp}/trading-verify.XXXXXX")"
trap 'rm -rf -- "$verify_dir"' EXIT
cd "$repo_dir"

require_coverage() {
  local label="$1" profile="$2" minimum="$3" actual
  actual="$(go tool cover -func="$profile" | awk '/^total:/ {gsub(/%/, "", $3); print $3}')"
  if ! awk -v actual="$actual" -v minimum="$minimum" 'BEGIN { exit !(actual >= minimum) }'; then
    echo "$label 覆盖率 ${actual}% 低于门槛 ${minimum}%" >&2
    exit 1
  fi
  echo "$label 覆盖率：${actual}%"
}

if "$verify_local"; then
  echo "前端测试与生产构建"
  if [[ ! -d web/node_modules ]]; then
    npm --prefix web ci --prefer-offline --no-audit --no-fund
  fi
  if "$verify_full"; then
    npm --prefix web run check
  else
    npm --prefix web run test -- --run
    npm --prefix web run build
  fi

  if "$verify_full"; then
    echo "Go 测试与覆盖率（含文档与性能回归）"
    tested_packages=()
    while IFS= read -r package; do
      if [[ -n "$package" ]]; then tested_packages+=("$package"); fi
    done < <(go list -f '{{if or .TestGoFiles .XTestGoFiles}}{{.ImportPath}}{{end}}' ./...)
    go test "${tested_packages[@]}" -coverprofile="$verify_dir/all.cover" -count=1
    require_coverage "总计" "$verify_dir/all.cover" 80
    for area in market indicator strategy backtest; do
      awk -v prefix="trading/internal/$area/" 'NR == 1 || index($1, prefix) == 1' \
        "$verify_dir/all.cover" > "$verify_dir/$area.cover"
      require_coverage "$area" "$verify_dir/$area.cover" 90
    done
    echo "Race Detector"
    go test -race ./... -count=1
  else
    echo "Go 测试（含文档与性能回归）"
    go test ./...
  fi
  go vet ./...
fi

if "$verify_mysql"; then
  echo "远端 MySQL 8.4/x86_64 隔离验收"
  go test -tags=deployment ./internal/infrastructure/mysql -run '^TestDeploymentMySQLCompatibility$' -count=1
  go test -tags=integration ./internal/infrastructure/mysql/... ./data -count=1
fi

if [[ -n "$image_target" ]]; then
  echo "linux/amd64 镜像验收：$image_target"
  grep -q '^config\*\.yaml$' .dockerignore
  grep -q '^!config\.example\.yaml$' .dockerignore
  grep -q '^\*.tar$' .dockerignore
  image_targets=("$image_target")
  if [[ "$image_target" == all ]]; then image_targets=(updater workbench updater-configured); fi
  build_proxy_args=()
  if [[ -n "${TRADING_DOCKER_BUILD_PROXY:-}" ]]; then
    build_proxy_args+=(--build-arg "HTTP_PROXY=$TRADING_DOCKER_BUILD_PROXY" --build-arg "HTTPS_PROXY=$TRADING_DOCKER_BUILD_PROXY")
  fi
  for target in "${image_targets[@]}"; do
    config_args=()
    role="$target"
    if [[ "$target" == updater-configured ]]; then
      role=updater
      config_args+=(--no-cache-filter updater-configured --secret id=updater_config,src=config.updater.example.yaml)
    fi
    # Empty-array expansion remains compatible with macOS bash 3.2 and set -u.
    docker buildx build --platform linux/amd64 --target "$target" --load \
      ${config_args[@]+"${config_args[@]}"} ${build_proxy_args[@]+"${build_proxy_args[@]}"} \
      -t "trading-$target:verify" .
    metadata="$(docker image inspect "trading-$target:verify" --format '{{.Os}}/{{.Architecture}} {{.Config.User}} {{json .Config.Entrypoint}} {{json .Config.Cmd}}')"
    expected="linux/amd64 app [\"/app/trading\",\"-service\",\"$role\"] [\"-config\",\"/app/config.yaml\"]"
    [[ "$metadata" == "$expected" ]]
    if [[ "$target" == updater-configured ]]; then
      docker run --rm --network none --platform linux/amd64 --entrypoint /bin/sh "trading-$target:verify" -c \
        'test "$(id -un)" = app && test -r /app/config.yaml && test "$(stat -c %a /app/config.yaml)" = 400'
    else
      docker run --rm --network none --platform linux/amd64 --entrypoint /bin/sh "trading-$target:verify" -c \
        'test ! -e /app/config.yaml && if [ "$1" = workbench ]; then test -s /app/web/dist/index.html; else test ! -d /app/web; fi' sh "$target"
    fi
  done
fi

echo "所选检查通过；未选项目不计为通过"
