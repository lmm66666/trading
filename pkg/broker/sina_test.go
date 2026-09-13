package broker

import (
	"context"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"trading/model"
)

// TestSinaBrokerGetStockTodayInBatch 测试批量获取今日数据
func TestSinaBrokerGetStockTodayInBatch(t *testing.T) {
	requireLiveBroker(t)
	broker := NewSinaBroker()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	result, err := broker.GetStockTodayInBatch(ctx, []string{"sh000001"})
	if err != nil {
		t.Fatalf("fetch realtime batch failed: %v", err)
	}

	if len(result) == 0 {
		t.Fatal("expected at least one result, got none")
	}

	for code, kline := range result {
		t.Logf("%s: Open=%.2f High=%.2f Low=%.2f Close=%.2f Volume=%d",
			code, kline.Open, kline.High, kline.Low, kline.Close, kline.Volume)
	}
}

// TestSinaBrokerGetStockToday 测试单个获取今日数据
func TestSinaBrokerGetStockToday(t *testing.T) {
	requireLiveBroker(t)
	broker := NewSinaBroker()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	kline, err := broker.GetStockToday(ctx, "sh000001")
	if err != nil {
		t.Fatalf("fetch realtime single failed: %v", err)
	}

	t.Logf("sh000001: Open=%.2f High=%.2f Low=%.2f Close=%.2f Volume=%d",
		kline.Open, kline.High, kline.Low, kline.Close, kline.Volume)
}

// TestSinaBrokerGetStockHistorical 测试获取历史K线
func TestSinaBrokerGetStockHistorical(t *testing.T) {
	requireLiveBroker(t)
	broker := NewSinaBroker()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	data, err := broker.GetStockHistorical(ctx, "sh000001", 1680, 30)
	if err != nil {
		t.Fatalf("fetch historical failed: %v", err)
	}

	if len(data) == 0 {
		t.Fatal("expected historical data, got none")
	}

	for _, kline := range data {
		t.Logf("First: Date=%s Open=%.2f High=%.2f Low=%.2f Close=%.2f Volume=%d",
			kline.Date, kline.Open, kline.High, kline.Low, kline.Close, kline.Volume)
	}
	t.Logf("Total data points: %d", len(data))
}

// TestSinaBrokerImplementsInterface 验证 SinaBroker 实现了 Broker 接口
func TestSinaBrokerImplementsInterface(t *testing.T) {
	var _ Broker = (*SinaBroker)(nil)
}

// TestSinaBrokerRealtimeToKline 测试实时数据解析为 Kline
func TestSinaBrokerRealtimeToKline(t *testing.T) {
	tests := []struct {
		name    string
		code    string
		content string
		want    *model.StockKline
	}{
		{
			name:    "stock",
			code:    "sh600000",
			content: "浦发银行,10.50,10.40,10.60,10.70,10.30,10.55,10.60,123456,130456789,f10,f11,f12,f13,f14,f15,f16,f17,f18,f19,f20,f21,f22,f23,f24,f25,f26,f27,f28,f29,2025-04-22,15:00:00,00",
			want: &model.StockKline{
				Code:   "sh600000",
				Date:   "2025-04-22",
				Open:   10.50,
				High:   10.70,
				Low:    10.30,
				Close:  10.60,
				Volume: 123456,
			},
		},
		{
			name:    "exchange",
			code:    "USDCNY",
			content: "16:30:00,7.2000,7.2100,7.2050,1000,7.2150,7.2200,7.2250,7.2100,美元人民币,2025-04-22",
			want: &model.StockKline{
				Code:   "USDCNY",
				Date:   "2025-04-22",
				Open:   7.20,
				High:   7.205,
				Low:    7.20,
				Close:  7.21,
				Volume: 1000,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseRealtimeToKline(tt.code, tt.content)
			if got == nil {
				t.Fatal("expected non-nil kline")
			}
			if got.Code != tt.want.Code {
				t.Errorf("Code = %s, want %s", got.Code, tt.want.Code)
			}
			if got.Date != tt.want.Date {
				t.Errorf("Date = %s, want %s", got.Date, tt.want.Date)
			}
			if got.Open != tt.want.Open {
				t.Errorf("Open = %.2f, want %.2f", got.Open, tt.want.Open)
			}
			if got.High != tt.want.High {
				t.Errorf("High = %.2f, want %.2f", got.High, tt.want.High)
			}
			if got.Low != tt.want.Low {
				t.Errorf("Low = %.2f, want %.2f", got.Low, tt.want.Low)
			}
			if got.Close != tt.want.Close {
				t.Errorf("Close = %.2f, want %.2f", got.Close, tt.want.Close)
			}
			if got.Volume != tt.want.Volume {
				t.Errorf("Volume = %d, want %d", got.Volume, tt.want.Volume)
			}
		})
	}
}

// TestExtractJSONP 测试 JSONP 提取
func TestExtractJSONP(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "with script prefix",
			input: "/*<script>location.href='//sina.com';</script>*/callback({\"code\":1})",
			want:  "{\"code\":1}",
		},
		{
			name:  "plain jsonp",
			input: "cb({\"code\":2})",
			want:  "{\"code\":2}",
		},
		{
			name:    "no parens",
			input:   "invalid",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractJSONP(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("extractJSONP() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("extractJSONP() = %s, want %s", got, tt.want)
			}
		})
	}
}

// TestParseFinancialReportResponse 测试财报解析（核心指标映射）
func TestParseFinancialReportResponse(t *testing.T) {
	jsonp := `/*<script>location.href='//sina.com';</script>*/cb({"result":{"status":{"code":0},"data":{"report_count":"2","report_date":[{"date_value":"20250630","date_description":"2025半年报","date_type":2},{"date_value":"20250331","date_description":"2025一季报","date_type":1}],"report_list":{"20250630":{"rType":"合并期末","rCurrency":"CNY","data_source":"其他","is_audit":"已审计","publish_date":"20250830","is_exist_yoy":true,"data":[{"item_field":"PARENETP","item_title":"归母净利润","item_value":"1000000.50","item_precision":"f2","item_source":"lrb","item_tongbi":0.15},{"item_field":"BIZTOTINCO","item_title":"营业总收入","item_value":"5000000.00","item_precision":"f2","item_source":"lrb","item_tongbi":"0.08"},{"item_field":"BIZTOTCOST","item_title":"营业成本","item_value":"3000000.00","item_precision":"f2","item_source":"lrb","item_tongbi":"0.05"},{"item_field":"NPCUT","item_title":"扣非净利润","item_value":"900000.00","item_precision":"f2","item_source":"lrb","item_tongbi":0.12},{"item_field":"SGPMARGIN","item_title":"毛利率","item_value":"40.00","item_precision":"f2","item_source":"lrb"},{"item_field":"SNPMARGINCONMS","item_title":"销售净利率","item_value":"20.00","item_precision":"f2","item_source":"lrb"},{"item_field":"ROEWEIGHTED","item_title":"净资产收益率","item_value":"15.50","item_precision":"f2","item_source":"lrb"},{"item_field":"ROA","item_title":"总资产报酬率","item_value":"8.20","item_precision":"f2","item_source":"lrb"},{"item_field":"ASSLIABRT","item_title":"资产负债率","item_value":"45.60","item_precision":"f2","item_source":"lrb"},{"item_field":"EPSBASIC","item_title":"基本每股收益","item_value":"1.25","item_precision":"f2","item_source":"lrb"},{"item_field":"NAPS","item_title":"每股净资产","item_value":"8.50","item_precision":"f2","item_source":"lrb"},{"item_field":"TAGRT","item_title":"营业总收入增长率","item_value":"0.10","item_precision":"f2","item_source":"lrb"},{"item_field":"NPGRT","item_title":"归属母公司净利润增长率","item_value":"0.15","item_precision":"f2","item_source":"lrb"},{"item_field":"OPPRORT","item_title":"营业利润率","item_value":"25.00","item_precision":"f2","item_source":"lrb"},{"item_field":"EBITMARGIN","item_title":"息税前利润率","item_value":"30.00","item_precision":"f2","item_source":"lrb"},{"item_field":"PROTOTCRT","item_title":"成本费用利润率","item_value":"35.00","item_precision":"f2","item_source":"lrb"},{"item_field":"CURRENTRT","item_title":"流动比率","item_value":"1.50","item_precision":"f2","item_source":"lrb"},{"item_field":"QUICKRT","item_title":"速动比率","item_value":"1.20","item_precision":"f2","item_source":"lrb"},{"item_field":"TATURNRT","item_title":"总资产周转率","item_value":"0.80","item_precision":"f2","item_source":"lrb"},{"item_field":"INVTURNRT","item_title":"存货周转率","item_value":"6.50","item_precision":"f2","item_source":"lrb"},{"item_field":"ACCRECGTURNRT","item_title":"应收账款周转率","item_value":"12.00","item_precision":"f2","item_source":"lrb"},{"item_field":"MANANETR","item_title":"经营现金流量净额","item_value":"800000.00","item_precision":"f2","item_source":"lrb"},{"item_field":"OPNCFPS","item_title":"每股经营现金流","item_value":"1.00","item_precision":"f2","item_source":"lrb"}]},"20250331":{"rType":"合并期末","rCurrency":"CNY","data_source":"其他","is_audit":"已审计","publish_date":"20250430","is_exist_yoy":false,"data":[{"item_field":"PARENETP","item_title":"归母净利润","item_value":"400000.20","item_precision":"f2","item_source":"lrb","item_tongbi":""},{"item_field":"BIZTOTINCO","item_title":"营业总收入","item_value":"2000000.00","item_precision":"f2","item_source":"lrb","item_tongbi":""}]}}}}})`

	reports, totalCount, err := parseFinancialReportResponse("600004", jsonp)
	if err != nil {
		t.Fatalf("parseFinancialReportResponse failed: %v", err)
	}

	if totalCount != 2 {
		t.Errorf("totalCount = %d, want 2", totalCount)
	}

	if len(reports) != 2 {
		t.Fatalf("expected 2 reports, got %d", len(reports))
	}

	// 验证 2025 半年报
	r0 := reports[0]
	if r0.Code != "600004" {
		t.Errorf("Code = %s, want 600004", r0.Code)
	}
	if r0.ReportDate != "20250630" {
		t.Errorf("ReportDate = %s, want 20250630", r0.ReportDate)
	}
	if r0.ReportType != 2 {
		t.Errorf("ReportType = %d, want 2", r0.ReportType)
	}
	if r0.TotalRevenue != 5000000.00 {
		t.Errorf("TotalRevenue = %.2f, want 5000000.00", r0.TotalRevenue)
	}
	if r0.TotalCost != 3000000.00 {
		t.Errorf("TotalCost = %.2f, want 3000000.00", r0.TotalCost)
	}
	if r0.NetProfit != 1000000.50 {
		t.Errorf("NetProfit = %.2f, want 1000000.50", r0.NetProfit)
	}
	if r0.NetProfitCut != 900000.00 {
		t.Errorf("NetProfitCut = %.2f, want 900000.00", r0.NetProfitCut)
	}
	if r0.GrossMargin != 40.00 {
		t.Errorf("GrossMargin = %.2f, want 40.00", r0.GrossMargin)
	}
	if r0.NetMargin != 20.00 {
		t.Errorf("NetMargin = %.2f, want 20.00", r0.NetMargin)
	}
	if r0.ROE != 15.50 {
		t.Errorf("ROE = %.2f, want 15.50", r0.ROE)
	}
	if r0.ROA != 8.20 {
		t.Errorf("ROA = %.2f, want 8.20", r0.ROA)
	}
	if r0.AssetLiabilityRatio != 45.60 {
		t.Errorf("AssetLiabilityRatio = %.2f, want 45.60", r0.AssetLiabilityRatio)
	}
	if r0.EPS != 1.25 {
		t.Errorf("EPS = %.2f, want 1.25", r0.EPS)
	}
	if r0.BPS != 8.50 {
		t.Errorf("BPS = %.2f, want 8.50", r0.BPS)
	}
	if r0.OperatingMargin != 25.00 {
		t.Errorf("OperatingMargin = %.2f, want 25.00", r0.OperatingMargin)
	}
	if r0.EBITMargin != 30.00 {
		t.Errorf("EBITMargin = %.2f, want 30.00", r0.EBITMargin)
	}
	if r0.CostProfitRatio != 35.00 {
		t.Errorf("CostProfitRatio = %.2f, want 35.00", r0.CostProfitRatio)
	}
	if r0.CurrentRatio != 1.50 {
		t.Errorf("CurrentRatio = %.2f, want 1.50", r0.CurrentRatio)
	}
	if r0.QuickRatio != 1.20 {
		t.Errorf("QuickRatio = %.2f, want 1.20", r0.QuickRatio)
	}
	if r0.TotalAssetTurnover != 0.80 {
		t.Errorf("TotalAssetTurnover = %.2f, want 0.80", r0.TotalAssetTurnover)
	}
	if r0.InventoryTurnover != 6.50 {
		t.Errorf("InventoryTurnover = %.2f, want 6.50", r0.InventoryTurnover)
	}
	if r0.ReceivablesTurnover != 12.00 {
		t.Errorf("ReceivablesTurnover = %.2f, want 12.00", r0.ReceivablesTurnover)
	}
	if r0.OperatingCashFlow != 800000.00 {
		t.Errorf("OperatingCashFlow = %.2f, want 800000.00", r0.OperatingCashFlow)
	}
	if r0.OperatingCashFlowPerShare != 1.00 {
		t.Errorf("OperatingCashFlowPerShare = %.2f, want 1.00", r0.OperatingCashFlowPerShare)
	}

	// 验证 2025 一季报（仅部分字段）
	r1 := reports[1]
	if r1.ReportDate != "20250331" {
		t.Errorf("report[1].ReportDate = %s, want 20250331", r1.ReportDate)
	}
	if r1.NetProfit != 400000.20 {
		t.Errorf("NetProfit = %.2f, want 400000.20", r1.NetProfit)
	}
	if r1.TotalRevenue != 2000000.00 {
		t.Errorf("TotalRevenue = %.2f, want 2000000.00", r1.TotalRevenue)
	}
}

// TestParseFinancialReportResponseAPIError 测试 API 返回错误码
func TestParseFinancialReportResponseAPIError(t *testing.T) {
	jsonp := `cb({"result":{"status":{"code":1}}})`
	_, _, err := parseFinancialReportResponse("000001", jsonp)
	if err == nil {
		t.Fatal("expected error for API error code")
	}
}

// TestSinaBrokerGetFinancialReportHistorical 端到端测试财报接口
func TestSinaBrokerGetFinancialReportHistorical(t *testing.T) {
	requireLiveBroker(t)
	broker := NewSinaBroker()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	reports, totalCount, err := broker.GetFinancialReportHistorical(ctx, "sh600004", 1, 5)
	if err != nil {
		t.Fatalf("fetch financial report failed: %v", err)
	}

	if totalCount <= 0 {
		t.Fatal("expected totalCount > 0")
	}

	if len(reports) == 0 {
		t.Fatal("expected at least one report")
	}

	for _, r := range reports {
		t.Logf("Report: %s type=%d, Revenue=%.2f, NetProfit=%.2f, GrossMargin=%.2f, NetMargin=%.2f, ROE=%.2f",
			r.ReportDate, r.ReportType, r.TotalRevenue, r.NetProfit, r.GrossMargin, r.NetMargin, r.ROE)
	}
}

func requireLiveBroker(t *testing.T) {
	t.Helper()
	if os.Getenv("BROKER_LIVE_TESTS") != "1" {
		t.Skip("set BROKER_LIVE_TESTS=1 to run live broker test")
	}
}

// TestSinaBrokerRetryHonorsCtxCancel 验证 retry backoff 阶段响应 ctx 取消，不会跑满所有重试。
func TestSinaBrokerRetryHonorsCtxCancel(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	b := &SinaBroker{
		client:     &http.Client{Timeout: 2 * time.Second},
		baseURL:    srv.URL,
		maxRetries: 5,
		retryDelay: 500 * time.Millisecond,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := b.getBytesURL(ctx, srv.URL, nil)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error after ctx timeout")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded, got %v", err)
	}
	// 5 次 retry 的累计 backoff 约 7.5s，ctx 80ms 超时后应立刻返回
	if elapsed > 800*time.Millisecond {
		t.Fatalf("expected fast exit on ctx cancel, took %v", elapsed)
	}
	if got := attempts.Load(); got == 0 {
		t.Fatalf("expected at least one server hit before cancel, got %d", got)
	}
}

// TestSinaBrokerRetryAlreadyCancelledCtx 验证已取消的 ctx 直接返回错误。
func TestSinaBrokerRetryAlreadyCancelledCtx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	b := &SinaBroker{
		client:     &http.Client{Timeout: 2 * time.Second},
		baseURL:    srv.URL,
		maxRetries: 3,
		retryDelay: 500 * time.Millisecond,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := b.getBytesURL(ctx, srv.URL, nil)
	if err == nil {
		t.Fatal("expected error from cancelled ctx")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

// TestSinaBrokerGetExchangeRate 测试获取单个汇率
func TestSinaBrokerGetExchangeRate(t *testing.T) {
	requireLiveBroker(t)
	broker := NewSinaBroker()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	rate, err := broker.GetExchangeRate(ctx, "USDCNY")
	if err != nil {
		t.Fatalf("fetch exchange rate failed: %v", err)
	}

	t.Logf("USDCNY: Open=%.4f Now=%.4f ChangePercent=%.2f%%",
		rate.Open, rate.Now, rate.ChangePercent)
}

// TestSinaBrokerGetExchangeRateBatch 测试批量获取汇率
func TestSinaBrokerGetExchangeRateBatch(t *testing.T) {
	requireLiveBroker(t)
	broker := NewSinaBroker()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	codes := []string{"USDCNY", "USDJPY"}
	result, err := broker.GetExchangeRateBatch(ctx, codes)
	if err != nil {
		t.Fatalf("fetch exchange rate batch failed: %v", err)
	}

	if len(result) == 0 {
		t.Fatal("expected at least one result")
	}

	for code, rate := range result {
		t.Logf("%s: Open=%.4f Now=%.4f ChangePercent=%.2f%%",
			code, rate.Open, rate.Now, rate.ChangePercent)
	}
}

// TestParseExchangeRate 测试汇率解析
func TestParseExchangeRate(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    *model.ExchangeRate
	}{
		{
			name:    "valid usdcny",
			content: "16:30:00,7.0000,7.0700,7.0500,1000,7.0800,7.0900,7.1000,7.0700,美元人民币,2025-04-22",
			want: &model.ExchangeRate{
				Code:          "USDCNY",
				Open:          7.0,
				Now:           7.07,
				ChangePercent: 1.0,
			},
		},
		{
			name:    "insufficient fields",
			content: "16:30:00,7.2000,7.2100",
			want:    nil,
		},
		{
			name:    "zero open",
			content: "16:30:00,0,7.2100,7.2050,1000,7.2150,7.2200,7.2250,7.2100,美元人民币,2025-04-22",
			want: &model.ExchangeRate{
				Code:          "USDCNY",
				Open:          0,
				Now:           7.21,
				ChangePercent: 0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseExchangeRate("USDCNY", tt.content)
			if tt.want == nil {
				if got != nil {
					t.Fatalf("expected nil, got %+v", got)
				}
				return
			}
			if got == nil {
				t.Fatal("expected non-nil rate")
			}
			if got.Code != tt.want.Code {
				t.Errorf("Code = %s, want %s", got.Code, tt.want.Code)
			}
			if got.Open != tt.want.Open {
				t.Errorf("Open = %.4f, want %.4f", got.Open, tt.want.Open)
			}
			if got.Now != tt.want.Now {
				t.Errorf("Now = %.4f, want %.4f", got.Now, tt.want.Now)
			}
			if math.Abs(got.ChangePercent-tt.want.ChangePercent) > 0.0001 {
				t.Errorf("ChangePercent = %.4f, want %.4f", got.ChangePercent, tt.want.ChangePercent)
			}
		})
	}
}
