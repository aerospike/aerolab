package backend_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestImages(t *testing.T) {
	t.Cleanup(cleanup)
	t.Run("setup", testSetup)
	t.Run("inventory empty", testInventoryEmpty)
	t.Run("image create", testImageCreate)
	t.Run("image delete", testImageDelete)
	t.Run("end inventory empty", testInventoryEmpty)
}

func testImageCreate(t *testing.T) {
	require.NoError(t, setup(false))
	require.NoError(t, testBackend.RefreshChangedInventory())
}

func testImageDelete(t *testing.T) {
	require.NoError(t, setup(false))
	require.NoError(t, testBackend.RefreshChangedInventory())
}
