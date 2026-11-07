package currencies

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	core "github.com/brickKit/mdm-currency/v3/internal/core"

	"github.com/brickKit/be-sdk-go/v3/db"
	"github.com/brickKit/be-sdk-go/v3/httpx"
	"github.com/brickKit/be-sdk-go/v3/list"
	"github.com/brickKit/be-sdk-go/v3/money"
	"github.com/brickKit/be-sdk-go/v3/search"
)

func GetCurrency(m *core.Base, c *httpx.Context) error {
	ctx, _, err := core.Check(c, core.OpRead)
	if err != nil {
		return err
	}
	var cur core.Currency
	err = m.RT.DB.Tx(ctx, func(ctx context.Context, tx db.Tx) error {
		cur, err = core.GetCurrency(ctx, tx, c.Param("code"), false)
		return err
	})
	if errors.Is(err, core.ErrNone) {
		return core.NotFound("currency")
	}
	if err != nil {
		return err
	}
	return c.JSON(200, cur)
}

type currencyPatch struct {
	Version  *int64             `json:"version"`
	Active   *bool              `json:"active"`
	Rounding *string            `json:"rounding"`
	CashUnit json.RawMessage    `json:"cash_unit"` // absent: unchanged; null: none
	Names    *map[string]string `json:"names"`
}

// UpdateCurrency changes what a deployment may decide about an ISO currency; the ISO data itself stays.
func UpdateCurrency(m *core.Base, c *httpx.Context) error {
	ctx, actor, err := core.Check(c, core.OpManage)
	if err != nil {
		return err
	}
	var in currencyPatch
	if err := c.BindJSON(&in); err != nil {
		return err
	}
	if in.Rounding != nil && *in.Rounding != string(money.HalfAwayFromZero) && *in.Rounding != string(money.HalfEven) {
		return core.Invalid("rounding", "rounding is half_away_from_zero or half_even")
	}
	var cash *string
	setCash := len(in.CashUnit) > 0
	if setCash && string(in.CashUnit) != "null" {
		var s string
		if json.Unmarshal(in.CashUnit, &s) != nil || !core.Positive(s, 4) {
			return core.Invalid("cash_unit", "cash_unit is a positive decimal string with at most 4 decimal places, or null")
		}
		cash = &s
	}
	if in.Names != nil {
		if err := core.CheckNames(*in.Names); err != nil {
			return err
		}
	}
	var out core.Currency
	err = m.RT.DB.Tx(ctx, func(ctx context.Context, tx db.Tx) error {
		cur, err := core.GetCurrency(ctx, tx, c.Param("code"), true)
		if err != nil {
			return err
		}
		if in.Version != nil && *in.Version != cur.Version {
			return core.ErrVersion.New(fmt.Sprintf("the currency is at version %d", cur.Version))
		}
		next := cur
		if in.Active != nil {
			next.Active = *in.Active
		}
		if in.Rounding != nil {
			next.Rounding = *in.Rounding
		}
		if setCash {
			next.CashUnit = cash
		}
		if in.Names != nil {
			next.Names = *in.Names
		}
		if sameCurrency(cur, next) { // nothing changes: no new version, no event
			out = cur
			return nil
		}
		names, _ := json.Marshal(next.Names)
		if _, err := tx.Exec(ctx, `UPDATE currency SET active = $2, rounding = $3, cash_unit = $4::numeric, names = $5,
			search = $6, search_version = $7, version = version + 1, updated_at = now() WHERE code = $1`,
			cur.Code, next.Active, next.Rounding, next.CashUnit, names, currencySearch(next), search.Version); err != nil {
			return err
		}
		if out, err = core.GetCurrency(ctx, tx, cur.Code, false); err != nil {
			return err
		}
		if err := core.Emit(m, ctx, tx, core.CurrencyChanged, out.Code, out.Version, out); err != nil {
			return err
		}
		return core.Change(m, ctx, tx, actor, core.OpManage, core.Currencies, out.Code, out)
	})
	if errors.Is(err, core.ErrNone) {
		return core.NotFound("currency")
	}
	if err != nil {
		return err
	}
	return c.JSON(200, out)
}

func sameCurrency(a, b core.Currency) bool {
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	return string(ja) == string(jb)
}

// currencySearch is the currency's search column: code and every name (protocol 14's list keyword).
func currencySearch(c core.Currency) string {
	vals := []string{c.Code, c.Name}
	for _, n := range c.Names {
		vals = append(vals, n)
	}
	return search.Text(vals...)
}

// The lists of the user surface (protocol 14: cursor pages, a stable order ending in the record's key, filters and
// the keyword declared here). Every field is ordinary, so a row is either listed whole or not at all.
var currencyList = list.Spec{Operation: core.OpRead, Alias: "c", IDColumn: "code", IDKind: list.Text,
	Columns: []list.Column{{Field: "active", Column: "active", Kind: list.Boolean, Filterable: true}},
	Search:  []list.SearchColumn{{Name: "names", Column: "search", Fields: []string{"code", "name", "names"}}}}

func ListCurrencies(m *core.Base, c *httpx.Context) error {
	ctx, q, err := core.Build(c, currencyList, list.Request{Filters: core.Eq("active", c.Query("active")), Q: c.Query("q")})
	if err != nil {
		return err
	}
	out, next, err := list.Page(ctx, m.RT.DB, q, "currency", core.CurrencyCols, core.ScanCurrency,
		func(v core.Currency) ([]string, string) { return nil, v.Code })
	if err != nil {
		return err
	}
	return c.JSON(200, map[string]any{"currencies": out, "next_cursor": next})
}

func Backfill(m *core.Base, ctx context.Context) error {
	for {
		n := 0
		err := m.RT.DB.BackgroundTx(ctx, func(ctx context.Context, tx db.Tx) error {
			rows, err := tx.Query(ctx, `SELECT `+core.CurrencyCols+` FROM currency
				WHERE search_version IS DISTINCT FROM $1 ORDER BY code LIMIT 100 FOR UPDATE SKIP LOCKED`, search.Version)
			if err != nil {
				return err
			}
			var due []core.Currency
			for rows.Next() {
				c, err := core.ScanCurrency(rows)
				if err != nil {
					rows.Close()
					return err
				}
				due = append(due, c)
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				return err
			}
			for _, c := range due { // the search column only: not a change of the record, no new version
				if _, err := tx.Exec(ctx, `UPDATE currency SET search = $2, search_version = $3 WHERE code = $1`,
					c.Code, currencySearch(c), search.Version); err != nil {
					return err
				}
			}
			n = len(due)
			return nil
		})
		if err != nil || n < 100 {
			return err
		}
	}
}
