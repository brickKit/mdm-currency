package currency

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/brickKit/be-sdk-go/v3/db"
	"github.com/brickKit/be-sdk-go/v3/errs"
	"github.com/brickKit/be-sdk-go/v3/events"
	"github.com/brickKit/be-sdk-go/v3/search"
	"github.com/brickKit/be-sdk-go/v3/work"
	currencyv3 "github.com/brickKit/mdm-currency/contract/v3/gen/go/be/mdm/currency/v3"
	"github.com/jackc/pgx/v5"
)

// aggregate is how the source methods read one kind of record.
type aggregate struct {
	table, key, cols string
	data             func(pgx.Row) (string, int64, any, error) // the ID, the version and the payload
}

var aggregates = map[currencyv3.Aggregate]aggregate{
	currencyv3.Aggregate_AGGREGATE_CURRENCY: {"currency", "code", currencyCols, func(r pgx.Row) (string, int64, any, error) {
		v, err := scanCurrency(r)
		return v.Code, v.Version, v, err
	}},
	currencyv3.Aggregate_AGGREGATE_RATE_TYPE: {"rate_type", "code", rateTypeCols, func(r pgx.Row) (string, int64, any, error) {
		v, err := scanRateType(r)
		return v.Code, v.Version, v, err
	}},
	currencyv3.Aggregate_AGGREGATE_EXCHANGE_RATE: {"exchange_rate", "id::text", rateCols, func(r pgx.Row) (string, int64, any, error) {
		v, err := scanRate(r)
		return v.ID, v.Version, v, err
	}},
}

func (m *Module) stamp(ctx context.Context, tx db.Tx) (*currencyv3.Stamp, error) {
	st, err := events.ReadStamp(ctx, tx, m.instance, events.Stream{Name: Stream, Partitions: 4})
	if err != nil {
		return nil, err
	}
	return &currencyv3.Stamp{Deployment: m.deployment, Instance: st.Instance, Epoch: st.Epoch.String(),
		Positions: st.Positions}, nil
}

// Snapshot pages through one aggregate by key, with the stamp of the same snapshot (protocol 02, S3).
func (m *Module) Snapshot(ctx context.Context, in *currencyv3.SnapshotRequest) (*currencyv3.SnapshotResponse, error) {
	a, ok := aggregates[in.GetAggregate()]
	if !ok {
		return nil, invalid("aggregate", "an aggregate of this component")
	}
	limit := int(in.GetLimit())
	if limit == 0 {
		limit = MaxBatch
	}
	if limit < 0 || limit > MaxBatch {
		return nil, errs.New(errs.ReasonBatchTooLarge, fmt.Sprintf("at most %d records a page", MaxBatch))
	}
	out := &currencyv3.SnapshotResponse{}
	err := m.rt.DB.Tx(ctx, func(ctx context.Context, tx db.Tx) error {
		var err error
		if out.Stamp, err = m.stamp(ctx, tx); err != nil {
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
func (m *Module) Read(ctx context.Context, in *currencyv3.ReadRequest) (*currencyv3.ReadResponse, error) {
	a, ok := aggregates[in.GetAggregate()]
	if !ok {
		return nil, invalid("aggregate", "an aggregate of this component")
	}
	if len(in.GetIds()) > MaxBatch {
		return nil, errs.New(errs.ReasonBatchTooLarge, fmt.Sprintf("at most %d IDs a call", MaxBatch))
	}
	found := map[string]*currencyv3.ReadResult{}
	out := &currencyv3.ReadResponse{}
	err := m.rt.DB.Tx(ctx, func(ctx context.Context, tx db.Tx) error {
		var err error
		if out.Stamp, err = m.stamp(ctx, tx); err != nil {
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
func (m *Module) EffectiveRate(ctx context.Context, in *currencyv3.EffectiveRateRequest) (*currencyv3.EffectiveRateResponse, error) {
	if err := checkPair(in.GetRateType(), in.GetDate(), in.GetFromCurrency(), in.GetToCurrency()); err != nil {
		return nil, err
	}
	r, err := m.effective(ctx, in.GetRateType(), in.GetDate(), in.GetFromCurrency(), in.GetToCurrency())
	if err != nil {
		return nil, err
	}
	return &currencyv3.EffectiveRateResponse{Id: r.ID, Version: r.Version, Rate: r.Rate, RateDate: r.RateDate,
		RateType: r.RateType, FromCurrency: r.FromCurrency, ToCurrency: r.ToCurrency}, nil
}

// Jobs declares the search backfill: currencies written by a migration (the ISO table) have no search column yet,
// and a newer normalization version of the SDK rewrites the old ones (protocol 14).
func (m *Module) Jobs(j *work.Jobs) error {
	j.Add(work.Job{Name: "mdm.currency.search_backfill", Kind: work.Singleton, Effect: work.Local, Interval: 10 * time.Minute,
		MaxAttempts: work.Unlimited, Handle: func(ctx context.Context, _ *work.Run) error { return m.backfill(ctx) }})
	return nil
}

func (m *Module) backfill(ctx context.Context) error {
	for {
		n := 0
		err := m.rt.DB.BackgroundTx(ctx, func(ctx context.Context, tx db.Tx) error {
			rows, err := tx.Query(ctx, `SELECT `+currencyCols+` FROM currency
				WHERE search_version IS DISTINCT FROM $1 ORDER BY code LIMIT 100 FOR UPDATE SKIP LOCKED`, search.Version)
			if err != nil {
				return err
			}
			var due []Currency
			for rows.Next() {
				c, err := scanCurrency(rows)
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
