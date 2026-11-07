//go:build integration

package itest

import (
	"github.com/brickKit/be-sdk-go/v3/errs"
	currencyv3 "github.com/brickKit/mdm-currency/contract/v3/gen/go/be/mdm/currency/v3"
	core "github.com/brickKit/mdm-currency/v3/internal/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// source: the system surface over mTLS: Read (exists, purged), the stamp, Snapshot pages, a batch too large, the
// effective rate.
func (fx *fixture) source() {
	t, ctx := fx.t, fx.ctx
	// the system surface over mTLS
	tls := fx.w.TLS(t, "erp/sales")
	conn, err := grpc.NewClient(itSystem, grpc.WithTransportCredentials(credentials.NewTLS(tls.ClientConfig(core.ID))))
	require.NoError(t, err)
	defer conn.Close()
	src := currencyv3.NewSourceServiceClient(conn)
	read, err := src.Read(ctx, &currencyv3.ReadRequest{Aggregate: currencyv3.Aggregate_AGGREGATE_CURRENCY,
		Ids: []string{"CHF", "XXQ"}})
	require.NoError(t, err)
	require.Len(t, read.Results, 2)
	assert.Equal(t, currencyv3.ReadState_READ_STATE_EXISTS, read.Results[0].State)
	assert.Contains(t, string(read.Results[0].Data), `"cash_unit":"0.1"`)
	assert.Equal(t, currencyv3.ReadState_READ_STATE_PURGED, read.Results[1].State, "never existed: purged, not an error")
	assert.Equal(t, fx.env["IMPLEMENTATION_INSTANCE"], read.Stamp.Instance)
	assert.Len(t, read.Stamp.Positions, 4)
	assert.Regexp(t, `^[0-9]+\.[0-9]+$`, read.Stamp.Epoch)
	snap, err := src.Snapshot(ctx, &currencyv3.SnapshotRequest{Aggregate: currencyv3.Aggregate_AGGREGATE_CURRENCY, Limit: 100})
	require.NoError(t, err)
	assert.Len(t, snap.Records, 100)
	assert.NotEmpty(t, snap.NextCursor)
	_, err = src.Read(ctx, &currencyv3.ReadRequest{Aggregate: currencyv3.Aggregate_AGGREGATE_CURRENCY,
		Ids: make([]string, core.MaxBatch+1)})
	e, _, ok := errs.FromGRPC(err)
	require.True(t, ok, "%v", err)
	assert.Equal(t, errs.ReasonBatchTooLarge, e.Reason, "more than 500 IDs: refused, not cut")
	rate, err := currencyv3.NewRateServiceClient(conn).EffectiveRate(ctx, &currencyv3.EffectiveRateRequest{
		RateType: "spot", Date: "2026-10-30", FromCurrency: "EUR", ToCurrency: "CNY"})
	require.NoError(t, err)
	assert.Equal(t, "7.7", rate.Rate)
	assert.Equal(t, "2026-10-18", rate.RateDate)

}
