package util_test

import (
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetRandomTime(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		lowerBound time.Duration
		upperBound time.Duration
	}{
		{1 * time.Second, 10 * time.Second},
		{0, 0},
		{-1 * time.Second, -3 * time.Second},
		{1 * time.Second, 2000000001 * time.Nanosecond},
		{1 * time.Millisecond, 10 * time.Millisecond},
		// {1 * time.Second, 1000000001 * time.Nanosecond}, // This case fails
	}

	// Loop through each test case
	for i, tc := range testCases {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()

			// Try each test case 100 times to avoid fluke test results
			for j := range 100 {
				t.Run(strconv.Itoa(j), func(t *testing.T) {
					t.Parallel()

					actual := util.GetRandomTime(tc.lowerBound, tc.upperBound)

					if tc.lowerBound > 0 && tc.upperBound > 0 {
						if actual < tc.lowerBound {
							t.Fatalf("Randomly computed time %v should not be less than lowerBound %v", actual, tc.lowerBound)
						}

						if actual > tc.upperBound {
							t.Fatalf("Randomly computed time %v should not be greater than upperBound %v", actual, tc.upperBound)
						}
					}
				})
			}
		})
	}
}

func TestGenerateUUID(t *testing.T) {
	t.Parallel()

	t.Run("ValidUUIDFormat", func(t *testing.T) {
		t.Parallel()

		generatedUUID := util.GenerateUUID()
		require.NotEmpty(t, generatedUUID, "Generated UUID should not be empty")

		// Verify it's a valid UUID by parsing it
		parsedUUID, err := uuid.Parse(generatedUUID)
		require.NoError(t, err, "Generated UUID should be parseable")
		assert.NotEqual(t, uuid.Nil, parsedUUID, "Generated UUID should not be nil UUID")
	})

	t.Run("Uniqueness", func(t *testing.T) {
		t.Parallel()

		uuid1 := util.GenerateUUID()
		uuid2 := util.GenerateUUID()
		assert.NotEqual(t, uuid1, uuid2, "Two generated UUIDs should be different")
	})

	t.Run("RFC4122Compliance", func(t *testing.T) {
		t.Parallel()

		generatedUUID := util.GenerateUUID()

		// RFC 4122 UUID format: xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx
		// Where y is one of [8, 9, A, B]
		assert.Len(t, generatedUUID, 36, "UUID should be 36 characters long")
		assert.Equal(t, '-', rune(generatedUUID[8]), "Character at position 8 should be hyphen")
		assert.Equal(t, '-', rune(generatedUUID[13]), "Character at position 13 should be hyphen")
		assert.Equal(t, '-', rune(generatedUUID[18]), "Character at position 18 should be hyphen")
		assert.Equal(t, '-', rune(generatedUUID[23]), "Character at position 23 should be hyphen")
	})

	t.Run("StressTest", func(t *testing.T) {
		t.Parallel()

		// Generate multiple UUIDs and ensure they're all unique
		const numUUIDs = 1000
		uuidMap := make(map[string]bool, numUUIDs)

		for range numUUIDs {
			generatedUUID := util.GenerateUUID()
			assert.False(t, uuidMap[generatedUUID], "UUID collision detected: %s", generatedUUID)
			uuidMap[generatedUUID] = true
		}

		assert.Len(t, uuidMap, numUUIDs, "All generated UUIDs should be unique")
	})
}
