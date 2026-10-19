package currency

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/brickKit/be-sdk-go/v3/canon"
	"github.com/brickKit/be-sdk-go/v3/db"
	"github.com/brickKit/be-sdk-go/v3/events"
	"github.com/jackc/pgx/v5"
)

// Currency is a currency's complete state: the payload of mdm.currency.changed.v1 and of its Read and Snapshot.
type Currency struct {
	Code        string            `json:"code"`
	NumericCode string            `json:"numeric_code"`
	MinorUnit   int               `json:"minor_unit"`
	Rounding    string            `json:"rounding"`
	CashUnit    *string           `json:"cash_unit"`
	Active      bool              `json:"active"`
	Name        string            `json:"name"`
	Names       map[string]string `json:"names"`
	Version     int64             `json:"version"`
	UpdatedAt   string            `json:"updated_at"`
}

// RateType is a rate type's complete state (mdm.rate_type.changed.v1).
type RateType struct {
	Code      string            `json:"code"`
	Active    bool              `json:"active"`
	Name      string            `json:"name"`
	Names     map[string]string `json:"names"`
	Version   int64             `json:"version"`
	UpdatedAt string            `json:"updated_at"`
}

// Rate is an exchange rate's complete state (mdm.exchange_rate.published.v1).
type Rate struct {
	ID           string `json:"id"`
	RateType     string `json:"rate_type"`
	RateDate     string `json:"rate_date"`
	FromCurrency string `json:"from_currency"`
	ToCurrency   string `json:"to_currency"`
	Rate         string `json:"rate"`
	Source       string `json:"source"`
	Version      int64  `json:"version"`
	UpdatedAt    string `json:"updated_at"`
}

// The columns each record is read with; trailing zeros of the numeric columns are dropped in SQL (the contract's
// decimal strings carry none: protocol 14, security 09a).
const (
	currencyCols = `code, numeric_code, minor_unit, rounding, trim_scale(cash_unit)::text, active, name, names, version,
		updated_at`
	rateTypeCols = `code, active, name, names, version, updated_at`
	rateCols     = `id::text, rate_type, rate_date::text, from_currency, to_currency, trim_scale(rate)::text, source, version,
		updated_at`
)

func scanCurrency(row pgx.Row) (Currency, error) {
	var c Currency
	var names []byte
	var at time.Time
	err := row.Scan(&c.Code, &c.NumericCode, &c.MinorUnit, &c.Rounding, &c.CashUnit, &c.Active, &c.Name, &names,
		&c.Version, &at)
	if err == nil {
		c.UpdatedAt, err = canon.Time(at), json.Unmarshal(names, &c.Names)
	}
	return c, err
}

func scanRateType(row pgx.Row) (RateType, error) {
	var t RateType
	var names []byte
	var at time.Time
	err := row.Scan(&t.Code, &t.Active, &t.Name, &names, &t.Version, &at)
	if err == nil {
		t.UpdatedAt, err = canon.Time(at), json.Unmarshal(names, &t.Names)
	}
	return t, err
}

func scanRate(row pgx.Row) (Rate, error) {
	var r Rate
	var at time.Time
	err := row.Scan(&r.ID, &r.RateType, &r.RateDate, &r.FromCurrency, &r.ToCurrency, &r.Rate, &r.Source, &r.Version, &at)
	r.UpdatedAt = canon.Time(at)
	return r, err
}

// errNone is a lookup that found no row.
var errNone = errors.New("no such record")

func one[T any](row pgx.Row, scan func(pgx.Row) (T, error)) (T, error) {
	v, err := scan(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, errNone
	}
	return v, err
}

func getCurrency(ctx context.Context, tx db.Tx, code string, lock bool) (Currency, error) {
	q := `SELECT ` + currencyCols + ` FROM currency WHERE code = $1`
	if lock {
		q += ` FOR UPDATE`
	}
	return one(tx.QueryRow(ctx, q, code), scanCurrency)
}

func getRateType(ctx context.Context, tx db.Tx, code string, lock bool) (RateType, error) {
	q := `SELECT ` + rateTypeCols + ` FROM rate_type WHERE code = $1`
	if lock {
		q += ` FOR UPDATE`
	}
	return one(tx.QueryRow(ctx, q, code), scanRateType)
}

func getRate(ctx context.Context, tx db.Tx, id string) (Rate, error) {
	return one(tx.QueryRow(ctx, `SELECT `+rateCols+` FROM exchange_rate WHERE id = $1`, id), scanRate)
}

// effective is the rate of the type and pair on the latest date not after date.
func effective(ctx context.Context, tx db.Tx, rateType, date, from, to string) (Rate, error) {
	return one(tx.QueryRow(ctx, `SELECT `+rateCols+` FROM exchange_rate WHERE rate_type = $1 AND from_currency = $2
		AND to_currency = $3 AND rate_date <= $4::date ORDER BY rate_date DESC LIMIT 1`, rateType, from, to, date), scanRate)
}

// emit writes the record's full-state event into the outbox, in the transaction that changed it (protocol 02, S2).
func (m *Module) emit(ctx context.Context, tx db.Tx, typ, id string, version int64, data any) error {
	_, err := m.rt.Events.Emit(ctx, tx, Stream, events.Event{Type: typ, RecordID: id, Version: version,
		Projection: events.Full, Data: data})
	return err
}
