package currency

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/brickKit/be-sdk-go/v3/audit"
	sdkauthz "github.com/brickKit/be-sdk-go/v3/authz"
	"github.com/brickKit/be-sdk-go/v3/db"
	"github.com/brickKit/be-sdk-go/v3/errs"
	"github.com/brickKit/be-sdk-go/v3/grpcx"
	"github.com/brickKit/be-sdk-go/v3/httpx"
	"github.com/brickKit/be-sdk-go/v3/identity"
	"github.com/brickKit/be-sdk-go/v3/ids"
	"github.com/brickKit/be-sdk-go/v3/money"
	"github.com/brickKit/be-sdk-go/v3/runtime"
	"github.com/brickKit/be-sdk-go/v3/search"
	currencyv3 "github.com/brickKit/mdm-currency/contract/v3/gen/go/be/mdm/currency/v3"
	"github.com/jackc/pgx/v5"
)

// The component's own reasons (the SDK's catalog has the shared ones).
var (
	errVersion  = errs.Define(ID, "VERSION_CONFLICT", errs.Aborted, 409, errs.RetryNo, "The record changed since that version")
	errInactive = errs.Define(ID, "INACTIVE", errs.FailedPrecondition, 409, errs.RetryNo,
		"A rate needs an active rate type and active currencies")
)

var (
	isoCode  = regexp.MustCompile(`^[A-Z]{3}$`)
	typeCode = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)
	isoDate  = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`)
)

// Module is the currency component.
type Module struct {
	currencyv3.UnimplementedSourceServiceServer
	currencyv3.UnimplementedRateServiceServer
	rt         *runtime.Runtime
	deployment string
	instance   string
}

// New builds the module from its runtime.
func New(rt *runtime.Runtime) (*Module, error) {
	return &Module{rt: rt, deployment: rt.Config.String("DEPLOYMENT_ID"),
		instance: rt.Config.String("IMPLEMENTATION_INSTANCE")}, nil
}

// Routes registers the user surface (contract/openapi.yaml).
func (m *Module) Routes(r *httpx.Routes) error {
	r.Add(httpx.Route{Method: "GET", Path: "/api/currency/currencies", Operation: OpRead, Handle: m.listCurrencies})
	r.Add(httpx.Route{Method: "GET", Path: "/api/currency/currencies/:code", Operation: OpRead, Handle: m.getCurrency})
	r.Add(httpx.Route{Method: "PATCH", Path: "/api/currency/currencies/:code", Operation: OpManage, Handle: m.updateCurrency})
	r.Add(httpx.Route{Method: "GET", Path: "/api/currency/rate-types", Operation: OpRead, Handle: m.listRateTypes})
	r.Add(httpx.Route{Method: "PUT", Path: "/api/currency/rate-types/:code", Operation: OpManage, Handle: m.putRateType})
	r.Add(httpx.Route{Method: "GET", Path: "/api/currency/rates", Operation: OpRateRead, Handle: m.listRates})
	r.Add(httpx.Route{Method: "GET", Path: "/api/currency/rates/effective", Operation: OpRateRead, Handle: m.effectiveHTTP})
	r.Add(httpx.Route{Method: "PUT", Path: "/api/currency/rates/:rate_type/:date/:from_currency/:to_currency", Operation: OpMaintain, Handle: m.putRate})
	return nil
}

// Services registers the system surface: every component of the deployment reads master data (boundary cards: master
// data is read by all business components).
func (m *Module) Services(s *grpcx.Services) error {
	s.Register(&currencyv3.SourceService_ServiceDesc, m, map[string]grpcx.Method{
		"Snapshot": {Operation: OpSystem, Callers: []string{grpcx.AnyComponent}},
		"Read":     {Operation: OpSystem, Callers: []string{grpcx.AnyComponent}}})
	s.Register(&currencyv3.RateService_ServiceDesc, m, map[string]grpcx.Method{
		"EffectiveRate": {Operation: OpSystem, Callers: []string{grpcx.AnyComponent}}})
	return nil
}

// check admits the request's subject to op and returns the context to work in and the acting principal.
func check(c *httpx.Context, op string) (context.Context, string, error) {
	s, _ := sdkauthz.From(c.Ctx())
	ctx, _, err := s.Check(c.Ctx(), op, sdkauthz.Facts{})
	if err != nil {
		return ctx, "", err
	}
	u, _ := identity.UserFrom(ctx)
	return ctx, u.Subject, nil
}

func invalid(field, why string) error {
	return errs.New(errs.ReasonRequestInvalid, why).WithViolations(errs.Violation{Field: field, Reason: "INVALID", Description: why})
}

func notFound(what string) error { return errs.New(errs.ReasonNotFound, what+" not found") }

// change records the business change with the record's new state (protocol 03), in the transaction that made it.
func (m *Module) change(ctx context.Context, tx db.Tx, actor, op, typ, id string, after any) error {
	_, err := m.rt.Audit.Write(ctx, tx, audit.Record{Category: audit.BusinessChange, Operation: op, Outcome: audit.Succeeded,
		Actor: actor, Target: map[string]any{"type": typ, "id": id}, Changes: map[string]any{"after": after}})
	return err
}

func (m *Module) getCurrency(c *httpx.Context) error {
	ctx, _, err := check(c, OpRead)
	if err != nil {
		return err
	}
	var cur Currency
	err = m.rt.DB.Tx(ctx, func(ctx context.Context, tx db.Tx) error {
		cur, err = getCurrency(ctx, tx, c.Param("code"), false)
		return err
	})
	if errors.Is(err, errNone) {
		return notFound("currency")
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

// updateCurrency changes what a deployment may decide about an ISO currency; the ISO data itself stays.
func (m *Module) updateCurrency(c *httpx.Context) error {
	ctx, actor, err := check(c, OpManage)
	if err != nil {
		return err
	}
	var in currencyPatch
	if err := c.BindJSON(&in); err != nil {
		return err
	}
	if in.Rounding != nil && *in.Rounding != string(money.HalfAwayFromZero) && *in.Rounding != string(money.HalfEven) {
		return invalid("rounding", "rounding is half_away_from_zero or half_even")
	}
	var cash *string
	setCash := len(in.CashUnit) > 0
	if setCash && string(in.CashUnit) != "null" {
		var s string
		if json.Unmarshal(in.CashUnit, &s) != nil || !positive(s, 4) {
			return invalid("cash_unit", "cash_unit is a positive decimal string with at most 4 decimal places, or null")
		}
		cash = &s
	}
	if in.Names != nil {
		if err := checkNames(*in.Names); err != nil {
			return err
		}
	}
	var out Currency
	err = m.rt.DB.Tx(ctx, func(ctx context.Context, tx db.Tx) error {
		cur, err := getCurrency(ctx, tx, c.Param("code"), true)
		if err != nil {
			return err
		}
		if in.Version != nil && *in.Version != cur.Version {
			return errVersion.New(fmt.Sprintf("the currency is at version %d", cur.Version))
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
		if out, err = getCurrency(ctx, tx, cur.Code, false); err != nil {
			return err
		}
		if err := m.emit(ctx, tx, CurrencyChanged, out.Code, out.Version, out); err != nil {
			return err
		}
		return m.change(ctx, tx, actor, OpManage, Currencies, out.Code, out)
	})
	if errors.Is(err, errNone) {
		return notFound("currency")
	}
	if err != nil {
		return err
	}
	return c.JSON(200, out)
}

func sameCurrency(a, b Currency) bool {
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	return string(ja) == string(jb)
}

// currencySearch is the currency's search column: code and every name (protocol 14's list keyword).
func currencySearch(c Currency) string {
	vals := []string{c.Code, c.Name}
	for _, n := range c.Names {
		vals = append(vals, n)
	}
	return search.Text(vals...)
}

// checkNames refuses translations without a language tag or a name.
func checkNames(names map[string]string) error {
	for tag, n := range names {
		if tag == "" || len(tag) > 35 || strings.TrimSpace(n) == "" {
			return invalid("names", "names maps a language tag to a non-empty name")
		}
	}
	return nil
}

type rateTypeIn struct {
	Name   string            `json:"name"`
	Names  map[string]string `json:"names"`
	Active *bool             `json:"active"`
}

// putRateType creates the rate type or changes it; the same content again changes nothing.
func (m *Module) putRateType(c *httpx.Context) error {
	ctx, actor, err := check(c, OpManage)
	if err != nil {
		return err
	}
	code := c.Param("code")
	if !typeCode.MatchString(code) {
		return invalid("code", "a rate type's code is lower case letters, digits and _, at most 32, starting with a letter")
	}
	var in rateTypeIn
	if err := c.BindJSON(&in); err != nil {
		return err
	}
	if strings.TrimSpace(in.Name) == "" {
		return invalid("name", "name is required")
	}
	if in.Names == nil {
		in.Names = map[string]string{}
	}
	if err := checkNames(in.Names); err != nil {
		return err
	}
	active := in.Active == nil || *in.Active
	var out RateType
	err = m.rt.DB.Tx(ctx, func(ctx context.Context, tx db.Tx) error {
		names, _ := json.Marshal(in.Names)
		cur, err := getRateType(ctx, tx, code, true)
		switch {
		case errors.Is(err, errNone):
			if _, err := tx.Exec(ctx, `INSERT INTO rate_type (code, active, name, names) VALUES ($1, $2, $3, $4)
				ON CONFLICT (code) DO NOTHING`, code, active, in.Name, names); err != nil {
				return err
			}
		case err != nil:
			return err
		case cur.Active == active && cur.Name == in.Name && sameNames(cur.Names, in.Names):
			out = cur
			return nil
		default:
			if _, err := tx.Exec(ctx, `UPDATE rate_type SET active = $2, name = $3, names = $4, version = version + 1,
				updated_at = now() WHERE code = $1`, code, active, in.Name, names); err != nil {
				return err
			}
		}
		if out, err = getRateType(ctx, tx, code, false); err != nil {
			return err
		}
		if err := m.emit(ctx, tx, RateTypeChanged, out.Code, out.Version, out); err != nil {
			return err
		}
		return m.change(ctx, tx, actor, OpManage, Currencies, "rate_type:"+out.Code, out)
	})
	if err != nil {
		return err
	}
	return c.JSON(200, out)
}

func sameNames(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

type rateIn struct {
	Rate   string `json:"rate"`
	Source string `json:"source"`
}

// putRate enters or corrects the one rate of a type, date and pair (the boundary card's invariant): a correction is
// a new version of the same record, published again; the same rate again changes nothing.
func (m *Module) putRate(c *httpx.Context) error {
	ctx, actor, err := check(c, OpMaintain)
	if err != nil {
		return err
	}
	typ, date, from, to := c.Param("rate_type"), c.Param("date"), c.Param("from_currency"), c.Param("to_currency")
	if err := checkPair(typ, date, from, to); err != nil {
		return err
	}
	var in rateIn
	if err := c.BindJSON(&in); err != nil {
		return err
	}
	if !positive(in.Rate, 10) {
		return invalid("rate", "rate is a positive decimal string with at most 10 decimal places")
	}
	if in.Source == "" {
		in.Source = "manual"
	}
	var out Rate
	err = m.rt.DB.Tx(ctx, func(ctx context.Context, tx db.Tx) error {
		var ok bool
		if err := tx.QueryRow(ctx, `SELECT (SELECT active FROM rate_type WHERE code = $1) AND
			(SELECT count(*) = 2 FROM currency WHERE code IN ($2, $3) AND active)`, typ, from, to).Scan(&ok); err != nil {
			return err
		}
		if !ok {
			return errInactive.New(fmt.Sprintf("rate type %s, currencies %s and %s: each must exist and be active", typ, from, to))
		}
		var id string
		var same bool
		err := tx.QueryRow(ctx, `SELECT id::text, rate = $5::numeric AND source = $6 FROM exchange_rate
			WHERE rate_type = $1 AND from_currency = $2 AND to_currency = $3 AND rate_date = $4::date FOR UPDATE`,
			typ, from, to, date, in.Rate, in.Source).Scan(&id, &same)
		switch {
		case err == nil && same:
			out, err = getRate(ctx, tx, id)
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
		if out, err = getRate(ctx, tx, id); err != nil {
			return err
		}
		if err := m.emit(ctx, tx, RatePublished, out.ID, out.Version, out); err != nil {
			return err
		}
		return m.change(ctx, tx, actor, OpMaintain, Rates, out.ID, out)
	})
	if err != nil {
		return err
	}
	return c.JSON(200, out)
}

func (m *Module) effectiveHTTP(c *httpx.Context) error {
	ctx, _, err := check(c, OpRateRead)
	if err != nil {
		return err
	}
	typ, date, from, to := c.Query("rate_type"), c.Query("date"), c.Query("from_currency"), c.Query("to_currency")
	if err := checkPair(typ, date, from, to); err != nil {
		return err
	}
	r, err := m.effective(ctx, typ, date, from, to)
	if err != nil {
		return err
	}
	return c.JSON(200, r)
}

// effective answers the effective rate for both surfaces.
func (m *Module) effective(ctx context.Context, typ, date, from, to string) (Rate, error) {
	var r Rate
	err := m.rt.DB.Tx(ctx, func(ctx context.Context, tx db.Tx) error {
		var err error
		r, err = effective(ctx, tx, typ, date, from, to)
		return err
	})
	if errors.Is(err, errNone) {
		return r, errs.New(errs.ReasonNotFound, "no "+typ+" rate for "+from+"/"+to+" on or before "+date)
	}
	return r, err
}

func checkPair(typ, date, from, to string) error {
	switch {
	case !typeCode.MatchString(typ):
		return invalid("rate_type", "rate_type is a rate type's code")
	case !isoDate.MatchString(date):
		return invalid("date", "date is YYYY-MM-DD")
	case !isoCode.MatchString(from) || !isoCode.MatchString(to):
		return invalid("currency", "currencies are ISO 4217 codes")
	case from == to:
		return invalid("currency", "a rate is between two different currencies")
	}
	return nil
}

// positive reports a positive decimal string with at most places decimal places.
func positive(s string, places int) bool {
	d, err := money.Parse(s)
	if err != nil || d.Sign() <= 0 {
		return false
	}
	i := strings.IndexByte(s, '.')
	return i < 0 || len(s)-i-1 <= places
}
