// Package currency is mdm/currency (boundary card mdm/currency): currencies (ISO 4217, with their minor unit,
// rounding and cash unit), rate types and daily exchange rates, kept by people over the user surface, read by
// components over the system surface (the source methods of protocol 02 and the effective rate), and published as
// full-state events. Step 6 of the representative deployment experiment: no rate source adapter yet (D75).
package currency

import (
	"embed"
	"io/fs"

	sdkauthz "github.com/brickKit/be-sdk-go/v3/authz"
	"github.com/brickKit/be-sdk-go/v3/component"
	"github.com/brickKit/be-sdk-go/v3/events"
	"github.com/brickKit/be-sdk-go/v3/runtime"
)

// Identity of the component.
const (
	ID       = "mdm/currency"
	Version  = "3.0.5"
	GRPCPort = 9226
)

// The stream and its event types (contract/events).
const (
	Stream          = "mdm.currency"
	CurrencyChanged = "mdm.currency.changed.v1"
	RateTypeChanged = "mdm.rate_type.changed.v1"
	RatePublished   = "mdm.exchange_rate.published.v1"
)

// Resources and operations (contract/openapi.yaml names each route's operation).
const (
	Currencies = "mdm.currency"
	OpRead     = "mdm.currency.read"
	OpManage   = "mdm.currency.manage"
	Rates      = "mdm.exchange_rate"
	OpRateRead = "mdm.exchange_rate.read"
	OpMaintain = "mdm.exchange_rate.maintain"
	System     = "mdm.currency.system"
	OpSystem   = "mdm.currency.system.read"
)

// MaxBatch is the most IDs a Read takes and records a Snapshot page gives (decision 0304).
const MaxBatch = 500

//go:embed migrations/*.sql
var migrationFiles embed.FS

func migrations() fs.FS {
	sub, _ := fs.Sub(migrationFiles, "migrations")
	return sub
}

// Definitions are the component's resources and operations. No dimensions in step 6, declared as such (an empty
// set, not a missing one): every reader with the operation sees every currency and rate.
func Definitions() *sdkauthz.Definitions {
	none := map[string]sdkauthz.Dimension{}
	fields := func(names ...string) map[string]sdkauthz.Field {
		out := map[string]sdkauthz.Field{}
		for _, n := range names {
			out[n] = sdkauthz.Field{Class: sdkauthz.Ordinary}
		}
		return out
	}
	return &sdkauthz.Definitions{Component: ID,
		Resources: []sdkauthz.Resource{
			{Type: Currencies, ViewOperation: OpRead, Dimensions: none, Fields: fields("code", "numeric_code", "minor_unit",
				"rounding", "cash_unit", "active", "name", "names", "version", "updated_at")},
			{Type: Rates, ViewOperation: OpRateRead, Dimensions: none, Fields: fields("id", "rate_type", "rate_date",
				"from_currency", "to_currency", "rate", "source", "version", "updated_at")},
			{Type: System, ViewOperation: OpSystem, Dimensions: none, Fields: map[string]sdkauthz.Field{}},
		},
		Operations: []sdkauthz.Operation{
			{Key: OpRead, ResourceType: Currencies, Purpose: "read", RiskClass: "low"},
			{Key: OpManage, ResourceType: Currencies, Purpose: "manage", RiskClass: "low",
				SideEffects: []string{"business_write"}, Audit: true},
			{Key: OpRateRead, ResourceType: Rates, Purpose: "read", RiskClass: "low"},
			{Key: OpMaintain, ResourceType: Rates, Purpose: "action", RiskClass: "low",
				SideEffects: []string{"business_write"}, Audit: true},
			{Key: OpSystem, ResourceType: System, Purpose: "read", RiskClass: "low"},
		}}
}

// Definition is the component as the launcher and the shells take it.
func Definition() component.Definition {
	return component.Definition{ID: ID, Version: Version, Migrations: migrations(), GRPCPort: GRPCPort,
		Streams: []events.Stream{{Name: Stream, Partitions: 4}}, Authz: Definitions(),
		New: func(rt *runtime.Runtime) (component.Module, error) { return New(rt) }}
}
