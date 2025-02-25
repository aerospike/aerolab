package backend_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/aerospike/aerolab/pkg/backend/backends"
)

// Q1: how to make it abort further tests if one fails?
// Q2: how to make reusable tests, like for example I would like to test if inventory is empty again later?
// Q3: are we sure the Describe->When->It always runs in order defined?

var _ = Describe("Instance integration tests", func() {
	When("inventory is empty", func() {
		var inventory *backends.Inventory
		BeforeEach(func() {
			inventory = testBackend.GetInventory()
		})
		When("listing the instance count", func() {
			It("zero instances exist", func() {
				Expect(inventory.Instances.WithNotState(backends.LifeCycleStateTerminated).Count()).To(Equal(0))
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
		When("no expiry system is installed", func() {
			It("zero expiry systems exist", func() {
				expiryList, err := testBackend.ExpiryList()
				Expect(err).NotTo(HaveOccurred())
				Expect(expiryList.ExpirySystems).To(BeEmpty())
			})
		})
	})
	When("create a new instance", func() {
		var image *backends.Image
		BeforeEach(func() {
			imgs := testBackend.GetInventory().Images
			Expect(imgs.Describe()).NotTo(BeEmpty())
			img1 := imgs.WithInAccount(false)
			Expect(img1.Describe()).NotTo(BeEmpty())
			img2 := img1.WithOSName("ubuntu")
			Expect(img2.Describe()).NotTo(BeEmpty())
			img3 := img2.WithOSVersion("24.04")
			Expect(img3.Describe()).NotTo(BeEmpty())
			img4 := img3.WithArchitecture(backends.ArchitectureX8664)
			Expect(img4.Describe()).NotTo(BeEmpty())
			img5 := img4.WithZoneName(Options.TestRegions[0])
			Expect(img5.Describe()).NotTo(BeEmpty())
			image = img5.Describe()[0]
			Expect(image).NotTo(BeNil())
		})
		It("should get price for creating an instance", func() {
			costPPH, costGB, err := testBackend.CreateInstancesGetPrice(&backends.CreateInstanceInput{
				ClusterName:      "test-cluster",
				Nodes:            3,
				Image:            image,
				NetworkPlacement: Options.TestRegions[0] + "a",
				Firewalls:        []string{"test-aerolab-fw"},
				BackendType:      backends.BackendTypeAWS,
				InstanceType:     "r6a.large",
				Owner:            "test-owner",
				Description:      "test-description",
				Disks:            []string{"type=gp2,size=20,count=2"},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(costPPH).NotTo(BeZero())
			Expect(costGB).NotTo(BeZero())
			costPPH1, costGB1, err := testBackend.CreateInstancesGetPrice(&backends.CreateInstanceInput{
				ClusterName:      "test-cluster",
				Nodes:            1,
				Image:            image,
				NetworkPlacement: Options.TestRegions[0] + "a",
				Firewalls:        []string{"test-aerolab-fw"},
				BackendType:      backends.BackendTypeAWS,
				InstanceType:     "r6a.large",
				Owner:            "test-owner",
				Description:      "test-description",
				Disks:            []string{"type=gp2,size=20,count=1"},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(costPPH1 * 3).To(Equal(costPPH))
			Expect(costGB1 * 6).To(Equal(costGB))

		})
		It("should create a cluster of 3 instances", func() {
			insts, err := testBackend.CreateInstances(&backends.CreateInstanceInput{
				ClusterName:      "test-cluster",
				Nodes:            3,
				Image:            image,
				NetworkPlacement: Options.TestRegions[0] + "a",
				Firewalls:        []string{},
				BackendType:      backends.BackendTypeAWS,
				InstanceType:     "r6a.large",
				Owner:            "test-owner",
				Description:      "test-description",
				Disks:            []string{"type=gp2,size=20,count=2"},
			}, 2*time.Minute)
			Expect(err).NotTo(HaveOccurred())
			Expect(insts.Instances.Count()).To(Equal(3))
		})
	})
	When("get new inventory", func() {
		It("should get new inventory", func() {
			err := testBackend.RefreshChangedInventory()
			Expect(err).NotTo(HaveOccurred())
			Expect(testBackend.GetInventory().Firewalls.Count()).To(Equal(1))
			Expect(testBackend.GetInventory().Instances.WithNotState(backends.LifeCycleStateTerminated).Count()).To(Equal(3))
			Expect(testBackend.GetInventory().Volumes.Count()).To(Equal(6))
			for _, vol := range testBackend.GetInventory().Volumes.Describe() {
				Expect(vol.Size).To(Equal(20 * backends.StorageGiB))
			}
		})
	})
})
