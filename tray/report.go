package main

import "fmt"

// Report mirrors the JSON contract emitted by `td spend --json`
// (internal/spend.Report). The macOS app decodes the same shape.
type Report struct {
	Schema int `json:"schema"`
	Spend  struct {
		Today     float64 `json:"today"`
		Month     float64 `json:"month"`
		Lifetime  float64 `json:"lifetime"`
		Currency  string  `json:"currency"`
		Available bool    `json:"available"`
	} `json:"spend"`
	Saved struct {
		Today    float64 `json:"today"`
		Lifetime float64 `json:"lifetime"`
		Tokens   int     `json:"tokens"`
	} `json:"saved"`
	SharePct  float64 `json:"share_pct"`
	TDVersion string  `json:"td_version"`
}

// money is the dropdown format: always cents.
func money(v float64) string { return fmt.Sprintf("$%.2f", v) }

// moneyShort is the always-visible format: whole dollars once large.
func moneyShort(v float64) string {
	if v >= 100 {
		return fmt.Sprintf("$%.0f", v)
	}
	return fmt.Sprintf("$%.2f", v)
}

// microMoney shows enough digits to stay non-zero for sub-cent savings.
func microMoney(v float64) string {
	if v > 0 && v < 0.01 {
		return fmt.Sprintf("$%.4f", v)
	}
	return fmt.Sprintf("$%.2f", v)
}
