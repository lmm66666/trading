package broker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"trading/model"
)

// ShiborBroker Shibor 数据提供者接口
type ShiborBroker interface {
	// FetchShibor 获取 Shibor 利率数据
	// indicatorId: 001=隔夜, 002=1周, 003=2周, 004=1月, 005=3月, 006=6月, 007=9月, 008=1年
	FetchShibor(ctx context.Context, indicatorID string) ([]model.ShiborData, error)
}

// EastMoneyBroker 东方财富数据中心 API 客户端
type EastMoneyBroker struct {
	client  *http.Client
	baseURL string
}

// NewEastMoneyBroker 创建东方财富数据提供者
func NewEastMoneyBroker() *EastMoneyBroker {
	return &EastMoneyBroker{
		client:  &http.Client{Timeout: 15 * time.Second},
		baseURL: "https://datacenter-web.eastmoney.com",
	}
}

// shiborResponse API 响应结构
type shiborResponse struct {
	Success bool `json:"success"`
	Result  struct {
		Data []model.ShiborData `json:"data"`
	} `json:"result"`
}

// FetchShibor 获取 Shibor 利率数据
func (b *EastMoneyBroker) FetchShibor(ctx context.Context, indicatorID string) ([]model.ShiborData, error) {
	params := url.Values{}
	params.Set("reportName", "RPT_IMP_INTRESTRATEN")
	params.Set("columns", "REPORT_DATE,REPORT_PERIOD,IR_RATE,CHANGE_RATE,INDICATOR_ID,LATEST_RECORD,MARKET,MARKET_CODE,CURRENCY,CURRENCY_CODE")
	params.Set("filter", fmt.Sprintf(`(MARKET_CODE="001")(CURRENCY_CODE="CNY")(INDICATOR_ID="%s")`, indicatorID))
	params.Set("pageNumber", "1")
	params.Set("pageSize", "20")
	params.Set("sortTypes", "-1")
	params.Set("sortColumns", "REPORT_DATE")
	params.Set("source", "WEB")
	params.Set("client", "WEB")

	apiURL := fmt.Sprintf("%s/api/data/v1/get?%s", b.baseURL, params.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}

	req.Header.Set("Accept", "*/*")
	req.Header.Set("Referer", "https://data.eastmoney.com/shibor/shibor/001,CNY,001.html")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body failed: %w", err)
	}

	var result shiborResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse json failed: %w", err)
	}

	if !result.Success {
		return nil, fmt.Errorf("api returned error")
	}

	return result.Result.Data, nil
}

// ShiborPeriod Shibor 期限定义
type ShiborPeriod struct {
	ID   string // 指标ID
	Code string // 代码
	Name string // 显示名称
}

// 预定义的 Shibor 期限
var (
	ShiborON = ShiborPeriod{ID: "001", Code: "SHIBOR_ON", Name: "Shibor隔夜"}
	Shibor1W = ShiborPeriod{ID: "002", Code: "SHIBOR_1W", Name: "Shibor1周"}
	Shibor2W = ShiborPeriod{ID: "003", Code: "SHIBOR_2W", Name: "Shibor2周"}
	Shibor1M = ShiborPeriod{ID: "004", Code: "SHIBOR_1M", Name: "Shibor1月"}
	Shibor3M = ShiborPeriod{ID: "005", Code: "SHIBOR_3M", Name: "Shibor3月"}
	Shibor6M = ShiborPeriod{ID: "006", Code: "SHIBOR_6M", Name: "Shibor6月"}
	Shibor9M = ShiborPeriod{ID: "007", Code: "SHIBOR_9M", Name: "Shibor9月"}
	Shibor1Y = ShiborPeriod{ID: "008", Code: "SHIBOR_1Y", Name: "Shibor1年"}
)

// AllShiborPeriods 返回所有预定义的 Shibor 期限
func AllShiborPeriods() []ShiborPeriod {
	return []ShiborPeriod{ShiborON, Shibor1W, Shibor2W, Shibor1M, Shibor3M, Shibor6M, Shibor9M, Shibor1Y}
}
