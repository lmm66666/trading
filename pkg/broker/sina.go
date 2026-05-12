package broker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"

	"trading/model"
)

// SinaBroker 新浪财经行情数据提供者
type SinaBroker struct {
	client     *http.Client
	baseURL    string
	maxRetries int
	retryDelay time.Duration
}

// NewSinaBroker 创建新浪财经数据提供者
func NewSinaBroker() *SinaBroker {
	return &SinaBroker{
		client:     &http.Client{Timeout: 15 * time.Second},
		baseURL:    "https://hq.sinajs.cn",
		maxRetries: 3,
		retryDelay: 500 * time.Millisecond,
	}
}

func (p *SinaBroker) getBytes(ctx context.Context, path string, headers map[string]string) ([]byte, error) {
	return p.getBytesURL(ctx, p.baseURL+path, headers)
}

func (p *SinaBroker) getBytesURL(ctx context.Context, rawURL string, headers map[string]string) ([]byte, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid url: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt <= p.maxRetries; attempt++ {
		if attempt > 0 {
			timer := time.NewTimer(p.retryDelay * time.Duration(attempt))
			select {
			case <-timer.C:
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return nil, fmt.Errorf("create request failed: %w", err)
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}

		resp, err := p.client.Do(req)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, ctxErr
			}
			lastErr = err
			continue
		}

		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, ctxErr
			}
			lastErr = err
			continue
		}

		if resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("server error: status=%d", resp.StatusCode)
			continue
		}

		if resp.StatusCode >= 400 {
			return nil, fmt.Errorf("client error: status=%d", resp.StatusCode)
		}

		return body, nil
	}

	return nil, fmt.Errorf("request failed after %d retries: %w", p.maxRetries, lastErr)
}

// GetStockTodayInBatch 批量获取今日行情数据
func (p *SinaBroker) GetStockTodayInBatch(ctx context.Context, codes []string) (map[string]*model.StockKline, error) {
	if len(codes) == 0 {
		return nil, nil
	}

	body, err := p.getBytes(ctx, fmt.Sprintf("/list=%s", strings.Join(codes, ",")), map[string]string{
		"Referer": "https://finance.sina.com.cn/",
	})
	if err != nil {
		return nil, fmt.Errorf("fetch realtime batch failed: %w", err)
	}

	utf8Body, err := gbkToUTF8(body)
	if err != nil {
		return nil, fmt.Errorf("gbk to utf8 conversion failed: %w", err)
	}

	rawMap := parseRealtimeResponse(string(utf8Body))
	result := make(map[string]*model.StockKline, len(rawMap))
	for code, content := range rawMap {
		if kline := parseRealtimeToKline(code, content); kline != nil {
			result[code] = kline
		}
	}
	return result, nil
}

// GetStockToday 获取单个代码的今日数据
func (p *SinaBroker) GetStockToday(ctx context.Context, code string) (*model.StockKline, error) {
	result, err := p.GetStockTodayInBatch(ctx, []string{code})
	if err != nil {
		return nil, err
	}
	kline, ok := result[code]
	if !ok {
		return nil, fmt.Errorf("no data for code: %s", code)
	}
	return kline, nil
}

// GetStockHistorical 获取历史 K 线数据
func (p *SinaBroker) GetStockHistorical(ctx context.Context, symbol string, scale int, length int) ([]model.StockKline, error) {
	u, err := url.Parse("https://money.finance.sina.com.cn/quotes_service/api/json_v2.php/CN_MarketData.getKLineData")
	if err != nil {
		return nil, fmt.Errorf("invalid url: %w", err)
	}
	q := u.Query()
	q.Set("symbol", symbol)
	q.Set("scale", strconv.Itoa(scale))
	q.Set("ma", "no")
	q.Set("datalen", strconv.Itoa(length))
	u.RawQuery = q.Encode()

	body, err := p.getBytesURL(ctx, u.String(), map[string]string{
		"Referer": "https://finance.sina.com.cn/",
	})
	if err != nil {
		return nil, fmt.Errorf("fetch historical failed: %w", err)
	}

	return parseKLineResponse(symbol, string(body))
}

// --- parser ---

var realtimeRegex = regexp.MustCompile(`var hq_str_([^=]+)="([^"]*)"`)

func parseRealtimeResponse(body string) map[string]string {
	result := make(map[string]string)
	for _, m := range realtimeRegex.FindAllStringSubmatch(body, -1) {
		if len(m) >= 3 && m[2] != "" {
			result[m[1]] = m[2]
		}
	}
	return result
}

func parseRealtimeToKline(code, content string) *model.StockKline {
	fields := strings.Split(content, ",")
	if len(fields) < 5 {
		return nil
	}

	if IsStockCode(code) && len(fields) >= 32 {
		return parseStockToKline(code, fields)
	}
	return parseExchangeToKline(code, fields)
}

func parseStockToKline(code string, fields []string) *model.StockKline {
	open, _ := strconv.ParseFloat(fields[1], 64)
	high, _ := strconv.ParseFloat(fields[4], 64)
	low, _ := strconv.ParseFloat(fields[5], 64)
	closePrice, _ := strconv.ParseFloat(fields[3], 64)
	volume, _ := strconv.ParseInt(fields[8], 10, 64)

	date := fields[30]
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}

	return &model.StockKline{
		Code:   code,
		Date:   date,
		Open:   open,
		High:   high,
		Low:    low,
		Close:  closePrice,
		Volume: volume,
	}
}

func parseExchangeToKline(code string, fields []string) *model.StockKline {
	open, _ := strconv.ParseFloat(fields[1], 64)
	closePrice, _ := strconv.ParseFloat(fields[2], 64)
	high, _ := strconv.ParseFloat(fields[3], 64)
	volume, _ := strconv.ParseInt(fields[4], 10, 64)

	var date string
	if len(fields) > 10 {
		date = fields[10]
	}
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}

	return &model.StockKline{
		Code:   code,
		Date:   date,
		Open:   open,
		High:   high,
		Low:    open,
		Close:  closePrice,
		Volume: volume,
	}
}

// IsStockCode 判断是否为股票代码（sh/sz/hk 开头）
func IsStockCode(code string) bool {
	return strings.HasPrefix(code, "sh") ||
		strings.HasPrefix(code, "sz") ||
		strings.HasPrefix(code, "hk")
}

// --- helpers ---

func gbkToUTF8(data []byte) ([]byte, error) {
	reader := transform.NewReader(bytes.NewReader(data), simplifiedchinese.GBK.NewDecoder())
	return io.ReadAll(reader)
}

type kLineItem struct {
	Day    string `json:"day"`
	Open   string `json:"open"`
	High   string `json:"high"`
	Low    string `json:"low"`
	Close  string `json:"close"`
	Volume string `json:"volume"`
}

func parseKLineResponse(symbol, body string) ([]model.StockKline, error) {
	var items []kLineItem
	if err := json.Unmarshal([]byte(body), &items); err != nil {
		return nil, fmt.Errorf("parse kline failed: %w", err)
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("no kline data")
	}

	result := make([]model.StockKline, 0, len(items))
	for _, item := range items {
		open, _ := strconv.ParseFloat(item.Open, 64)
		high, _ := strconv.ParseFloat(item.High, 64)
		low, _ := strconv.ParseFloat(item.Low, 64)
		closePrice, _ := strconv.ParseFloat(item.Close, 64)
		volume, _ := strconv.ParseInt(item.Volume, 10, 64)

		date := item.Day
		if len(date) > 10 {
			date = date[:10]
		}

		result = append(result, model.StockKline{
			Code:   symbol,
			Date:   date,
			Open:   open,
			High:   high,
			Low:    low,
			Close:  closePrice,
			Volume: volume,
		})
	}
	return result, nil
}

// GetFinancialReportHistorical 获取历史财报数据
func (p *SinaBroker) GetFinancialReportHistorical(ctx context.Context, symbol string, page, num int) ([]*model.FinancialReport, int, error) {
	if page < 1 {
		page = 1
	}
	if num < 1 {
		num = 10
	}

	callback := fmt.Sprintf("fin_cb_%d", time.Now().UnixNano())
	u, err := url.Parse("https://quotes.sina.cn/cn/api/openapi.php/CompanyFinanceService.getFinanceReport2022")
	if err != nil {
		return nil, 0, fmt.Errorf("invalid url: %w", err)
	}
	q := u.Query()
	q.Set("paperCode", symbol)
	q.Set("source", "gjzb")
	q.Set("type", "0")
	q.Set("page", strconv.Itoa(page))
	q.Set("num", strconv.Itoa(num))
	q.Set("callback", callback)
	u.RawQuery = q.Encode()

	body, err := p.getBytesURL(ctx, u.String(), map[string]string{
		"Referer": "https://money.finance.sina.com.cn/",
	})
	if err != nil {
		return nil, 0, fmt.Errorf("fetch financial report failed: %w", err)
	}

	code := strings.TrimPrefix(symbol, "sh")
	code = strings.TrimPrefix(code, "sz")

	return parseFinancialReportResponse(code, string(body))
}

// --- financial report parser ---

// 原始 API 数据结构，仅在 broker 层内部使用

type financialReportDateItem struct {
	DateValue string `json:"date_value"`
	DateDesc  string `json:"date_description"`
	DateType  int    `json:"date_type"`
}

type financialReportIndicator struct {
	ItemField     string `json:"item_field"`
	ItemTitle     string `json:"item_title"`
	ItemValue     string `json:"item_value"`
	ItemTongbi    any    `json:"item_tongbi"`
	ItemPrecision string `json:"item_precision"`
	ItemSource    string `json:"item_source"`
}

type financialReportDetail struct {
	RType       string                     `json:"rType"`
	RCurrency   string                     `json:"rCurrency"`
	DataSource  string                     `json:"data_source"`
	IsAudit     string                     `json:"is_audit"`
	PublishDate string                     `json:"publish_date"`
	IsExistYOY  bool                       `json:"is_exist_yoy"`
	Data        []financialReportIndicator `json:"data"`
}

type financialReportData struct {
	ReportCount string                           `json:"report_count"`
	ReportDate  []financialReportDateItem        `json:"report_date"`
	ReportList  map[string]financialReportDetail `json:"report_list"`
}

type financialReportResponse struct {
	Result struct {
		Status struct {
			Code int `json:"code"`
		} `json:"status"`
		Data *financialReportData `json:"data"`
	} `json:"result"`
}

func extractJSONP(body string) (string, error) {
	body = strings.TrimPrefix(body, "/*<script>location.href='//sina.com';</script>*/")
	body = strings.TrimSpace(body)

	start := strings.Index(body, "(")
	end := strings.LastIndex(body, ")")
	if start == -1 || end == -1 || start >= end {
		return "", fmt.Errorf("invalid JSONP format")
	}
	return body[start+1 : end], nil
}

func parseFinancialReportResponse(code, body string) ([]*model.FinancialReport, int, error) {
	jsonStr, err := extractJSONP(body)
	if err != nil {
		return nil, 0, err
	}

	var resp financialReportResponse
	if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil {
		return nil, 0, fmt.Errorf("parse financial report JSON failed: %w", err)
	}

	if resp.Result.Status.Code != 0 {
		return nil, 0, fmt.Errorf("financial report API error: code=%d", resp.Result.Status.Code)
	}

	if resp.Result.Data == nil {
		return nil, 0, fmt.Errorf("no financial report data")
	}

	totalCount, _ := strconv.Atoi(resp.Result.Data.ReportCount)
	reports := make([]*model.FinancialReport, 0, len(resp.Result.Data.ReportDate))

	for _, dateItem := range resp.Result.Data.ReportDate {
		detail, ok := resp.Result.Data.ReportList[dateItem.DateValue]
		if !ok {
			continue
		}

		report := &model.FinancialReport{
			Code:       code,
			ReportDate: dateItem.DateValue,
			ReportType: dateItem.DateType,
		}
		fillFinancialReport(report, detail)
		reports = append(reports, report)
	}

	return reports, totalCount, nil
}

func buildFieldMap(data []financialReportIndicator) map[string]financialReportIndicator {
	m := make(map[string]financialReportIndicator, len(data))
	for _, ind := range data {
		if ind.ItemField != "" {
			m[ind.ItemField] = ind
		}
	}
	return m
}

func setFloat(fv map[string]financialReportIndicator, field string, target *float64) {
	if ind, ok := fv[field]; ok {
		*target, _ = strconv.ParseFloat(ind.ItemValue, 64)
	}
}

// setFloatAlt 尝试主字段名，不存在则尝试备选字段名
// 用于兼容银行类企业（如浦发银行）与普通企业的不同字段命名
func setFloatAlt(fv map[string]financialReportIndicator, primary, alt string, target *float64) {
	if ind, ok := fv[primary]; ok {
		*target, _ = strconv.ParseFloat(ind.ItemValue, 64)
		return
	}
	if ind, ok := fv[alt]; ok {
		*target, _ = strconv.ParseFloat(ind.ItemValue, 64)
	}
}

func fillFinancialReport(report *model.FinancialReport, detail financialReportDetail) {
	fv := buildFieldMap(detail.Data)

	// 利润表（银行类企业字段名不同：BIZINCO/BIZEXPE/NETPARECOMPPROF）
	setFloatAlt(fv, "BIZTOTINCO", "BIZINCO", &report.TotalRevenue)
	setFloatAlt(fv, "BIZTOTCOST", "BIZEXPE", &report.TotalCost)
	setFloatAlt(fv, "PARENETP", "NETPARECOMPPROF", &report.NetProfit)
	setFloat(fv, "NPCUT", &report.NetProfitCut)

	// 盈利能力
	setFloat(fv, "SGPMARGIN", &report.GrossMargin)
	setFloat(fv, "SNPMARGINCONMS", &report.NetMargin)
	setFloat(fv, "ROEWEIGHTED", &report.ROE)
	setFloat(fv, "ROA", &report.ROA)

	// 偿债能力
	setFloat(fv, "ASSLIABRT", &report.AssetLiabilityRatio)
	setFloat(fv, "CURRENTRT", &report.CurrentRatio)
	setFloat(fv, "QUICKRT", &report.QuickRatio)

	// 运营效率
	setFloat(fv, "TATURNRT", &report.TotalAssetTurnover)
	setFloat(fv, "INVTURNRT", &report.InventoryTurnover)
	setFloat(fv, "ACCRECGTURNRT", &report.ReceivablesTurnover)

	// 现金流
	setFloat(fv, "MANANETR", &report.OperatingCashFlow)
	setFloat(fv, "OPNCFPS", &report.OperatingCashFlowPerShare)

	// 每股指标
	setFloat(fv, "EPSBASIC", &report.EPS)
	setFloat(fv, "NAPS", &report.BPS)

	// 其他盈利指标
	setFloat(fv, "OPPRORT", &report.OperatingMargin)
	setFloat(fv, "EBITMARGIN", &report.EBITMargin)
	setFloat(fv, "PROTOTCRT", &report.CostProfitRatio)
}
