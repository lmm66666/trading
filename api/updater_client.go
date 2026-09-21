package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"trading/internal/application"
	"trading/internal/market"
	"trading/internal/port"
)

const updaterRefreshPath = "/internal/v1/market/refresh"
const refreshBodyLimit = 1 << 20

// UpdaterClient is the fixed, authenticated transport for manual refresh only.
// Queries and compute never depend on the updater being available.
type UpdaterClient struct {
	endpoint string
	token    string
	http     *http.Client
}

type updaterError struct {
	status  int
	message string
}

func (e *updaterError) Error() string { return e.message }

func ValidateUpdaterToken(token string) error {
	if len(token) < 32 || strings.ContainsAny(token, " \t\r\n") {
		return errors.New("updater token must contain at least 32 non-whitespace bytes")
	}
	for _, b := range []byte(token) {
		if b < 33 || b > 126 {
			return errors.New("updater token must be printable ASCII")
		}
	}
	return nil
}

func NewUpdaterClient(origin, token string) (*UpdaterClient, error) {
	if err := ValidateUpdaterToken(token); err != nil {
		return nil, err
	}
	u, err := url.Parse(origin)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(origin, "#") {
		return nil, errors.New("updater URL must be an HTTP or HTTPS origin")
	}
	u.Path = updaterRefreshPath
	// HTTP/2 can transparently replay POST on stream errors. Refresh has no
	// idempotency key, so pin this private transport to HTTP/1.
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Protocols = new(http.Protocols)
	transport.Protocols.SetHTTP1(true)
	return &UpdaterClient{endpoint: u.String(), token: token, http: &http.Client{Transport: transport, Timeout: 40 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

func (client *UpdaterClient) refresh(ctx context.Context, input marketRefreshRequest) (int, any, error) {
	body, err := json.Marshal(input)
	if err != nil {
		return 0, nil, &updaterError{502, "UPDATER_BAD_RESPONSE"}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, client.endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, nil, &updaterError{503, "UPDATER_UNAVAILABLE"}
	}
	// Remove the replay capability inferred from bytes.Reader. A failed cached
	// HTTP/1 connection must surface an error instead of resending this POST.
	req.GetBody = nil
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+client.token)
	resp, err := client.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return 0, nil, ctx.Err()
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return 0, nil, &updaterError{504, "UPDATER_TIMEOUT"}
		}
		return 0, nil, &updaterError{503, "UPDATER_UNAVAILABLE"}
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(resp.Body, refreshBodyLimit+1))
	if err != nil {
		if ctx.Err() != nil {
			return 0, nil, ctx.Err()
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return 0, nil, &updaterError{504, "UPDATER_TIMEOUT"}
		}
		return 0, nil, &updaterError{502, "UPDATER_BAD_RESPONSE"}
	}
	bad := &updaterError{502, "UPDATER_BAD_RESPONSE"}
	if len(payload) > refreshBodyLimit {
		return 0, nil, bad
	}
	var envelope struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return 0, nil, bad
	}
	if resp.StatusCode == 200 || resp.StatusCode == 202 {
		if envelope.Code != 0 || envelope.Message != "success" {
			return 0, nil, bad
		}
		if input.Exchange == "" && resp.StatusCode == 202 {
			var data port.BatchRefreshReceipt
			if json.Unmarshal(envelope.Data, &data) != nil || !validRefreshReceipt(data.Stock, false) || !validRefreshReceipt(data.Futures, true) {
				return 0, nil, bad
			}
			return 202, data, nil
		}
		if input.Exchange != "" && resp.StatusCode == 200 {
			var data application.RefreshResult
			if json.Unmarshal(envelope.Data, &data) != nil || data.Instrument != (market.InstrumentID{Exchange: market.Exchange(input.Exchange), Code: input.Code}) || data.Version == 0 || data.Quality.Validate() != nil || data.DailyBars < 0 || data.WeeklyBars < 0 {
				return 0, nil, bad
			}
			return 200, data, nil
		}
		return 0, nil, bad
	}
	messages := map[int]string{400: "INVALID_REQUEST", 404: "NOT_FOUND", 409: "AMBIGUOUS_INSTRUMENT", 429: "MARKET_REFRESH_ALREADY_RUNNING"}
	if message, ok := messages[resp.StatusCode]; ok && envelope.Code == resp.StatusCode && envelope.Message == message {
		return 0, nil, &updaterError{resp.StatusCode, message}
	}
	return 0, nil, bad
}

// Capability failures do not prevent database-backed progress/history reads.
func (client *UpdaterClient) futuresEnabled(ctx context.Context) *bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, client.endpoint+"/status", nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Authorization", "Bearer "+client.token)
	resp, err := client.http.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil
	}
	payload, err := io.ReadAll(io.LimitReader(resp.Body, refreshBodyLimit+1))
	if err != nil || len(payload) > refreshBodyLimit {
		return nil
	}
	var envelope struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			FuturesEnabled *bool `json:"futures_enabled"`
		} `json:"data"`
	}
	if json.Unmarshal(payload, &envelope) != nil || envelope.Code != 0 || envelope.Message != "success" {
		return nil
	}
	return envelope.Data.FuturesEnabled
}

func validRefreshReceipt(receipt port.RefreshReceipt, allowDisabled bool) bool {
	switch receipt.Status {
	case "ACCEPTED":
		return port.ValidateIdentity(receipt.RunID, "run_id", 64, false) == nil && receipt.ErrorCode == ""
	case "ALREADY_RUNNING", "DISABLED":
		return (receipt.Status != "DISABLED" || allowDisabled) && receipt.RunID == "" && !receipt.ProgressAvailable && receipt.ErrorCode == ""
	case "FAILED":
		return receipt.RunID == "" && !receipt.ProgressAvailable && receipt.ErrorCode == "REFRESH_UNAVAILABLE"
	default:
		return false
	}
}
