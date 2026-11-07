package source

import (
	"context"
	"encoding/json"
	"fmt"

	core "github.com/brickKit/mdm-currency/v3/internal/core"
	rates "github.com/brickKit/mdm-currency/v3/internal/rates"

	"github.com/brickKit/be-sdk-go/v3/db"
	"github.com/brickKit/be-sdk-go/v3/errs"
	"github.com/brickKit/be-sdk-go/v3/events"
	currencyv3 "github.com/brickKit/mdm-currency/contract/v3/gen/go/be/mdm/currency/v3"
	"github.com/jackc/pgx/v5"
)

// aggregate is how the source methods read one kind of record.
type aggregate struct {
	table, key, cols string
	data             func(pgx.Row) (string, int64, any, error) // the ID, the version and the payload
}

var aggregates = map[currencyv3.Aggregate]aggregate{
	currencyv3.Aggregate_AGGREGATE_CURRENCY: {"currency", "code", core.CurrencyCols, func(r pgx.Row) (string, int64, any, error) {
		v, err := core.ScanCurrency(r)
		return v.Code, v.Version, v, err
	}},
	currencyv3.Aggregate_AGGREGATE_RATE_TYPE: {"rate_type", "code", core.RateTypeCols, func(r pgx.Row) (string, int64, any, error) {
		v, err := core.ScanRateType(r)
		return v.Code, v.Version, v, err
	}},
	currencyv3.Aggregate_AGGREGATE_EXCHANGE_RATE: {"exchange_rate", "id::text", core.RateCols, func(r pgx.Row) (string, int64, any, error) {
		v, err := core.ScanRate(r)
		return v.ID, v.Version, v, err
	}},
}

func stamp(m *core.Base, ctx context.Context, tx db.Tx) (*currencyv3.Stamp, error) {
	st, err := events.ReadStamp(ctx, tx, m.Instance, events.Stream{Name: core.Stream, Partitions: 4})
	if err != nil {
		return nil, err
	}
	return &currencyv3.Stamp{Deployment: m.Deployment, Instance: st.Instance, Epoch: st.Epoch.String(),
		Positions: st.Positions}, nil
}

// Snapshot pages through one aggregate by key, with the stamp of the same snapshot (protocol 02, S3).
func Snapshot(m *core.Base, ctx context.Context, in *currencyv3.SnapshotRequest) (*currencyv3.SnapshotResponse, error) {
	a, ok := aggregates[in.GetAggregate()]
	if !ok {
		return nil, core.Invalid("aggregate", "an aggregate of this component")
	}
	limit := int(in.GetLimit())
	if limit == 0 {
		limit = core.MaxBatch
	}
	if limit < 0 || limit > core.MaxBatch {
		return nil, errs.New(errs.ReasonBatchTooLarge, fmt.Sprintf("at most %d records a page", core.MaxBatch))
	}
	out := &currencyv3.SnapshotResponse{}
	err := m.RT.DB.Tx(ctx, func(ctx context.Context, tx db.Tx) error {
		var err error
		if out.Stamp, err = stamp(m, ctx, tx); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, fmt.Sprintf(`SELECT %s FROM %s WHERE %s > $1 ORDER BY %s LIMIT %d`,
			a.cols, a.table, a.key, a.key, limit+1), in.GetCursor())
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			if len(out.Records) == limit {
				out.NextCursor = out.Records[len(out.Records)-1].Id
				break
			}
			id, version, data, err := a.data(rows)
			if err != nil {
				return err
			}
			raw, err := json.Marshal(data)
			if err != nil {
				return err
			}
			out.Records = append(out.Records, &currencyv3.Record{Id: id, Version: version, Data: raw})
		}
		return rows.Err()
	}, db.Snapshot())
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Read answers by ID in the requested order: exists, or purged (this component deletes nothing, so no tombstones).
func Read(m *core.Base, ctx context.Context, in *currencyv3.ReadRequest) (*currencyv3.ReadResponse, error) {
	a, ok := aggregates[in.GetAggregate()]
	if !ok {
		return nil, core.Invalid("aggregate", "an aggregate of this component")
	}
	if len(in.GetIds()) > core.MaxBatch {
		return nil, errs.New(errs.ReasonBatchTooLarge, fmt.Sprintf("at most %d IDs a call", core.MaxBatch))
	}
	found := map[string]*currencyv3.ReadResult{}
	out := &currencyv3.ReadResponse{}
	err := m.RT.DB.Tx(ctx, func(ctx context.Context, tx db.Tx) error {
		var err error
		if out.Stamp, err = stamp(m, ctx, tx); err != nil {
			return err
		}
		if len(in.GetIds()) == 0 {
			return nil
		}
		rows, err := tx.Query(ctx, fmt.Sprintf(`SELECT %s FROM %s WHERE %s = ANY($1::text[])`, a.cols, a.table, a.key),
			in.GetIds())
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			id, version, data, err := a.data(rows)
			if err != nil {
				return err
			}
			raw, err := json.Marshal(data)
			if err != nil {
				return err
			}
			found[id] = &currencyv3.ReadResult{Id: id, State: currencyv3.ReadState_READ_STATE_EXISTS, Version: version, Data: raw}
		}
		return rows.Err()
	}, db.Snapshot())
	if err != nil {
		return nil, err
	}
	for _, id := range in.GetIds() {
		r, ok := found[id]
		if !ok {
			r = &currencyv3.ReadResult{Id: id, State: currencyv3.ReadState_READ_STATE_PURGED}
		}
		out.Results = append(out.Results, r)
	}
	return out, nil
}

// EffectiveRate is RateService.EffectiveRate (the same answer as GET /api/currency/rates/effective).
func EffectiveRate(m *core.Base, ctx context.Context, in *currencyv3.EffectiveRateRequest) (*currencyv3.EffectiveRateResponse, error) {
	if err := core.CheckPair(in.GetRateType(), in.GetDate(), in.GetFromCurrency(), in.GetToCurrency()); err != nil {
		return nil, err
	}
	r, err := rates.Effective(m, ctx, in.GetRateType(), in.GetDate(), in.GetFromCurrency(), in.GetToCurrency())
	if err != nil {
		return nil, err
	}
	return &currencyv3.EffectiveRateResponse{Id: r.ID, Version: r.Version, Rate: r.Rate, RateDate: r.RateDate,
		RateType: r.RateType, FromCurrency: r.FromCurrency, ToCurrency: r.ToCurrency}, nil
}
