package rates

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	core "github.com/brickKit/mdm-currency/v3/internal/core"

	"github.com/brickKit/be-sdk-go/v3/db"
	"github.com/brickKit/be-sdk-go/v3/errs"
	"github.com/brickKit/be-sdk-go/v3/httpx"
	"github.com/brickKit/be-sdk-go/v3/ids"
	"github.com/brickKit/be-sdk-go/v3/list"
	"github.com/jackc/pgx/v5"
)

type rateTypeIn struct {
	Name   string            `json:"name"`
	Names  map[string]string `json:"names"`
	Active *bool             `json:"active"`
}

// PutRateType creates the rate type or changes it; the same content again changes nothing.
func PutRateType(m *core.Base, c *httpx.Context) error {
	ctx, actor, err := core.Check(c, core.OpManage)
	if err != nil {
		return err
	}
	code := c.Param("code")
	if !core.TypeCode.MatchString(code) {
		return core.Invalid("code", "a rate type's code is lower case letters, digits and _, at most 32, starting with a letter")
	}
	var in rateTypeIn
	if err := c.BindJSON(&in); err != nil {
		return err
	}
	if strings.TrimSpace(in.Name) == "" {
		return core.Invalid("name", "name is required")
	}
	if in.Names == nil {
		in.Names = map[string]string{}
	}
	if err := core.CheckNames(in.Names); err != nil {
		return err
	}
	active := in.Active == nil || *in.Active
	var out core.RateType
	err = m.RT.DB.Tx(ctx, func(ctx context.Context, tx db.Tx) error {
		names, _ := json.Marshal(in.Names)
		cur, err := core.GetRateType(ctx, tx, code, true)
		switch {
		case errors.Is(err, core.ErrNone):
			if _, err := tx.Exec(ctx, `INSERT INTO rate_type (code, active, name, names) VALUES ($1, $2, $3, $4)
				ON CONFLICT (code) DO NOTHING`, code, active, in.Name, names); err != nil {
				return err
			}
		case err != nil:
			return err
		case cur.Active == active && cur.Name == in.Name && core.SameNames(cur.Names, in.Names):
			out = cur
			return nil
		default:
			if _, err := tx.Exec(ctx, `UPDATE rate_type SET active = $2, name = $3, names = $4, version = version + 1,
				updated_at = now() WHERE code = $1`, code, active, in.Name, names); err != nil {
				return err
			}
		}
		if out, err = core.GetRateType(ctx, tx, code, false); err != nil {
			return err
		}
		if err := core.Emit(m, ctx, tx, core.RateTypeChanged, out.Code, out.Version, out); err != nil {
			return err
		}
		return core.Change(m, ctx, tx, actor, core.OpManage, core.Currencies, "rate_type:"+out.Code, out)
	})
	if err != nil {
		return err
	}
	return c.JSON(200, out)
}

type rateIn struct {
	Rate   string `json:"rate"`
	Source string `json:"source"`
}

// PutRate enters or corrects the one rate of a type, date and pair (the boundary card's invariant): a correction is
// a new version of the same record, published again; the same rate again changes nothing.
func PutRate(m *core.Base, c *httpx.Context) error {
	ctx, actor, err := core.Check(c, core.OpMaintain)
	if err != nil {
		return err
	}
	typ, date, from, to := c.Param("rate_type"), c.Param("date"), c.Param("from_currency"), c.Param("to_currency")
	if err := core.CheckPair(typ, date, from, to); err != nil {
		return err
	}
	var in rateIn
	if err := c.BindJSON(&in); err != nil {
		return err
	}
	if !core.Positive(in.Rate, 10) {
		return core.Invalid("rate", "rate is a positive decimal string with at most 10 decimal places")
	}
	if in.Source == "" {
		in.Source = "manual"
	}
	var out core.Rate
	err = m.RT.DB.Tx(ctx, func(ctx context.Context, tx db.Tx) error {
		var ok bool
		if err := tx.QueryRow(ctx, `SELECT (SELECT active FROM rate_type WHERE code = $1) AND
			(SELECT count(*) = 2 FROM currency WHERE code IN ($2, $3) AND active)`, typ, from, to).Scan(&ok); err != nil {
			return err
		}
		if !ok {
			return core.ErrInactive.New(fmt.Sprintf("rate type %s, currencies %s and %s: each must exist and be active", typ, from, to))
		}
		var id string
		var same bool
		err := tx.QueryRow(ctx, `SELECT id::text, rate = $5::numeric AND source = $6 FROM exchange_rate
			WHERE rate_type = $1 AND from_currency = $2 AND to_currency = $3 AND rate_date = $4::date FOR UPDATE`,
			typ, from, to, date, in.Rate, in.Source).Scan(&id, &same)
		switch {
		case err == nil && same:
			out, err = core.GetRate(ctx, tx, id)
			return err
		case err == nil:
			if _, err := tx.Exec(ctx, `UPDATE exchange_rate SET rate = $2::numeric, source = $3, version = version + 1,
				updated_at = now() WHERE id = $1`, id, in.Rate, in.Source); err != nil {
				return err
			}
		case errors.Is(err, pgx.ErrNoRows):
			id = ids.New().String()
			if _, err := tx.Exec(ctx, `INSERT INTO exchange_rate (id, rate_type, rate_date, from_currency, to_currency,
				rate, source) VALUES ($1, $2, $3::date, $4, $5, $6::numeric, $7)`, id, typ, date, from, to, in.Rate,
				in.Source); err != nil {
				return err
			}
		default:
			return err
		}
		if out, err = core.GetRate(ctx, tx, id); err != nil {
			return err
		}
		if err := core.Emit(m, ctx, tx, core.RatePublished, out.ID, out.Version, out); err != nil {
			return err
		}
		return core.Change(m, ctx, tx, actor, core.OpMaintain, core.Rates, out.ID, out)
	})
	if err != nil {
		return err
	}
	return c.JSON(200, out)
}

func EffectiveHTTP(m *core.Base, c *httpx.Context) error {
	ctx, _, err := core.Check(c, core.OpRateRead)
	if err != nil {
		return err
	}
	typ, date, from, to := c.Query("rate_type"), c.Query("date"), c.Query("from_currency"), c.Query("to_currency")
	if err := core.CheckPair(typ, date, from, to); err != nil {
		return err
	}
	r, err := Effective(m, ctx, typ, date, from, to)
	if err != nil {
		return err
	}
	return c.JSON(200, r)
}

// Effective answers the effective rate for both surfaces.
func Effective(m *core.Base, ctx context.Context, typ, date, from, to string) (core.Rate, error) {
	var r core.Rate
	err := m.RT.DB.Tx(ctx, func(ctx context.Context, tx db.Tx) error {
		var err error
		r, err = core.Effective(ctx, tx, typ, date, from, to)
		return err
	})
	if errors.Is(err, core.ErrNone) {
		return r, errs.New(errs.ReasonNotFound, "no "+typ+" rate for "+from+"/"+to+" on or before "+date)
	}
	return r, err
}

var rateTypeList = list.Spec{Operation: core.OpRead, Alias: "t", IDColumn: "code", IDKind: list.Text}

var rateList = list.Spec{Operation: core.OpRateRead, Alias: "r", IDColumn: "id", IDKind: list.UUID,
	Columns: []list.Column{
		{Field: "rate_date", Column: "rate_date", Kind: list.Date, Sortable: true, Filterable: true},
		{Field: "rate_type", Column: "rate_type", Kind: list.Text, Filterable: true},
		{Field: "from_currency", Column: "from_currency", Kind: list.Text, Filterable: true},
		{Field: "to_currency", Column: "to_currency", Kind: list.Text, Filterable: true}}}

func ListRateTypes(m *core.Base, c *httpx.Context) error {
	ctx, q, err := core.Build(c, rateTypeList, list.Request{})
	if err != nil {
		return err
	}
	out, next, err := list.Page(ctx, m.RT.DB, q, "rate_type", core.RateTypeCols, core.ScanRateType,
		func(v core.RateType) ([]string, string) { return nil, v.Code })
	if err != nil {
		return err
	}
	return c.JSON(200, map[string]any{"rate_types": out, "next_cursor": next})
}

func ListRates(m *core.Base, c *httpx.Context) error {
	var f []list.Filter
	f = append(f, core.Eq("rate_type", c.Query("rate_type"))...)
	f = append(f, core.Eq("from_currency", c.Query("from_currency"))...)
	f = append(f, core.Eq("to_currency", c.Query("to_currency"))...)
	if v := c.Query("date_from"); v != "" {
		f = append(f, list.Filter{Field: "rate_date", Op: "gte", Values: []string{v}})
	}
	if v := c.Query("date_to"); v != "" {
		f = append(f, list.Filter{Field: "rate_date", Op: "lte", Values: []string{v}})
	}
	ctx, q, err := core.Build(c, rateList, list.Request{Filters: f, Sort: []list.Order{{Field: "rate_date", Desc: true}}})
	if err != nil {
		return err
	}
	out, next, err := list.Page(ctx, m.RT.DB, q, "exchange_rate", core.RateCols, core.ScanRate,
		func(v core.Rate) ([]string, string) { return []string{v.RateDate}, v.ID })
	if err != nil {
		return err
	}
	return c.JSON(200, map[string]any{"rates": out, "next_cursor": next})
}
