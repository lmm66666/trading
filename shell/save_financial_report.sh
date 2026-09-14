#!/bin/bash
#
# 每 7s 调用 SaveFinancialReportData 保存一只股票的财报数据
# 用法: ./save_financial_report.sh <base_url>
# 示例: ./save_financial_report.sh http://localhost:8080
#

set -euo pipefail

BASE_URL="${1:-${TRADING_API_BASE_URL:-}}"
if [[ -z "$BASE_URL" ]]; then
  echo "base_url 参数或 TRADING_API_BASE_URL 环境变量不能为空" >&2
  exit 2
fi
API_URL="${BASE_URL}/api/stocks/financial-report"
INTERVAL=2

codes=(
          301073 301075 301076 301077 301078 301079 301080 301081
          301082 301083 301085 301086 301087 301088 301089 301111
)

total=${#codes[@]}

echo "Found ${total} stocks, interval=${INTERVAL}s, target=${API_URL}"
echo "---"

success=0
fail=0

for i in "${!codes[@]}"; do
  code="${codes[$i]}"
  idx=$((i + 1))

  resp=$(curl -s -w "\n%{http_code}" -X POST "$API_URL" \
    -H "Content-Type: application/json" \
    -d "{\"code\": \"${code}\"}" \
    --max-time 30)

  http_code=$(echo "$resp" | tail -1 | tr -d '
')
  body=$(echo "$resp" | sed '$d')

  if ! [[ "$http_code" =~ ^[0-9]+$ ]]; then
    http_code="000"
  fi

  if [ "$http_code" -eq 200 ]; then
    echo "[${idx}/${total}] ${code} OK"
    success=$((success + 1))
  else
    echo "[${idx}/${total}] ${code} FAIL (HTTP ${http_code}) ${body}"
    fail=$((fail + 1))
  fi

  if [ "$idx" -lt "$total" ]; then
    sleep "$INTERVAL"
  fi
done

echo "---"
echo "Done: ${success} success, ${fail} failed, ${total} total"
