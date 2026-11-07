//go:build integration

package itest

import (
	core "github.com/brickKit/mdm-currency/v3/internal/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// outbox: each change is a full-state event in the outbox, in the same transaction, and audited.
func (fx *fixture) outbox() {
	t, ctx := fx.t, fx.ctx
	// each change is a full-state event in the outbox, in the same transaction
	var n int
	require.NoError(t, fx.admin.QueryRow(ctx, "SELECT count(*) FROM "+itSchema+".be_outbox WHERE record_id = $1 AND type = $2",
		fx.r1["id"], core.RatePublished).Scan(&n))
	assert.Equal(t, 2, n, "entered, corrected; the same rate again published nothing")
	require.NoError(t, fx.admin.QueryRow(ctx, "SELECT count(*) FROM "+itSchema+".be_audit_log").Scan(&n))
	assert.Positive(t, n, "the changes are audited")

}
