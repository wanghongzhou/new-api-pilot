package service

import (
	"math/big"
	"time"
)

var usageSummaryLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

func todayAverageMetric(total *big.Int, now time.Time) *string {
	local := now.In(usageSummaryLocation)
	start := time.Date(
		local.Year(),
		local.Month(),
		local.Day(),
		0,
		0,
		0,
		0,
		usageSummaryLocation,
	)
	elapsedSeconds := local.Unix() - start.Unix()
	minutes := (elapsedSeconds + 59) / 60
	if minutes < 1 {
		minutes = 1
	}
	value := new(big.Rat).SetFrac(new(big.Int).Set(total), big.NewInt(minutes)).FloatString(6)
	return &value
}
