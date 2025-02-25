package backend_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/aerospike/aerolab/pkg/backend/backends"
)

var _ = Describe("Instance integration tests", func() {
	When("inventory is empty", func() {
		var inventory *backends.Inventory
		BeforeEach(func() {
			inventory = testBackend.GetInventory()
		})
		When("listing the instance count", func() {
			It("zero instances exist", func() {
				Expect(inventory.Instances.Count()).To(Equal(0))
			})
		})
		When("listing the volume count", func() {
			It("zero volumes exist", func() {
				Expect(inventory.Volumes.Count()).To(Equal(0))
			})
		})
		When("listing the network count", func() {
			It("zero managed networks exist", func() {
				Expect(inventory.Networks.WithAerolabManaged(true).Count()).To(Equal(0))
			})
			It("one unmanaged network exists", func() {
				Expect(inventory.Networks.WithAerolabManaged(false).Count()).To(Equal(1))
			})
		})
		When("listing the firewall count", func() {
			It("zero firewalls exist", func() {
				Expect(inventory.Firewalls.Count()).To(Equal(0))
			})
		})
		When("listing the image count", func() {
			It("zero premade images exist", func() {
				Expect(inventory.Images.WithInAccount(true).Count()).To(Equal(0))
			})
			It("34 default images exist", func() {
				Expect(inventory.Images.WithInAccount(false).Count()).To(Equal(34))
			})
		})
	})
})
