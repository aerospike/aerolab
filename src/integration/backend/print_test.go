package backend_test

import "testing"

func Test00_Print(t *testing.T) {
	t.Cleanup(cleanup)
	t.Run("setup", testSetup)
	t.Run("inventory print", testInventoryPrint)
	t.Run("expiries print", testExpiriesPrint)
}
