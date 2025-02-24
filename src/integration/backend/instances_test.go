package backend_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/aerospike/aerolab/pkg/backend"

	_ "github.com/aerospike/aerolab/pkg/backend/clouds/baws"
)

var _ = Describe("Instance integration tests", func() {
	var err error

	BeforeEach(func() {
		err = testBackend.ForceRefreshInventory()
		Expect(err).NotTo(HaveOccurred())
	})

	When("the instance inventory is empty", func() {
		var inventory *backend.Inventory
		BeforeEach(func() {
			inventory, err = testBackend.GetInventory()
			Expect(err).NotTo(HaveOccurred())
		})
		When("listing the instance count", func() {
			It("zero instances exist", func() {
				Expect(inventory.Instances.Count()).To(Equal(0))
			})
		})
	})
})
