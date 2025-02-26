package backend_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/sshexec"
)

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
	When("test tagging", func() {
		It("should tag instances", func() {
			insts := testBackend.GetInventory().Instances.WithNotState(backends.LifeCycleStateTerminated)
			Expect(insts.Count()).To(Equal(3))
			Expect(insts.AddTags(map[string]string{"test-tag": "test-value"})).NotTo(HaveOccurred())
			Expect(testBackend.RefreshChangedInventory()).NotTo(HaveOccurred())

			insts = testBackend.GetInventory().Instances.WithNotState(backends.LifeCycleStateTerminated)
			Expect(insts.Count()).To(Equal(3))
			for _, inst := range insts.Describe() {
				Expect(inst.Tags).To(HaveKeyWithValue("test-tag", "test-value"))
			}

			Expect(insts.RemoveTags([]string{"test-tag"})).NotTo(HaveOccurred())
			Expect(testBackend.RefreshChangedInventory()).NotTo(HaveOccurred())

			insts = testBackend.GetInventory().Instances.WithNotState(backends.LifeCycleStateTerminated)
			Expect(insts.Count()).To(Equal(3))
			for _, inst := range insts.Describe() {
				Expect(inst.Tags).NotTo(HaveKey("test-tag"))
			}
		})
	})
	When("test expiry", func() {
		It("should set expiry", func() {
			insts := testBackend.GetInventory().Instances.WithNotState(backends.LifeCycleStateTerminated)
			Expect(insts.Count()).To(Equal(3))
			newExp := time.Now().Add(time.Hour * 24 * 30)
			Expect(insts.ChangeExpiry(newExp)).NotTo(HaveOccurred())
			Expect(testBackend.RefreshChangedInventory()).NotTo(HaveOccurred())

			insts = testBackend.GetInventory().Instances.WithNotState(backends.LifeCycleStateTerminated)
			Expect(insts.Count()).To(Equal(3))
			for _, inst := range insts.Describe() {
				Expect(inst.Expires).To(BeTemporally("~", newExp, time.Second*10))
			}
			Expect(insts.ChangeExpiry(time.Time{})).NotTo(HaveOccurred())
			Expect(testBackend.RefreshChangedInventory()).NotTo(HaveOccurred())

			insts = testBackend.GetInventory().Instances.WithNotState(backends.LifeCycleStateTerminated)
			Expect(insts.Count()).To(Equal(3))
			for _, inst := range insts.Describe() {
				Expect(inst.Expires).To(BeZero())
			}
		})
	})
	When("test stop/start", func() {
		It("should stop", func() {
			insts := testBackend.GetInventory().Instances.WithState(backends.LifeCycleStateRunning)
			Expect(insts.Count()).To(Equal(3))
			Expect(insts.Stop(false, 2*time.Minute)).NotTo(HaveOccurred())
			Expect(testBackend.RefreshChangedInventory()).NotTo(HaveOccurred())
			insts = testBackend.GetInventory().Instances.WithState(backends.LifeCycleStateRunning)
			Expect(insts.Count()).To(Equal(0))
			for _, inst := range insts.Describe() {
				Expect(inst.InstanceState).To(Equal(backends.LifeCycleStateStopped))
			}
		})
		It("should start", func() {
			insts := testBackend.GetInventory().Instances.WithState(backends.LifeCycleStateStopped)
			Expect(insts.Count()).To(Equal(3))
			Expect(insts.Start(2 * time.Minute)).NotTo(HaveOccurred())
			Expect(testBackend.RefreshChangedInventory()).NotTo(HaveOccurred())
			insts = testBackend.GetInventory().Instances.WithState(backends.LifeCycleStateRunning)
			Expect(insts.Count()).To(Equal(3))
			for _, inst := range insts.Describe() {
				Expect(inst.InstanceState).To(Equal(backends.LifeCycleStateRunning))
			}
		})
	})
	When("test exec", func() {
		It("should exec", func() {
			insts := testBackend.GetInventory().Instances.WithNotState(backends.LifeCycleStateTerminated)
			Expect(insts.Count()).To(Equal(3))
			outs := insts.Exec(&backends.ExecInput{
				ExecDetail: sshexec.ExecDetail{
					Command:        []string{"ls", "-la"},
					Stdin:          nil,
					Stdout:         nil,
					Stderr:         nil,
					SessionTimeout: 10 * time.Second,
					Env:            []*sshexec.Env{},
					Terminal:       true,
				},
				Username:        "root",
				ConnectTimeout:  10 * time.Second,
				ParallelThreads: 2,
			})
			Expect(outs).To(HaveLen(3))
			for _, out := range outs {
				Expect(out.Output).NotTo(BeNil())
				Expect(out.Output.Err).NotTo(HaveOccurred())
			}
		})
	})
	When("test sftp", func() {
		It("should sftp", func() {
			insts := testBackend.GetInventory().Instances.WithNotState(backends.LifeCycleStateTerminated)
			Expect(insts.Count()).To(Equal(3))
			confs, err := insts.GetSftpConfig("root")
			Expect(err).NotTo(HaveOccurred())
			Expect(confs).To(HaveLen(3))
			for _, conf := range confs {
				Expect(conf.Host).NotTo(BeEmpty())
				Expect(conf.Port).NotTo(BeZero())
				Expect(conf.Username).To(Equal("root"))
				sftpClient, err := sshexec.NewSftp(conf)
				Expect(err).NotTo(HaveOccurred())
				Expect(sftpClient).NotTo(BeNil())
				remote := sftpClient.GetRemoteClient()
				Expect(remote).NotTo(BeNil())
				files, err := remote.ReadDir("/tmp")
				Expect(err).NotTo(HaveOccurred())
				Expect(files).NotTo(BeEmpty())
				sftpClient.Close()
			}
		})
	})
	When("test terminate", func() {
		It("should terminate", func() {
			insts := testBackend.GetInventory().Instances.WithNotState(backends.LifeCycleStateTerminated)
			Expect(insts.Count()).To(Equal(3))
			Expect(insts.Terminate(2 * time.Minute)).NotTo(HaveOccurred())
			Expect(testBackend.RefreshChangedInventory()).NotTo(HaveOccurred())
			Expect(testBackend.GetInventory().Instances.WithNotState(backends.LifeCycleStateTerminated).Count()).To(Equal(0))
			Expect(testBackend.GetInventory().Volumes.Count()).To(Equal(0))
			Expect(testBackend.GetInventory().Firewalls.Count()).To(Equal(1))
			Expect(testBackend.GetInventory().Firewalls.Delete(2 * time.Minute)).NotTo(HaveOccurred())
			Expect(testBackend.RefreshChangedInventory()).NotTo(HaveOccurred())
			Expect(testBackend.GetInventory().Firewalls.Count()).To(Equal(0))
		})
	})
})
