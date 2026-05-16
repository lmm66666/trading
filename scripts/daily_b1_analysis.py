#!/usr/bin/env python3
"""日线 B1 分析报告生成器"""

import concurrent.futures
import json
import math
import urllib.request
import os
from datetime import datetime

API_BASE = "http://192.168.31.85:41027"
MAX_WORKERS = 20


def fetch_json(url, timeout=30):
    try:
        req = urllib.request.Request(url)
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            return json.loads(resp.read().decode("utf-8"))
    except Exception as e:
        return {"error": str(e)}


def get_signal_stocks():
    url = f"{API_BASE}/api/stocks/signal?strategy=daily_b1_buy"
    data = fetch_json(url, timeout=120)
    if data.get("code") == 0:
        return data["data"]["codes"]
    return []


def get_price_data(code):
    url = f"{API_BASE}/api/stocks/price?code={code}&cycle=daily&pagesize=60"
    data = fetch_json(url, timeout=30)
    if data.get("code") == 0:
        return code, data["data"]["data"]
    return code, []


def get_financial_report(code):
    url = f"{API_BASE}/api/stocks/financial-report?code={code}&pagesize=20"
    data = fetch_json(url, timeout=30)
    if data.get("code") == 0:
        return code, data["data"]["data"]
    return code, []


def get_stock_names():
    names = {}
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


def compute_ma(prices, period):
    if len(prices) < period:
        return []
    ma = []
    for i in range(period - 1, len(prices)):
        avg = sum(prices[i - period + 1:i + 1]) / period
        ma.append(avg)
    return ma


def compute_ema(prices, period):
    if len(prices) < period:
        return []
    multiplier = 2 / (period + 1)
    ema = [sum(prices[:period]) / period]
    for i in range(period, len(prices)):
        ema.append((prices[i] - ema[-1]) * multiplier + ema[-1])
    return ema


def compute_kdj(klines):
    """计算 KDJ 指标，返回 (k, d, j) 最新值"""
    if len(klines) < 9:
        return None, None, None
    closes = [k["close"] for k in klines]
    highs = [k["high"] for k in klines]
    lows = [k["low"] for k in klines]

    n = 9
    rsvs = []
    for i in range(n - 1, len(closes)):
        hn = max(highs[i - n + 1:i + 1])
        ln = min(lows[i - n + 1:i + 1])
        if hn == ln:
            rsv = 50
        else:
            rsv = (closes[i] - ln) / (hn - ln) * 100
        rsvs.append(rsv)

    k = 50
    d = 50
    for rsv in rsvs:
        k = 2 / 3 * k + 1 / 3 * rsv
        d = 2 / 3 * d + 1 / 3 * k

    j = 3 * k - 2 * d
    return k, d, j


def compute_macd(prices):
    """计算 MACD，返回 (dif, dea, macd) 最新值"""
    if len(prices) < 26:
        return None, None, None
    ema12 = compute_ema(prices, 12)
    ema26 = compute_ema(prices, 26)
    if len(ema12) < len(ema26):
        return None, None, None
    dif = [ema12[i] - ema26[i] for i in range(len(ema26))]
    dea = compute_ema(dif, 9)
    if not dea:
        return None, None, None
    macd = [(dif[i] - dea[i]) * 2 for i in range(len(dea))]
    return dif[-1], dea[-1], macd[-1]


def score_technical(klines):
    """短线技术面评分（0-100）"""
    if len(klines) < 20:
        return 50, "K线数据不足"

    closes = [k["close"] for k in klines]
    volumes = [k["volume"] for k in klines]
    latest_close = closes[0]

    # 1. KDJ 位置 (25分)
    k, d, j = compute_kdj(klines)
    kdj_score = 0
    kdj_reasons = []
    if k is not None:
        if k < 20 and d < 20:
            kdj_score = 25
            kdj_reasons.append(f"KDJ超卖区(K={k:.1f},D={d:.1f})")
        elif k < 30:
            kdj_score = 20
            kdj_reasons.append(f"KDJ低位(K={k:.1f},D={d:.1f})")
        elif k < 50:
            kdj_score = 15
            kdj_reasons.append(f"KDJ中性偏低(K={k:.1f})")
        elif k < 80:
            kdj_score = 10
            kdj_reasons.append(f"KDJ中性(K={k:.1f})")
        else:
            kdj_score = 5
            kdj_reasons.append(f"KDJ高位(K={k:.1f})")

        # 金叉加分
        if len(klines) >= 10:
            prev_k, prev_d, _ = compute_kdj(klines[1:])
            if prev_k is not None and k > d and prev_k <= prev_d:
                kdj_score = min(25, kdj_score + 5)
                kdj_reasons.append("KDJ金叉")
    else:
        kdj_reasons.append("KDJ计算失败")

    # 2. MA 趋势 (25分)
    ma5 = compute_ma(closes, 5)
    ma10 = compute_ma(closes, 10)
    ma20 = compute_ma(closes, 20)

    ma_score = 0
    ma_reasons = []
    if ma20:
        ma20_latest = ma20[0] if len(ma20) > 0 else None
        ma20_prev = ma20[1] if len(ma20) > 1 else None

        if ma20_latest:
            # 价格站稳 MA20
            if latest_close > ma20_latest * 1.05:
                ma_score = 20
                ma_reasons.append(f"价格高于MA20 {((latest_close/ma20_latest-1)*100):.1f}%")
            elif latest_close > ma20_latest:
                ma_score = 15
                ma_reasons.append("价格站稳MA20")
            elif latest_close > ma20_latest * 0.97:
                ma_score = 10
                ma_reasons.append("价格回踩MA20")
            else:
                ma_score = 5
                ma_reasons.append("价格跌破MA20")

            # MA20 向上加分
            if ma20_prev and ma20_latest > ma20_prev:
                ma_score = min(25, ma_score + 5)
                ma_reasons.append("MA20向上")

            # 多头排列加分
            if ma5 and ma10 and ma5[0] > ma10[0] > ma20_latest:
                ma_score = min(25, ma_score + 3)
                ma_reasons.append("均线多头排列")
    else:
        ma_reasons.append("MA数据不足")

    # 3. 成交量配合 (25分)
    vol_score = 0
    vol_reasons = []
    if len(volumes) >= 10:
        recent_vol = sum(volumes[:5]) / 5
        prev_vol = sum(volumes[5:10]) / 5
        latest_vol = volumes[0]

        if prev_vol > 0:
            vol_ratio = recent_vol / prev_vol
            if vol_ratio > 1.5:
                vol_score = 20
                vol_reasons.append(f"近期放量{vol_ratio:.1f}倍")
            elif vol_ratio > 1.2:
                vol_score = 15
                vol_reasons.append(f"近期温和放量{vol_ratio:.1f}倍")
            elif vol_ratio > 0.8:
                vol_score = 10
                vol_reasons.append("成交量平稳")
            else:
                vol_score = 5
                vol_reasons.append("近期缩量")

            # 最新日缩量回调是正面信号（放量上涨后缩量回调）
            if latest_vol < recent_vol * 0.7 and latest_close < closes[1]:
                vol_score = min(25, vol_score + 5)
                vol_reasons.append("缩量回调")
            elif latest_vol > recent_vol * 1.5 and latest_close < closes[1]:
                vol_score = max(0, vol_score - 5)
                vol_reasons.append("放量下跌")
    else:
        vol_reasons.append("成交量数据不足")

    # 4. 近期夏普比率 (25分)
    sharpe_score = 0
    sharpe_reasons = []
    if len(closes) >= 20:
        recent_returns = []
        for i in range(min(20, len(closes) - 1)):
            ret = (closes[i] - closes[i + 1]) / closes[i + 1] if closes[i + 1] != 0 else 0
            recent_returns.append(ret)

        if recent_returns:
            avg_ret = sum(recent_returns) / len(recent_returns)
            std_ret = math.sqrt(sum((r - avg_ret) ** 2 for r in recent_returns) / len(recent_returns)) if len(recent_returns) > 1 else 0.001

            # 年化夏普（简化：按日计算）
            if std_ret > 0:
                sharpe = (avg_ret * 252) / (std_ret * math.sqrt(252))
                if sharpe > 2:
                    sharpe_score = 25
                    sharpe_reasons.append(f"夏普比率优秀{sharpe:.2f}")
                elif sharpe > 1:
                    sharpe_score = 20
                    sharpe_reasons.append(f"夏普比率良好{sharpe:.2f}")
                elif sharpe > 0:
                    sharpe_score = 12
                    sharpe_reasons.append(f"夏普比率一般{sharpe:.2f}")
                else:
                    sharpe_score = 5
                    sharpe_reasons.append(f"夏普比率为负{sharpe:.2f}")
            else:
                sharpe_reasons.append("波动率极低")
                sharpe_score = 15
    else:
        sharpe_reasons.append("数据不足计算夏普")

    total = kdj_score + ma_score + vol_score + sharpe_score
    reasons = kdj_reasons + ma_reasons + vol_reasons + sharpe_reasons
    return total, "；".join(reasons[:5])


def compute_growth_rate(current, previous):
    if previous == 0 or previous is None:
        return 0
    return (current - previous) / abs(previous) * 100


def score_fundamental(reports):
    """长线基本面评分（0-100）"""
    if not reports or len(reports) < 4:
        return 50, "财报数据不足"

    sorted_reports = sorted(reports, key=lambda x: x.get("report_date", ""), reverse=True)
    recent = sorted_reports[:4]
    latest = recent[0]

    latest_date = latest.get("report_date", "")
    latest_year = int(latest_date[:4])

    prev_year_date = f"{latest_year - 1}{latest_date[4:]}"
    prev_year_report = None
    for r in sorted_reports:
        if r.get("report_date") == prev_year_date:
            prev_year_report = r
            break

    revenue_growth = 0
    profit_growth = 0
    if prev_year_report:
        revenue_growth = compute_growth_rate(
            latest.get("total_revenue", 0),
            prev_year_report.get("total_revenue", 0)
        )
        profit_growth = compute_growth_rate(
            latest.get("net_profit", 0),
            prev_year_report.get("net_profit", 0)
        )

    gross_margin = latest.get("gross_margin", 0) or 0
    net_margin = latest.get("net_margin", 0) or 0
    roe = latest.get("roe", 0) or 0
    roa = latest.get("roa", 0) or 0
    asset_liab_ratio = latest.get("asset_liability_ratio", 0) or 0
    operating_cash_flow = latest.get("operating_cash_flow", 0) or 0
    net_profit = latest.get("net_profit", 0) or 0

    cash_profit_ratio = 0
    if net_profit > 0:
        cash_profit_ratio = operating_cash_flow / net_profit

    score = 50
    reasons = []

    # 增长性 (0-40)
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

    if revenue_growth > 20:
        growth_score += 5
        reasons.append(f"营收同比增长{revenue_growth:.1f}%")
    elif revenue_growth < -10:
        growth_score -= 5
        reasons.append(f"营收同比下滑{abs(revenue_growth):.1f}%")

    growth_score = min(40, max(0, growth_score))

    # 护城河 (0-35)
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

    # 财务健康 (0-25)
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

    total = growth_score + moat_score + health_score
    return total, "；".join(reasons[:4])


def main():
    print("正在获取日线 B1 信号股票...")
    codes = get_signal_stocks()
    print(f"共获取 {len(codes)} 只信号股票")

    if not codes:
        print("未获取到信号股票")
        return

    print("正在并发获取 K 线数据与财报数据...")
    price_results = {}
    financial_results = {}

    with concurrent.futures.ThreadPoolExecutor(max_workers=MAX_WORKERS) as executor:
        # 获取 K 线
        price_futures = {executor.submit(get_price_data, code): code for code in codes}
        for future in concurrent.futures.as_completed(price_futures):
            code, klines = future.result()
            price_results[code] = klines

    with concurrent.futures.ThreadPoolExecutor(max_workers=MAX_WORKERS) as executor:
        # 获取财报
        fin_futures = {executor.submit(get_financial_report, code): code for code in codes}
        for future in concurrent.futures.as_completed(fin_futures):
            code, reports = future.result()
            financial_results[code] = reports

    print("正在评分...")
    name_map = get_stock_names()
    scored = []

    for code in codes:
        klines = price_results.get(code, [])
        reports = financial_results.get(code, [])

        short_score, short_reason = score_technical(klines)
        long_score, long_reason = score_fundamental(reports)
        name = name_map.get(code, "")

        scored.append({
            "code": code,
            "name": name,
            "short_score": short_score,
            "short_reason": short_reason,
            "long_score": long_score,
            "long_reason": long_reason,
        })

    # 按短线分数降序
    scored.sort(key=lambda x: x["short_score"], reverse=True)

    today = datetime.now().strftime("%Y-%m-%d")
    report_path = f"/Users/lmm/project/lmm/trading/docs/analysis/b1/{today}-日线b1分析.md"

    total = len(scored)
    high_quality = [s for s in scored if s["short_score"] >= 80 and s["long_score"] >= 70]
    short_high = [s for s in scored if s["short_score"] >= 80]
    long_high = [s for s in scored if s["long_score"] >= 80]

    lines = [
        f"# {today} 日线 B1 分析报告",
        "",
        "## 分析概览",
        "",
        f"- 扫描日期：{today}",
        "- 分析模式：日线 B1",
        f"- 信号股票总数：{total} 只",
        f"- 短线评分 ≥80 且长线评分 ≥70 的优质标的：{len(high_quality)} 只",
        f"- 短线评分 ≥80：{len(short_high)} 只",
        f"- 长线评分 ≥80：{len(long_high)} 只",
        "",
        "## 评分明细",
        "",
        "| 股票名称 | 股票代码 | 短线打分 | 短线原因 | 长线打分 | 长线原因 |",
        "|----------|----------|----------|----------|----------|----------|",
    ]

    for s in scored:
        name = s["name"] or "-"
        lines.append(f"| {name} | {s['code']} | {s['short_score']} | {s['short_reason']} | {s['long_score']} | {s['long_reason']} |")

    lines.extend([
        "",
        "## 重点关注（短线 ≥80 且长线 ≥70）",
        "",
    ])

    if high_quality:
        for s in high_quality:
            name = s["name"] or s["code"]
            lines.append(f"- **{name}（{s['code']}）**：短线 {s['short_score']} 分（{s['short_reason']}）；长线 {s['long_score']} 分（{s['long_reason']}）")
    else:
        lines.append("本周暂无双维度均达标的优质标的。")

    lines.extend([
        "",
        "## 短线强势标的（短线 ≥80）",
        "",
    ])

    for s in short_high[:15]:
        name = s["name"] or s["code"]
        lines.append(f"- **{name}（{s['code']}）**：短线 {s['short_score']} 分 — {s['short_reason']}；长线 {s['long_score']} 分 — {s['long_reason']}")

    lines.extend([
        "",
        "## 长线优质标的（长线 ≥80）",
        "",
    ])

    for s in long_high[:15]:
        name = s["name"] or s["code"]
        lines.append(f"- **{name}（{s['code']}）**：长线 {s['long_score']} 分 — {s['long_reason']}；短线 {s['short_score']} 分 — {s['short_reason']}")

    lines.extend([
        "",
        "## 风险提示",
        "",
        "- 短线评分高但长线评分低的股票适合波段操作，不宜长期持有",
        "- 所有评分基于历史数据，不构成投资建议",
        "- 市场系统性风险可能影响所有标的",
        "- 技术面评分依赖 K 线数据质量，部分股票可能因数据缺失导致评分偏差",
    ])

    with open(report_path, "w", encoding="utf-8") as f:
        f.write("\n".join(lines))

    print(f"报告已生成：{report_path}")
    print(f"信号股票总数：{total}")
    print(f"双维度优质标的：{len(high_quality)} 只")
    print(f"短线 ≥80：{len(short_high)} 只")
    print(f"长线 ≥80：{len(long_high)} 只")

    if high_quality:
        print("\n双维度优质标的：")
        for s in high_quality[:10]:
            name = s["name"] or s["code"]
            print(f"  {name}（{s['code']}）：短线{s['short_score']} 长线{s['long_score']}")

    if short_high:
        print("\n短线强势标的 TOP10：")
        for s in short_high[:10]:
            name = s["name"] or s["code"]
            print(f"  {name}（{s['code']}）：短线{s['short_score']} — {s['short_reason'][:50]}")


if __name__ == "__main__":
    main()
