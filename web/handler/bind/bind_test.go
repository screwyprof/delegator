package bind_test

import (
	"encoding/base64"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/screwyprof/delegator/web/api"
	"github.com/screwyprof/delegator/web/handler/bind"
	"github.com/screwyprof/delegator/web/tezos"
)

// Cursor tokens round-trip a delegation's position exactly and reject anything they didn't
// produce: malformed input, an unknown version, another year's token, an out-of-range time.
func TestCursorToken(t *testing.T) {
	t.Parallel()

	cursor := tezos.Cursor{Timestamp: time.Date(2025, 1, 15, 10, 0, 5, 123456000, time.UTC), ID: 1939557726552064}

	t.Run("it round-trips a position at microsecond precision", func(t *testing.T) {
		t.Parallel()

		for _, year := range []uint64{0, 2025} {
			// Act
			decoded, err := bind.DecodeCursor(bind.EncodeCursor(cursor, year), year)

			// Assert
			require.NoError(t, err, "year %d", year)
			assert.True(
				t,
				cursor.Timestamp.Equal(decoded.Timestamp),
				"timestamp %s != %s",
				cursor.Timestamp,
				decoded.Timestamp,
			)
			assert.Equal(t, cursor.ID, decoded.ID)
		}
	})

	t.Run("it produces URL-safe tokens", func(t *testing.T) {
		t.Parallel()

		assert.Regexp(t, `^[A-Za-z0-9_-]+$`, bind.EncodeCursor(cursor, 2025))
	})

	t.Run("it rejects a token issued under a different year filter", func(t *testing.T) {
		t.Parallel()

		token := bind.EncodeCursor(cursor, 2025)

		for _, year := range []uint64{0, 2024} {
			_, err := bind.DecodeCursor(token, year)
			assert.ErrorIs(t, err, bind.ErrInvalidCursor, "replayed with year=%d", year)
		}
	})

	t.Run("it rejects malformed tokens", func(t *testing.T) {
		t.Parallel()

		encode := func(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }
		for name, token := range map[string]string{
			"empty":            "",
			"not base64":       "!!!",
			"unknown version":  encode("v0:0:1736935205000000:5"),
			"missing fields":   encode("v1:0:1736935205000000"),
			"extra fields":     encode("v1:0:1736935205000000:5:6"),
			"non-numeric year": encode("v1:y:1736935205000000:5"),
			"non-numeric time": encode("v1:0:soon:5"),
			"non-numeric id":   encode("v1:0:1736935205000000:five"),
			"time before 0001": encode("v1:0:-9223372036854775808:5"),
			"time after 9999":  encode("v1:0:9223372036854775807:5"),
		} {
			_, err := bind.DecodeCursor(token, 0)
			assert.ErrorIs(t, err, bind.ErrInvalidCursor, name)
		}
	})
}

// Only the cursor parameters a request actually carries become criteria options, each bound to
// the request's year filter.
func TestCursorOptions(t *testing.T) {
	t.Parallel()

	cursor := tezos.Cursor{Timestamp: time.Date(2025, 1, 15, 10, 0, 5, 0, time.UTC), ID: 7}
	token := bind.EncodeCursor(cursor, 2025)

	criteriaFor := func(t *testing.T, req api.DelegationsRequest) tezos.DelegationsCriteria {
		t.Helper()

		opts, err := bind.CursorOptions(req)
		require.NoError(t, err)

		criteria, err := tezos.NewDelegationsCriteria(req.Year, req.Page, req.PerPage, opts...)
		require.NoError(t, err)

		return criteria
	}

	t.Run("it adds no options when neither cursor is given", func(t *testing.T) {
		t.Parallel()

		opts, err := bind.CursorOptions(api.DelegationsRequest{Year: 2025})

		require.NoError(t, err)
		assert.Empty(t, opts)
	})

	t.Run("it decodes an after cursor", func(t *testing.T) {
		t.Parallel()

		criteria := criteriaFor(t, api.DelegationsRequest{Year: 2025, After: token})

		require.NotNil(t, criteria.After)
		assert.Equal(t, cursor.ID, criteria.After.ID)
		assert.Nil(t, criteria.Before)
	})

	t.Run("it decodes a before cursor", func(t *testing.T) {
		t.Parallel()

		criteria := criteriaFor(t, api.DelegationsRequest{Year: 2025, Before: token})

		require.NotNil(t, criteria.Before)
		assert.Equal(t, cursor.ID, criteria.Before.ID)
		assert.Nil(t, criteria.After)
	})

	t.Run("it rejects an invalid cursor", func(t *testing.T) {
		t.Parallel()

		for name, req := range map[string]api.DelegationsRequest{
			"malformed after":        {After: "!!!"},
			"malformed before":       {Before: "!!!"},
			"after for another year": {Year: 2024, After: token},
		} {
			_, err := bind.CursorOptions(req)
			assert.ErrorIs(t, err, bind.ErrInvalidCursor, name)
		}
	})
}
