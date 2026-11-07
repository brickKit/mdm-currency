package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPositiveDecimals(t *testing.T) {
	for s, want := range map[string]bool{"7.6543210001": true, "1": true, "0.05": true, "0": false, "-1": false,
		"1.12345678901": false, "1e3": false, "": false, "01": false} {
		assert.Equal(t, want, Positive(s, 10), s)
	}
}
