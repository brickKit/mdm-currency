package currency

import (
	"context"

	sdkauthz "github.com/brickKit/be-sdk-go/v3/authz"
	"github.com/brickKit/be-sdk-go/v3/httpx"
	"github.com/brickKit/be-sdk-go/v3/list"
)

// The lists of the user surface (protocol 14: cursor pages, a stable order ending in the record's key, filters and
// the keyword declared here). Every field is ordinary, so a row is either listed whole or not at all.
var (
	currencyList = list.Spec{Operation: OpRead, Alias: "c", IDColumn: "code", IDKind: list.Text,
		Columns: []list.Column{{Field: "active", Column: "active", Kind: list.Boolean, Filterable: true}},
		Search:  []list.SearchColumn{{Name: "names", Column: "search", Fields: []string{"code", "name", "names"}}}}
	rateTypeList = list.Spec{Operation: OpRead, Alias: "t", IDColumn: "code", IDKind: list.Text}
	rateList     = list.Spec{Operation: OpRateRead, Alias: "r", IDColumn: "id", IDKind: list.UUID,
		Columns: []list.Column{
			{Field: "rate_date", Column: "rate_date", Kind: list.Date, Sortable: true, Filterable: true},
			{Field: "rate_type", Column: "rate_type", Kind: list.Text, Filterable: true},
			{Field: "from_currency", Column: "from_currency", Kind: list.Text, Filterable: true},
			{Field: "to_currency", Column: "to_currency", Kind: list.Text, Filterable: true}}}
)

func build(c *httpx.Context, spec list.Spec, req list.Request) (context.Context, *list.Query, error) {
	limit, err := list.ParseLimit(c.Query("limit"))
	if err != nil {
		return nil, nil, err
	}
	req.Limit, req.Cursor = limit, c.Query("cursor")
	s, _ := sdkauthz.From(c.Ctx())
	return list.Build(c.Ctx(), s, spec, req)
}

func eq(field, v string) []list.Filter {
	if v == "" {
		return nil
	}
	return []list.Filter{{Field: field, Op: "eq", Values: []string{v}}}
}

func (m *Module) listCurrencies(c *httpx.Context) error {
	ctx, q, err := build(c, currencyList, list.Request{Filters: eq("active", c.Query("active")), Q: c.Query("q")})
	if err != nil {
		return err
	}
	out, next, err := list.Page(ctx, m.rt.DB, q, "currency", currencyCols, scanCurrency,
		func(v Currency) ([]string, string) { return nil, v.Code })
	if err != nil {
		return err
	}
	return c.JSON(200, map[string]any{"currencies": out, "next_cursor": next})
}

func (m *Module) listRateTypes(c *httpx.Context) error {
	ctx, q, err := build(c, rateTypeList, list.Request{})
	if err != nil {
		return err
	}
	out, next, err := list.Page(ctx, m.rt.DB, q, "rate_type", rateTypeCols, scanRateType,
		func(v RateType) ([]string, string) { return nil, v.Code })
	if err != nil {
		return err
	}
	return c.JSON(200, map[string]any{"rate_types": out, "next_cursor": next})
}

func (m *Module) listRates(c *httpx.Context) error {
	var f []list.Filter
	f = append(f, eq("rate_type", c.Query("rate_type"))...)
	f = append(f, eq("from_currency", c.Query("from_currency"))...)
	f = append(f, eq("to_currency", c.Query("to_currency"))...)
	if v := c.Query("date_from"); v != "" {
		f = append(f, list.Filter{Field: "rate_date", Op: "gte", Values: []string{v}})
	}
	if v := c.Query("date_to"); v != "" {
		f = append(f, list.Filter{Field: "rate_date", Op: "lte", Values: []string{v}})
	}
	ctx, q, err := build(c, rateList, list.Request{Filters: f, Sort: []list.Order{{Field: "rate_date", Desc: true}}})
	if err != nil {
		return err
	}
	out, next, err := list.Page(ctx, m.rt.DB, q, "exchange_rate", rateCols, scanRate,
		func(v Rate) ([]string, string) { return []string{v.RateDate}, v.ID })
	if err != nil {
		return err
	}
	return c.JSON(200, map[string]any{"rates": out, "next_cursor": next})
}
