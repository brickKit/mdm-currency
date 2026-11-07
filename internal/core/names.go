package core

import (
	sdkauthz "github.com/brickKit/be-sdk-go/v3/authz"
)

// ID is the component's ID.
const ID = "mdm/currency"

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
	// OpResend is the redelivery of this component's stream (be-protocol P6.38: every component with streams serves
	// it; any component of the deployment may call it).
	OpResend = "mdm.currency.system.resend"
	// OpRecover is a full recovery's checks (be-protocol chapter 14: the recovery tool only).
	OpRecover = "mdm.currency.system.recover"
	OpSystem  = "mdm.currency.system.read"
)

// MaxBatch is the most IDs a Read takes and records a Snapshot page gives (decision 0304).
const MaxBatch = 500

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
			{Key: OpResend, ResourceType: System, Purpose: "read", RiskClass: "low"},
			{Key: OpRecover, ResourceType: System, Purpose: "read", RiskClass: "low"},
		}}
}
