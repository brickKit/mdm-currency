//go:build integration

package itest

import (
	"net/url"
	"time"

	"github.com/brickKit/be-sdk-go/v3/money"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// currencies: the ISO table comes with the migration and rounds to its minor unit; a deployment's decisions (cash
// rounding, translations) with the version guard; the keyword and the pages.
func (fx *fixture) currencies() {
	t, ctx := fx.t, fx.ctx
	// the ISO table comes with the migration; three decimals for KWD, none for JPY
	code, kwd := fx.alice.do("GET", "/api/currency/currencies/KWD", nil)
	require.Equal(t, 200, code, kwd)
	assert.EqualValues(t, 3, kwd["minor_unit"])
	_, jpy := fx.alice.do("GET", "/api/currency/currencies/JPY", nil)
	assert.EqualValues(t, 0, jpy["minor_unit"])
	// the boundary card's story: a three-decimal currency rounds to its minor unit
	d, err := money.Parse("12.34567")
	require.NoError(t, err)
	assert.Equal(t, "12.346", money.Round(d, int32(kwd["minor_unit"].(float64)), money.Mode(kwd["rounding"].(string))).Text('f'))

	code, _ = fx.bob.do("GET", "/api/currency/currencies/KWD", nil)
	assert.Equal(t, 404, code, "Bob holds no grant: not visible (P8.16)")

	// a deployment's decisions: cash rounding, translations; a stale version is refused; the same change twice once
	code, chf := fx.alice.do("PATCH", "/api/currency/currencies/CHF", map[string]any{"cash_unit": "0.10",
		"names": map[string]string{"zh-CN": "瑞士法郎", "de": "Schweizer Franken"}})
	require.Equal(t, 200, code, chf)
	assert.Equal(t, "0.1", chf["cash_unit"], "no trailing zeros")
	assert.EqualValues(t, 2, chf["version"], "the ISO row was version 1")
	v := chf["version"]
	code, again := fx.alice.do("PATCH", "/api/currency/currencies/CHF", map[string]any{"cash_unit": "0.1"})
	assert.Equal(t, 200, code)
	assert.Equal(t, v, again["version"], "nothing changed: no new version")
	code, stale := fx.alice.do("PATCH", "/api/currency/currencies/CHF", map[string]any{"version": 1, "active": false})
	assert.Equal(t, 409, code)
	assert.Equal(t, "VERSION_CONFLICT", stale["reason"])

	// the keyword: the translated name, its pinyin initials; pages by cursor
	require.Eventually(t, func() bool { // the backfill job writes the ISO rows' search column
		_, out := fx.alice.do("GET", "/api/currency/currencies?q=dinar&limit=200", nil)
		l, _ := out["currencies"].([]any)
		return len(l) >= 5
	}, 20*time.Second, 200*time.Millisecond)
	_, out := fx.alice.do("GET", "/api/currency/currencies?q=rsfl", nil) // 瑞士法郎: r s f l
	l, _ := out["currencies"].([]any)
	require.Len(t, l, 1)
	assert.Equal(t, "CHF", l[0].(map[string]any)["code"])
	seen, cursor := map[string]bool{}, ""
	for pages := 0; ; pages++ {
		require.Less(t, pages, 20)
		_, out := fx.alice.do("GET", "/api/currency/currencies?limit=50&cursor="+url.QueryEscape(cursor), nil)
		for _, c := range out["currencies"].([]any) {
			code := c.(map[string]any)["code"].(string)
			assert.False(t, seen[code], "no currency twice across pages")
			seen[code] = true
		}
		if cursor = out["next_cursor"].(string); cursor == "" {
			break
		}
	}
	var total int
	require.NoError(t, fx.admin.QueryRow(ctx, "SELECT count(*) FROM "+itSchema+".currency").Scan(&total))
	assert.Len(t, seen, total, "every currency once")

}
