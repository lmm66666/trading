package backtest

import (
	"trading/internal/market"
)

type RejectReason uint8

const (
	RejectNone RejectReason = iota
	RejectInvalidConfig
	RejectInvalidOrder
	RejectNotYetActive
	RejectNotTradable
	RejectLimitUp
	RejectLimitDown
	RejectInvalidLot
	RejectInsufficientCash
	RejectInsufficientPosition
	RejectDuplicateFill
	RejectArithmeticOverflow
	RejectInvalidBar
)

// ExecutionModel turns an eligible order into a fill without mutating Account.
// The caller applies the fill exactly once after recording it.
type ExecutionModel struct{ config Config }

func NewExecutionModel(config Config) (ExecutionModel, error) {
	if err := config.Validate(); err != nil {
		return ExecutionModel{}, err
	}
	return ExecutionModel{config: config}, nil
}

func (m ExecutionModel) Execute(order Order, bar market.Bar, account Account) (Fill, RejectReason) {
	if err := m.config.Validate(); err != nil {
		return Fill{}, RejectInvalidConfig
	}
	if !order.validFor(bar) {
		return Fill{}, RejectInvalidOrder
	}
	if !bar.CloseTime.After(order.CreatedAt) {
		return Fill{}, RejectNotYetActive
	}
	if account.hasFilledOrder(order.ID) {
		return Fill{}, RejectDuplicateFill
	}
	if reason := m.canFill(order, bar); reason != RejectNone {
		return Fill{}, reason
	}
	price, ok := executionPrice(bar.Open, bar.Low, bar.High, order.Side, m.config.SlippageBPS)
	if !ok {
		return Fill{}, RejectArithmeticOverflow
	}
	if order.Side == Buy {
		return m.buy(order, bar, account, price)
	}
	return m.sell(order, bar, account, price)
}

func (m ExecutionModel) canFill(order Order, bar market.Bar) RejectReason {
	if bar.Trading != market.Tradable || bar.Volume <= 0 {
		return RejectNotTradable
	}
	if bar.Instrument.Validate() != nil || bar.Open <= 0 || bar.Low <= 0 || bar.High <= 0 || bar.Low > bar.Open || bar.Open > bar.High {
		return RejectInvalidBar
	}
	if order.Side == Buy && bar.LimitUp != nil && bar.Open >= *bar.LimitUp {
		return RejectLimitUp
	}
	if order.Side == Sell && bar.LimitDown != nil && bar.Open <= *bar.LimitDown {
		return RejectLimitDown
	}
	return RejectNone
}

func (m ExecutionModel) buy(order Order, bar market.Bar, account Account, price market.Price) (Fill, RejectReason) {
	budget, ok := mulDivFloor(int64(account.Cash()), m.config.CashFractionBPS, basisPoints)
	if !ok {
		return Fill{}, RejectArithmeticOverflow
	}
	maxQuantity := budget / int64(price)
	if maxQuantity < m.config.LotSize {
		return Fill{}, RejectInsufficientCash
	}
	maxLots := maxQuantity / m.config.LotSize
	low, high := int64(0), maxLots
	for low < high {
		mid := low + (high-low+1)/2
		quantity, ok := mulDivFloor(mid, m.config.LotSize, 1)
		if !ok {
			return Fill{}, RejectArithmeticOverflow
		}
		fill, ok := m.makeFill(order, bar, price, quantity)
		if !ok {
			high = mid - 1
			continue
		}
		if fill.TotalDebit() <= market.Money(budget) {
			low = mid
		} else {
			high = mid - 1
		}
	}
	if low == 0 {
		return Fill{}, RejectInsufficientCash
	}
	quantity, ok := mulDivFloor(low, m.config.LotSize, 1)
	if !ok {
		return Fill{}, RejectArithmeticOverflow
	}
	fill, ok := m.makeFill(order, bar, price, quantity)
	if !ok {
		return Fill{}, RejectArithmeticOverflow
	}
	return fill, RejectNone
}

func (m ExecutionModel) sell(order Order, bar market.Bar, account Account, price market.Price) (Fill, RejectReason) {
	position, exists := account.Position(bar.Instrument)
	if !exists || position.Quantity <= 0 {
		return Fill{}, RejectInsufficientPosition
	}
	quantity := order.Quantity
	if quantity == 0 {
		quantity = position.Quantity
	}
	if quantity > position.Quantity {
		return Fill{}, RejectInsufficientPosition
	}
	fill, ok := m.makeFill(order, bar, price, quantity)
	if !ok {
		return Fill{}, RejectArithmeticOverflow
	}
	return fill, RejectNone
}

func (m ExecutionModel) makeFill(order Order, bar market.Bar, price market.Price, quantity int64) (Fill, bool) {
	gross, ok := mulDivFloor(int64(price), quantity, 1)
	if !ok {
		return Fill{}, false
	}
	commission, stampDuty, transferFee, ok := tradeFees(m.config, order.Side, market.Money(gross))
	if !ok {
		return Fill{}, false
	}
	if _, ok = addMoney(market.Money(gross), commission, stampDuty, transferFee); !ok {
		return Fill{}, false
	}
	return Fill{ID: order.ID + "@" + bar.OpenTime.UTC().Format("20060102T150405.999999999Z"), OrderID: order.ID, Side: order.Side, Instrument: bar.Instrument, Time: bar.OpenTime, Price: price, Quantity: quantity, Gross: market.Money(gross), Commission: commission, StampDuty: stampDuty, TransferFee: transferFee}, true
}
