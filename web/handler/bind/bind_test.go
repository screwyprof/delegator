package bind_test

import (
	"encoding/base64"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/screwyprof/delegator/web/handler/bind"
	"github.com/screwyprof/delegator/web/tezos"
)

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
			require.NotNil(t, decoded)
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

	t.Run("it treats an empty token as no cursor", func(t *testing.T) {
		t.Parallel()

		decoded, err := bind.DecodeCursor("", 0)

		require.NoError(t, err)
		assert.Nil(t, decoded)
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
