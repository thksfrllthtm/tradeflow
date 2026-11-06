package calculation

// Distribution of batch-level amounts (delivery, customs fee) across calculation items.

import (
	"strings"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/shopspring/decimal"
)

func AllocateDelivery(items []CalculationItem, totalUSD *decimal.Decimal, usd decimal.Decimal) {
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
			exact := service.MustDecimal(items[i].DeliveryUSD)
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
			value := service.MustDecimal(item.GrossKG)
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
		weight := service.MustDecimal(items[i].GrossKG)
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

func ParseItemStringDecimal(value string) *decimal.Decimal {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parsed, err := service.ParseDecimal(value)
	if err != nil {
		return nil
	}
	return &parsed
}

func AllocateFee(items []CalculationItem, fee *decimal.Decimal, included bool) {
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

func UpdateVATWithFee(item *CalculationItem) {
	if item.CustomsValueKZT == "" {
		return
	}
	vat, _ := item.DirectorySnapshot["vat"].(map[string]any)
	base := service.MustDecimal(item.CustomsValueKZT)
	if item.DutyKZT != "" {
		base = base.Add(service.MustDecimal(item.DutyKZT))
	}
	if item.AntiDumpingKZT != "" {
		base = base.Add(service.MustDecimal(item.AntiDumpingKZT))
	}
	if item.FeeKZT != "" {
		base = base.Add(service.MustDecimal(item.FeeKZT))
	}
	item.VATBaseKZT = base.StringFixed(2)
	switch service.StringValue(vat, "type") {
	case "exempt":
		item.VATKZT = "0.00"
	case "percentage":
		if item.DutyKZT == "" {
			item.VATKZT = ""
			return
		}
		rate, ok := service.DecimalValue(vat, "percent")
		if !ok || rate.IsNegative() {
			item.VATKZT = ""
			return
		}
		item.VATKZT = base.Mul(rate).Div(decimal.NewFromInt(100)).Round(2).StringFixed(2)
	default:
		item.VATKZT = ""
	}
}
