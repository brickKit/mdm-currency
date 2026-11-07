package core

import (
	"context"
	"regexp"
	"strings"

	"github.com/brickKit/be-sdk-go/v3/audit"
	sdkauthz "github.com/brickKit/be-sdk-go/v3/authz"
	"github.com/brickKit/be-sdk-go/v3/db"
	"github.com/brickKit/be-sdk-go/v3/errs"
	"github.com/brickKit/be-sdk-go/v3/httpx"
	"github.com/brickKit/be-sdk-go/v3/identity"
	"github.com/brickKit/be-sdk-go/v3/list"
	"github.com/brickKit/be-sdk-go/v3/money"
)

// The component's own reasons (the SDK's catalog has the shared ones).
var (
	ErrVersion  = errs.Define(ID, "VERSION_CONFLICT", errs.Aborted, 409, errs.RetryNo, "The record changed since that version")
	ErrInactive = errs.Define(ID, "INACTIVE", errs.FailedPrecondition, 409, errs.RetryNo,
		"A rate needs an active rate type and active currencies")
)

var (
	isoCode  = regexp.MustCompile(`^[A-Z]{3}$`)
	TypeCode = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)
	isoDate  = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`)
)

// Check admits the request's subject to op and returns the context to work in and the acting principal.
func Check(c *httpx.Context, op string) (context.Context, string, error) {
	s, _ := sdkauthz.From(c.Ctx())
	ctx, _, err := s.Check(c.Ctx(), op, sdkauthz.Facts{})
	if err != nil {
		return ctx, "", err
	}
	u, _ := identity.UserFrom(ctx)
	return ctx, u.Subject, nil
}

func Invalid(field, why string) error {
	return errs.New(errs.ReasonRequestInvalid, why).WithViolations(errs.Violation{Field: field, Reason: "INVALID", Description: why})
}

func NotFound(what string) error { return errs.New(errs.ReasonNotFound, what+" not found") }

// Change records the business change with the record's new state (protocol 03), in the transaction that made it.
func Change(m *Base, ctx context.Context, tx db.Tx, actor, op, typ, id string, after any) error {
	_, err := m.RT.Audit.Write(ctx, tx, audit.Record{Category: audit.BusinessChange, Operation: op, Outcome: audit.Succeeded,
		Actor: actor, Target: audit.TargetOf(typ, id, after), Changes: map[string]any{"after": after}})
	return err
}

// CheckNames refuses translations without a language tag or a name.
func CheckNames(names map[string]string) error {
	for tag, n := range names {
		if tag == "" || len(tag) > 35 || strings.TrimSpace(n) == "" {
			return Invalid("names", "names maps a language tag to a non-empty name")
		}
	}
	return nil
}

func SameNames(a, b map[string]string) bool {
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

func CheckPair(typ, date, from, to string) error {
	switch {
	case !TypeCode.MatchString(typ):
		return Invalid("rate_type", "rate_type is a rate type's code")
	case !isoDate.MatchString(date):
		return Invalid("date", "date is YYYY-MM-DD")
	case !isoCode.MatchString(from) || !isoCode.MatchString(to):
		return Invalid("currency", "currencies are ISO 4217 codes")
	case from == to:
		return Invalid("currency", "a rate is between two different currencies")
	}
	return nil
}

// Positive reports a positive decimal string with at most places decimal places.
func Positive(s string, places int) bool {
	d, err := money.Parse(s)
	if err != nil || d.Sign() <= 0 {
		return false
	}
	i := strings.IndexByte(s, '.')
	return i < 0 || len(s)-i-1 <= places
}

func Build(c *httpx.Context, spec list.Spec, req list.Request) (context.Context, *list.Query, error) {
	limit, err := list.ParseLimit(c.Query("limit"))
	if err != nil {
		return nil, nil, err
	}
	req.Limit, req.Cursor = limit, c.Query("cursor")
	s, _ := sdkauthz.From(c.Ctx())
	return list.Build(c.Ctx(), s, spec, req)
}

func Eq(field, v string) []list.Filter {
	if v == "" {
		return nil
	}
	return []list.Filter{{Field: field, Op: "eq", Values: []string{v}}}
}
