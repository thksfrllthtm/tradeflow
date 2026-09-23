package service

// Distribution of batch-level amounts (delivery, customs fee) across calculation items.

import (
	"strings"

	"github.com/shopspring/decimal"
)

func allocateDelivery(items []CalculationItem, totalUSD *decimal.Decimal, rates ...decimal.Decimal) {
	// The variadic tail keeps source compatibility with the former internal
	// helper signature (wagons, tariff, usd) while the service now passes the
	// already resolved party total and usd rate.
	var usd decimal.Decimal
	if len(rates) == 1 {
		usd = rates[0]
	} else if len(rates) >= 2 {
		if totalUSD != nil {
			total := totalUSD.Mul(rates[0]).Round(2)
			totalUSD = &total
		}
		usd = rates[1]
	} else {
		return
	}
	// Values previously distributed by another calculation are never a source
	// of truth. Keep only explicitly edited manual rows; all automatic rows are
	// rebuilt from the active party delivery mode.
	manual := false
	for i := range items {
		if items[i].Excluded {
			items[i].DeliveryUSD, items[i].DeliveryKZT = "", ""
			items[i].deliveryShareExact = nil
			continue
		}
		if items[i].DeliveryManual {
			manual = true
		} else {
			items[i].DeliveryUSD, items[i].DeliveryKZT = "", ""
			items[i].deliveryShareExact = nil
		}
	}
	if totalUSD == nil || totalUSD.IsNegative() {
		return
	}
	total := totalUSD.Round(2)
	totalKZT := total.Mul(usd).Round(2)
	if manual {
		// Manual USD shares remain operator input. Their KZT values are derived
		// deterministically, with the residual on the last manual row.
		remainingKZT := totalKZT
		eligible := 0
		for _, item := range items {
			if !item.Excluded && item.DeliveryManual && item.DeliveryUSD != "" {
				eligible++
			}
		}
		seen := 0
		for i := range items {
			if items[i].Excluded || !items[i].DeliveryManual || items[i].DeliveryUSD == "" {
				continue
			}
			seen++
			exact := mustDecimal(items[i].DeliveryUSD)
			items[i].deliveryShareExact = &exact
			share := exact.Mul(usd).Round(2)
			if seen == eligible {
				share = remainingKZT
			}
			remainingKZT = remainingKZT.Sub(share)
			items[i].DeliveryKZT = share.StringFixed(2)
		}
		return
	}
	var gross decimal.Decimal
	eligible := 0
	for _, item := range items {
		if !item.Excluded && item.GrossKG != "" {
			value := mustDecimal(item.GrossKG)
			if value.IsPositive() {
				gross = gross.Add(value)
				eligible++
			}
		}
	}
	if !gross.IsPositive() || eligible == 0 {
		return
	}
	remainingUSD, remainingKZT := total, totalKZT
	displayTotalUSD := decimal.Zero
	seen := 0
	for i := range items {
		if items[i].Excluded || items[i].GrossKG == "" {
			continue
		}
		weight := mustDecimal(items[i].GrossKG)
		if !weight.IsPositive() {
			continue
		}
		seen++
		// The exact proportional share feeds the customs value. Rounding to two
		// decimals happens only at the monetary display boundary (TC, duty, VAT),
		// never on this intermediate delivery share.
		exactShare := total.Mul(weight).Div(gross)
		if seen == eligible {
			exactShare = remainingUSD
		}
		remainingUSD = remainingUSD.Sub(exactShare)
		items[i].deliveryShareExact = &exactShare
		displayShare := exactShare.Round(2)
		if seen == eligible {
			// Keep the displayed shares summing exactly to the party total; the
			// exact share above still drives the customs value.
			displayShare = total.Sub(displayTotalUSD)
		}
		displayTotalUSD = displayTotalUSD.Add(displayShare)
		items[i].DeliveryUSD = displayShare.StringFixed(2)
		shareKZT := exactShare.Mul(usd).Round(2)
		if seen == eligible {
			shareKZT = remainingKZT
		}
		remainingKZT = remainingKZT.Sub(shareKZT)
		items[i].DeliveryKZT = shareKZT.StringFixed(2)
	}
}

func hasDeliveryInput(input CalculationInput) bool {
	if strings.TrimSpace(input.Wagons) != "" || strings.TrimSpace(input.DeliveryTotalUSD) != "" {
		return true
	}
	for _, item := range input.Items {
		if strings.TrimSpace(item.DeliveryUSD) != "" || item.DeliveryManual {
			return true
		}
	}
	return false
}

func parseItemStringDecimal(value string) *decimal.Decimal {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parsed, err := parseDecimal(value)
	if err != nil {
		return nil
	}
	return &parsed
}

func allocateFee(items []CalculationItem, fee *decimal.Decimal, included bool) {
	if fee == nil || !included {
		return
	}
	feeRounded := fee.Round(2)
	eligible := 0
	for _, item := range items {
		if !item.Excluded {
			eligible++
		}
	}
	if eligible == 0 {
		return
	}
	// The one-per-declaration fee is split equally between the item positions
	// of the declaration. The base share is floored to whole tiyns so the
	// rounded shares can never exceed the total; the remaining tiyns are
	// assigned one by one to the first positions. The sum therefore always
	// equals the entered fee exactly.
	count := decimal.NewFromInt(int64(eligible))
	baseShare := feeRounded.Div(count).RoundFloor(2)
	remaining := feeRounded.Sub(baseShare.Mul(count))
	oneTiyn := decimal.New(1, -2)
	for i := range items {
		if items[i].Excluded {
			items[i].FeeKZT = ""
			continue
		}
		share := baseShare
		if remaining.IsPositive() {
			share = share.Add(oneTiyn)
			remaining = remaining.Sub(oneTiyn)
		}
		items[i].FeeKZT = share.StringFixed(2)
	}
}

func updateVATWithFee(item *CalculationItem) {
	if item.CustomsValueKZT == "" {
		return
	}
	vat, _ := item.DirectorySnapshot["vat"].(map[string]any)
	base := mustDecimal(item.CustomsValueKZT)
	if item.DutyKZT != "" {
		base = base.Add(mustDecimal(item.DutyKZT))
	}
	if item.AntiDumpingKZT != "" {
		base = base.Add(mustDecimal(item.AntiDumpingKZT))
	}
	if item.FeeKZT != "" {
		base = base.Add(mustDecimal(item.FeeKZT))
	}
	item.VATBaseKZT = base.StringFixed(2)
	switch stringValue(vat, "type") {
	case "exempt":
		item.VATKZT = "0.00"
	case "percentage":
		if item.DutyKZT == "" {
			item.VATKZT = ""
			return
		}
		rate, ok := decimalValue(vat, "percent")
		if !ok || rate.IsNegative() {
			item.VATKZT = ""
			return
		}
		item.VATKZT = base.Mul(rate).Div(decimal.NewFromInt(100)).Round(2).StringFixed(2)
	default:
		item.VATKZT = ""
	}
}
