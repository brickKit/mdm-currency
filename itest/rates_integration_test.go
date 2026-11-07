//go:build integration

package itest

import (
	"net/url"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// rates: rate types; one rate per type, date and pair, the same again changes nothing, a correction is a new
// version; the effective rate; an inactive currency is refused; the list, latest first.
func (fx *fixture) rates() {
	t := fx.t
	// rates: one per type, date and pair; the same again changes nothing; a correction is a new version
	code, spot := fx.alice.do("PUT", "/api/currency/rate-types/spot", map[string]any{"name": "Spot"})
	require.Equal(t, 200, code, spot)
	code, r1 := fx.alice.do("PUT", "/api/currency/rates/spot/2026-10-15/EUR/CNY", map[string]any{"rate": "7.65"})
	require.Equal(t, 200, code, r1)
	_, r1b := fx.alice.do("PUT", "/api/currency/rates/spot/2026-10-15/EUR/CNY", map[string]any{"rate": "7.6500"})
	assert.Equal(t, r1["version"], r1b["version"], "7.6500 is 7.65")
	_, r2 := fx.alice.do("PUT", "/api/currency/rates/spot/2026-10-15/EUR/CNY", map[string]any{"rate": "7.66"})
	assert.Equal(t, r1["id"], r2["id"], "the same record")
	assert.EqualValues(t, 2, r2["version"])
	code, _ = fx.alice.do("PUT", "/api/currency/rates/spot/2026-10-18/EUR/CNY", map[string]any{"rate": "7.70"})
	require.Equal(t, 200, code)
	code, eff := fx.alice.do("GET", "/api/currency/rates/effective?rate_type=spot&date=2026-10-17&from_currency=EUR&to_currency=CNY", nil)
	require.Equal(t, 200, code, eff)
	assert.Equal(t, "7.66", eff["rate"])
	assert.Equal(t, "2026-10-15", eff["rate_date"], "the latest date not after the requested one")
	code, _ = fx.alice.do("GET", "/api/currency/rates/effective?rate_type=spot&date=2026-10-14&from_currency=EUR&to_currency=CNY", nil)
	assert.Equal(t, 404, code, "no rate that early")
	_, _ = fx.alice.do("PATCH", "/api/currency/currencies/BTN", map[string]any{"active": false})
	code, inactive := fx.alice.do("PUT", "/api/currency/rates/spot/2026-10-15/EUR/BTN", map[string]any{"rate": "30"})
	assert.Equal(t, 409, code)
	assert.Equal(t, "INACTIVE", inactive["reason"])
	_, rates := fx.alice.do("GET", "/api/currency/rates?from_currency=EUR&to_currency=CNY&limit=1", nil)
	first := rates["rates"].([]any)
	require.Len(t, first, 1)
	assert.Equal(t, "2026-10-18", first[0].(map[string]any)["rate_date"], "latest first")
	_, rest := fx.alice.do("GET", "/api/currency/rates?from_currency=EUR&to_currency=CNY&limit=1&cursor="+
		url.QueryEscape(rates["next_cursor"].(string)), nil)
	assert.Equal(t, "2026-10-15", rest["rates"].([]any)[0].(map[string]any)["rate_date"])

	fx.r1, fx.r2 = r1, r2
}
