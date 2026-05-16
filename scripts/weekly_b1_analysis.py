#!/usr/bin/env python3
"""周线 B1 分析报告生成器"""

import concurrent.futures
import json
import urllib.request
import urllib.error
from datetime import datetime

API_BASE = "http://192.168.31.85:41027"
MAX_WORKERS = 20


def fetch_json(url, timeout=30):
    """获取 JSON 数据"""
    try:
        req = urllib.request.Request(url)
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            return json.loads(resp.read().decode("utf-8"))
    except Exception as e:
        return {"error": str(e)}


def get_signal_stocks():
    """获取周线 B1 信号股票"""
    url = f"{API_BASE}/api/stocks/signal?strategy=weekly_b1_buy"
    data = fetch_json(url, timeout=120)
    if data.get("code") == 0:
        return data["data"]["codes"]
    return []


def get_financial_report(code):
    """获取单只股票财报数据"""
    url = f"{API_BASE}/api/stocks/financial-report?code={code}&pagesize=20"
    data = fetch_json(url, timeout=30)
    if data.get("code") == 0:
        return code, data["data"]["data"]
    return code, []


def get_stock_name(code):
    """通过代码推断股票名称（简化版，基于已知映射或规则）"""
    # 这里只是简化处理，实际需要更完善的名字映射
    return ""


def compute_growth_rate(current, previous):
    """计算增长率"""
    if previous == 0 or previous is None:
        return 0
    return (current - previous) / abs(previous) * 100


def score_stock(code, reports):
    """基于财报数据进行基本面评分"""
    if not reports or len(reports) < 4:
        return 50, "财报数据不足，无法充分评估"

    # 按报告日期排序（最新在前）
    sorted_reports = sorted(reports, key=lambda x: x.get("report_date", ""), reverse=True)

    # 取最近 4 个季度
    recent = sorted_reports[:4]

    # 最新年报/季报
    latest = recent[0]

    # 计算同比增长（同季度对比）
    # 需要找到去年同期数据
    yoy_growth = 0
    revenue_growth = 0
    profit_growth = 0

    latest_date = latest.get("report_date", "")
    latest_year = int(latest_date[:4])
    latest_type = latest.get("report_type", 0)

    # 找去年同期
    prev_year_date = f"{latest_year - 1}{latest_date[4:]}"
    prev_year_report = None
    for r in sorted_reports:
        if r.get("report_date") == prev_year_date:
            prev_year_report = r
            break

    if prev_year_report:
        revenue_growth = compute_growth_rate(
            latest.get("total_revenue", 0),
            prev_year_report.get("total_revenue", 0)
        )
        profit_growth = compute_growth_rate(
            latest.get("net_profit", 0),
            prev_year_report.get("net_profit", 0)
        )

    # 计算季度环比（可选）
    qoq_profit_growth = 0
    if len(recent) >= 2:
        qoq_profit_growth = compute_growth_rate(
            recent[0].get("net_profit", 0),
            recent[1].get("net_profit", 0)
        )

    # 核心财务指标
    gross_margin = latest.get("gross_margin", 0) or 0
    net_margin = latest.get("net_margin", 0) or 0
    roe = latest.get("roe", 0) or 0
    roa = latest.get("roa", 0) or 0
    asset_liab_ratio = latest.get("asset_liability_ratio", 0) or 0
    operating_cash_flow = latest.get("operating_cash_flow", 0) or 0
    net_profit = latest.get("net_profit", 0) or 0

    # 检查现金流是否大于净利润（健康的标志）
    cash_profit_ratio = 0
    if net_profit > 0:
        cash_profit_ratio = operating_cash_flow / net_profit

    # 评分逻辑
    score = 50  # 基础分
    reasons = []

    # 1. 增长性评分 (0-40分)
    growth_score = 0
    if profit_growth > 50:
        growth_score = 40
        reasons.append(f"净利润同比大增{profit_growth:.1f}%")
    elif profit_growth > 30:
        growth_score = 35
        reasons.append(f"净利润同比增长{profit_growth:.1f}%")
    elif profit_growth > 15:
        growth_score = 28
        reasons.append(f"净利润同比增长{profit_growth:.1f}%")
    elif profit_growth > 0:
        growth_score = 20
        reasons.append(f"净利润微增{profit_growth:.1f}%")
    elif profit_growth > -20:
        growth_score = 10
        reasons.append(f"净利润下滑{abs(profit_growth):.1f}%")
    else:
        growth_score = 0
        reasons.append(f"净利润大幅下滑{abs(profit_growth):.1f}%")

    # 营收增长补充
    if revenue_growth > 20:
        growth_score += 5
        reasons.append(f"营收同比增长{revenue_growth:.1f}%")
    elif revenue_growth < -10:
        growth_score -= 5
        reasons.append(f"营收同比下滑{abs(revenue_growth):.1f}%")

    growth_score = min(40, max(0, growth_score))

    # 2. 盈利能力/护城河评分 (0-35分)
    moat_score = 0
    if gross_margin > 50:
        moat_score += 15
        reasons.append(f"毛利率高达{gross_margin:.1f}%")
    elif gross_margin > 35:
        moat_score += 12
        reasons.append(f"毛利率{gross_margin:.1f}%")
    elif gross_margin > 20:
        moat_score += 8
        reasons.append(f"毛利率{gross_margin:.1f}%")
    else:
        moat_score += 4
        reasons.append(f"毛利率较低{gross_margin:.1f}%")

    if roe > 15:
        moat_score += 12
        reasons.append(f"ROE优秀{roe:.1f}%")
    elif roe > 10:
        moat_score += 9
        reasons.append(f"ROE良好{roe:.1f}%")
    elif roe > 5:
        moat_score += 6
        reasons.append(f"ROE一般{roe:.1f}%")
    else:
        moat_score += 2
        reasons.append(f"ROE较低{roe:.1f}%")

    if net_margin > 20:
        moat_score += 8
        reasons.append(f"净利率优秀{net_margin:.1f}%")
    elif net_margin > 10:
        moat_score += 6
        reasons.append(f"净利率良好{net_margin:.1f}%")
    elif net_margin > 5:
        moat_score += 4
        reasons.append(f"净利率一般{net_margin:.1f}%")
    else:
        moat_score += 2
        reasons.append(f"净利率较低{net_margin:.1f}%")

    moat_score = min(35, max(0, moat_score))

    # 3. 财务健康度评分 (0-25分)
    health_score = 0
    if asset_liab_ratio < 40:
        health_score += 10
        reasons.append(f"资产负债率低{asset_liab_ratio:.1f}%")
    elif asset_liab_ratio < 60:
        health_score += 8
        reasons.append(f"资产负债率适中{asset_liab_ratio:.1f}%")
    elif asset_liab_ratio < 70:
        health_score += 5
        reasons.append(f"资产负债率偏高{asset_liab_ratio:.1f}%")
    else:
        health_score += 2
        reasons.append(f"资产负债率较高{asset_liab_ratio:.1f}%")

    if cash_profit_ratio > 1.2:
        health_score += 10
        reasons.append("经营现金流覆盖净利润")
    elif cash_profit_ratio > 0.8:
        health_score += 7
        reasons.append("经营现金流基本匹配净利润")
    elif cash_profit_ratio > 0:
        health_score += 4
        reasons.append("经营现金流为正但低于净利润")
    else:
        health_score += 0
        reasons.append("经营现金流为负")

    if roa > 5:
        health_score += 5
    elif roa > 2:
        health_score += 3
    else:
        health_score += 1

    health_score = min(25, max(0, health_score))

    # 总分
    total_score = growth_score + moat_score + health_score

    # 组装原因描述（取前 3-4 个关键信息）
    reason_str = "；".join(reasons[:4])

    return total_score, reason_str


def get_stock_names():
    """从代码文件读取股票名称映射"""
    names = {}
    import os
    code_dir = "/Users/lmm/project/lmm/trading/shell/code"
    for fname in os.listdir(code_dir):
        if fname.endswith(".txt"):
            with open(os.path.join(code_dir, fname), "r", encoding="utf-8") as f:
                for line in f:
                    line = line.strip()
                    if not line:
                        continue
                    parts = line.split()
                    if len(parts) >= 2:
                        code = parts[0].strip()
                        name = parts[1].strip()
                        names[code] = name
    return names


def main():
    print("正在获取周线 B1 信号股票...")
    codes = get_signal_stocks()
    print(f"共获取 {len(codes)} 只信号股票")

    if not codes:
        print("未获取到信号股票")
        return

    print("正在并发获取财报数据...")
    results = {}
    with concurrent.futures.ThreadPoolExecutor(max_workers=MAX_WORKERS) as executor:
        futures = {executor.submit(get_financial_report, code): code for code in codes}
        for future in concurrent.futures.as_completed(futures):
            code, reports = future.result()
            results[code] = reports

    print("正在评分...")
    name_map = get_stock_names()
    scored = []
    for code, reports in results.items():
        score, reason = score_stock(code, reports)
        name = name_map.get(code, "")
        scored.append({
            "code": code,
            "name": name,
            "score": score,
            "reason": reason,
            "has_data": bool(reports)
        })

    # 按分数降序
    scored.sort(key=lambda x: x["score"], reverse=True)

    # 生成报告
    today = datetime.now().strftime("%Y-%m-%d")
    report_path = f"/Users/lmm/project/lmm/trading/docs/analysis/b1/{today}-周线b1分析.md"

    # 统计
    total = len(scored)
    high_quality = [s for s in scored if s["score"] >= 80]
    no_data = [s for s in scored if not s["has_data"]]

    lines = [
        f"# {today} 周线 B1 分析报告",
        "",
        "## 分析概览",
        "",
        f"- 扫描日期：{today}",
        "- 分析模式：周线 B1",
        f"- 信号股票总数：{total} 只",
        f"- 评分 ≥80 的优质标的：{len(high_quality)} 只",
        "",
        "## 评分明细",
        "",
        "| 股票名称 | 股票代码 | 打分 | 原因 |",
        "|----------|----------|------|------|",
    ]

    for s in scored:
        name = s["name"] or "-"
        code = s["code"]
        score = s["score"]
        reason = s["reason"]
        if not s["has_data"]:
            reason = "财报数据缺失"
        lines.append(f"| {name} | {code} | {score} | {reason} |")

    lines.extend([
        "",
        "## 重点关注（评分 ≥80）",
        "",
    ])

    if high_quality:
        for s in high_quality:
            name = s["name"] or s["code"]
            lines.append(f"- **{name}（{s['code']}）**：{s['reason']}")
    else:
        lines.append("本周暂无评分 ≥80 的优质标的。")

    lines.extend([
        "",
        "## 风险提示",
        "",
        "- 周线 B1 侧重长期价值投资，需承受短期波动",
        "- 所有评分基于历史数据，不构成投资建议",
        "- 宏观经济和行业周期变化可能影响基本面判断",
        "- 评分依赖财报数据完整性，部分股票可能因数据缺失导致评分偏差",
    ])

    with open(report_path, "w", encoding="utf-8") as f:
        f.write("\n".join(lines))

    print(f"报告已生成：{report_path}")
    print(f"信号股票总数：{total}")
    print(f"评分 ≥80 的优质标的：{len(high_quality)} 只")
    if high_quality:
        print("\n优质标的：")
        for s in high_quality[:10]:
            name = s["name"] or s["code"]
            print(f"  {name}（{s['code']}）：{s['score']}分 - {s['reason']}")


if __name__ == "__main__":
    main()
