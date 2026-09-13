package market

import (
	"errors"
	"math"
)

var ErrBarOverflow = errors.New("market: bar aggregate overflow")

func AggregateWeekly(id InstrumentID, daily []Bar) ([]Bar, error) {
	if len(daily) == 0 {
		return nil, nil
	}
	dataset, err := NewDataset(id, Day, 0, daily)
	if err != nil {
		return nil, err
	}
	ordered := dataset.Bars()
	weekly := make([]Bar, 0, len(ordered)/5+1)
	var current Bar
	var currentKey [2]int
	for index, day := range ordered {
		year, week := day.CloseTime.ISOWeek()
		key := [2]int{year, week}
		if index == 0 || key != currentKey {
			if index > 0 {
				weekly = append(weekly, current)
			}
			currentKey = key
			current = Bar{
				Instrument: id,
				Timeframe:  Week,
				OpenTime:   day.OpenTime,
				CloseTime:  day.CloseTime,
				Open:       day.Open,
				High:       day.High,
				Low:        day.Low,
				Close:      day.Close,
				Volume:     day.Volume,
				Amount:     day.Amount,
				Trading:    day.Trading,
			}
			continue
		}
		if day.High > current.High {
			current.High = day.High
		}
		if day.Low < current.Low {
			current.Low = day.Low
		}
		current.Close = day.Close
		current.CloseTime = day.CloseTime
		if current.Volume > math.MaxInt64-day.Volume || int64(current.Amount) > math.MaxInt64-int64(day.Amount) {
			return nil, ErrBarOverflow
		}
		current.Volume += day.Volume
		current.Amount += day.Amount
		if day.Trading > current.Trading {
			current.Trading = day.Trading
		}
	}
	weekly = append(weekly, current)
	return weekly, nil
}
