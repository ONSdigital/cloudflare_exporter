package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseZoneIDs_ReturnsMapOfNonPendingZones(t *testing.T) {
	f, err := os.Open("testdata/zones_resp.json")
	require.Nil(t, err)
	defer f.Close()
	zones, err := parseZoneIDs(f)
	require.Nil(t, err)
	assert.Equal(t, zones, map[string]string{"zone-1-id": "zone-1", "zone-2-id": "zone-2"})
}
