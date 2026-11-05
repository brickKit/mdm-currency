package currency

import (
	"context"
	"strings"

	"github.com/brickKit/be-sdk-go/v3/db"
	"github.com/brickKit/be-sdk-go/v3/recovery"
)

// RecoveryChecks registers what a full recovery asks this member (be-protocol chapter 14): whether the objects this
// member audits are still here, at which version, and the checks it reports.
func (m *Module) RecoveryChecks(c *recovery.Checks) error {
	currencies := recovery.TablePresence("currency", "code", "version")
	rateTypes := recovery.TablePresence("rate_type", "code", "version")
	// a rate type is audited under Currencies as "rate_type:<code>" (module.go)
	c.Presence(Currencies, func(ctx context.Context, tx db.Tx, ids []string) (map[string]int64, error) {
		var codes, types []string
		for _, id := range ids {
			if code, ok := strings.CutPrefix(id, "rate_type:"); ok {
				types = append(types, code)
			} else {
				codes = append(codes, id)
			}
		}
		out, err := currencies(ctx, tx, codes)
		if err != nil {
			return nil, err
		}
		found, err := rateTypes(ctx, tx, types)
		for code, v := range found {
			out["rate_type:"+code] = v
		}
		return out, err
	})
	c.Presence(Rates, recovery.TablePresence("exchange_rate", "id", "version"))
	return nil
}
