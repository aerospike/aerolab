//go:build integration_cloud

package backend_test

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	rtypes "github.com/aws/aws-sdk-go-v2/service/route53/types"
	"github.com/citrusleaf/aerolab/pkg/backend/backends"
	"github.com/citrusleaf/aerolab/pkg/backend/clouds/baws"
	"github.com/citrusleaf/aerolab/pkg/backend/clouds/bgcp"
	"github.com/citrusleaf/aerolab/pkg/backend/clouds/bgcp/connect"
	"github.com/stretchr/testify/require"
	dns "google.golang.org/api/dns/v1"
	"google.golang.org/api/option"
)

func Test20_InstancesDNS(t *testing.T) {
	t.Cleanup(cleanup)
	test := &testInstancesDNS{}
	t.Run("setup", testSetup)
	t.Run("inventory empty", testInventoryEmpty)
	t.Run("create instance", test.testCreateInstance)
	t.Run("test dns", test.testInstancesDNS)
	t.Run("records created", test.testRecordsCreated)
	t.Run("cleanup dns", test.testCleanupDNSOrphansOnly)
	t.Run("terminate instance", test.testInstancesTerminate)
	t.Run("records deleted", test.testRecordsDeleted)
	t.Run("cleanup dns", test.testCleanupDNS)
	t.Run("end inventory empty", testInventoryEmpty)
}

// testInstancesDNS carries the records the create step should have put in the
// hosted zone from one subtest to the next: the terminate check has to know
// which names to look for after the instances (and with them the inventory
// entries the names are derived from) are gone.
type testInstancesDNS struct {
	wantRecords map[string]string // FQDN (no trailing dot) -> IP address
}

// dnsConfig describes the hosted zone the DNS test attaches records to. It is
// account-specific, so the test skips unless it is configured.
//
// AEROLAB_TEST_DNS_ZONE_ID is the Route53 hosted zone id on AWS and the managed
// zone name on GCP. AEROLAB_TEST_DNS_REGION is the Route53 region on AWS; GCP
// Cloud DNS is global.
func dnsConfig(t *testing.T) (zoneID, domain, region string) {
	t.Helper()
	zoneID = os.Getenv("AEROLAB_TEST_DNS_ZONE_ID")
	domain = os.Getenv("AEROLAB_TEST_DNS_DOMAIN")
	if zoneID == "" || domain == "" {
		t.Skip("set AEROLAB_TEST_DNS_ZONE_ID and AEROLAB_TEST_DNS_DOMAIN to run the DNS test")
	}
	region = os.Getenv("AEROLAB_TEST_DNS_REGION")
	if region == "" {
		region = "us-east-1"
		if cloud == "gcp" {
			region = "global"
		}
	}
	return zoneID, domain, region
}

func (d *testInstancesDNS) testCreateInstance(t *testing.T) {
	require.NoError(t, setup(false))
	if cloud == "docker" {
		t.Skip("docker does not support dns")
		return
	}
	zoneID, domain, region := dnsConfig(t)
	require.NoError(t, testBackend.RefreshChangedInventory())
	image := getBasicImage(t)
	placement := Options.TestRegions[0] + "a"
	if strings.Count(Options.TestRegions[0], "-") == 1 {
		placement = Options.TestRegions[0] + "-a"
	}
	params := map[backends.BackendType]any{
		backends.BackendTypeAWS: &baws.CreateInstanceParams{
			Image:            image,
			NetworkPlacement: Options.TestRegions[0] + "a",
			InstanceType:     "r6a.large",
			Disks:            []string{"type=gp2,size=20,count=1"},
			Firewalls:        []string{},
			CustomDNS: &backends.InstanceDNS{
				DomainID:   zoneID,
				DomainName: domain,
				Region:     region,
			},
		},
		backends.BackendTypeGCP: gcpParams(&bgcp.CreateInstanceParams{
			Image:            image,
			NetworkPlacement: placement,
			InstanceType:     "e2-standard-4",
			Disks:            []string{"type=pd-ssd,size=20,count=1"},
			Firewalls:        []string{},
			CustomDNS: &backends.InstanceDNS{
				DomainID:   zoneID,
				DomainName: domain,
				Region:     region,
			},
		}),
	}
	insts, err := testBackend.CreateInstances(&backends.CreateInstanceInput{
		ClusterName:           "test-cluster",
		Nodes:                 3,
		BackendType:           backendType,
		Owner:                 "test-owner",
		Description:           "test-description",
		BackendSpecificParams: params,
	}, instanceReadyWait())
	require.NoError(t, err)
	require.Equal(t, insts.Instances.Count(), 3)
	err = testBackend.RefreshChangedInventory()
	require.NoError(t, err)
	require.Equal(t, testBackend.GetInventory().Instances.WithNotState(backends.LifeCycleStateTerminated).Count(), 3)
}

func (d *testInstancesDNS) testInstancesDNS(t *testing.T) {
	require.NoError(t, setup(false))
	if cloud == "docker" {
		t.Skip("docker does not support dns")
		return
	}
	zoneID, domain, region := dnsConfig(t)
	require.NoError(t, testBackend.RefreshChangedInventory())
	inst := testBackend.GetInventory().Instances.WithNotState(backends.LifeCycleStateTerminated)
	require.Equal(t, inst.Count(), 3)
	for _, i := range inst.Describe() {
		require.Equal(t, i.CustomDNS.DomainID, zoneID)
		require.Equal(t, i.CustomDNS.DomainName, domain)
		require.Equal(t, i.CustomDNS.Region, region)
		// The record name is left empty at create time, so each backend fills
		// in its own default: the instance id on AWS, a hash of the instance
		// name prefixed with "i-" on GCP.
		if cloud == "gcp" {
			require.True(t, strings.HasPrefix(i.CustomDNS.Name, "i-"), "GCP DNS name %q should be an i-<hash> record", i.CustomDNS.Name)
		} else {
			require.Equal(t, i.CustomDNS.Name, i.InstanceID)
		}
	}
}

// testRecordsCreated asserts that the create actually reached the DNS provider.
// Both backends create their records on a best-effort basis -- a failure is
// logged and the create still succeeds -- so an inventory that merely echoes
// back the DNS tags says nothing about whether the zone holds any records.
func (d *testInstancesDNS) testRecordsCreated(t *testing.T) {
	require.NoError(t, setup(false))
	if cloud == "docker" {
		t.Skip("docker does not support dns")
		return
	}
	zone := newDNSZone(t)
	require.NoError(t, testBackend.RefreshChangedInventory())
	inst := testBackend.GetInventory().Instances.WithNotState(backends.LifeCycleStateTerminated)
	require.Equal(t, inst.Count(), 3)
	d.wantRecords = make(map[string]string, inst.Count())
	for _, i := range inst.Describe() {
		require.NotNil(t, i.CustomDNS)
		d.wantRecords[i.CustomDNS.GetFQDN()] = i.IP.Routable()
	}
	require.Len(t, d.wantRecords, 3)
	zone.requirePresent(t, d.wantRecords)
}

// testCleanupDNSOrphansOnly runs the cleanup while the instances are still
// alive: it must delete a record whose instance is gone and leave the records of
// running instances alone. The expiry system runs this same call unattended, so
// a cleanup that deleted live records would take a working cluster's DNS with
// it, and one that deleted nothing would let spot-terminated nodes accumulate.
func (d *testInstancesDNS) testCleanupDNSOrphansOnly(t *testing.T) {
	require.NoError(t, setup(false))
	if cloud == "docker" {
		t.Skip("docker does not support dns")
		return
	}
	zone := newDNSZone(t)
	require.NotEmpty(t, d.wantRecords, "the records created subtest must have run first")
	// The orphan sits directly under the zone's own name rather than under
	// AEROLAB_TEST_DNS_DOMAIN: AWS only considers a record for cleanup when
	// stripping its first label leaves exactly the hosted zone name, and the
	// two differ whenever the configured domain is a subdomain of the zone.
	orphan := fmt.Sprintf("i-aerolabtest%d.%s", time.Now().UnixNano(), strings.TrimSuffix(zone.suffix(t), "."))
	// RFC 5737 documentation address: never routable, so a leaked record cannot
	// point at anything real.
	const orphanIP = "192.0.2.1"
	zone.createRecord(t, orphan, orphanIP)
	t.Cleanup(func() { zone.removeRecordQuietly(orphan, orphanIP) })

	require.NoError(t, testBackend.RefreshChangedInventory())
	require.NoError(t, testBackend.CleanupDNS())
	zone.requireAbsent(t, orphan)
	zone.requirePresent(t, d.wantRecords)
}

func (d *testInstancesDNS) testInstancesTerminate(t *testing.T) {
	require.NoError(t, setup(false))
	if cloud == "docker" {
		t.Skip("docker does not support dns")
		return
	}
	require.NoError(t, testBackend.RefreshChangedInventory())
	inst := testBackend.GetInventory().Instances.WithNotState(backends.LifeCycleStateTerminated)
	err := inst.Terminate(2 * time.Minute)
	require.NoError(t, err)
	err = testBackend.RefreshChangedInventory()
	require.NoError(t, err)
	require.Equal(t, testBackend.GetInventory().Instances.WithNotState(backends.LifeCycleStateTerminated).Count(), 0)
	require.NoError(t, testBackend.GetInventory().Firewalls.Delete(10*time.Minute))
}

// testRecordsDeleted asserts that terminate took the records with it. Like
// creation, the delete is best-effort in both backends, so nothing but the zone
// itself can confirm it happened.
func (d *testInstancesDNS) testRecordsDeleted(t *testing.T) {
	require.NoError(t, setup(false))
	if cloud == "docker" {
		t.Skip("docker does not support dns")
		return
	}
	zone := newDNSZone(t)
	require.NotEmpty(t, d.wantRecords, "the records created subtest must have run first")
	fqdns := make([]string, 0, len(d.wantRecords))
	for fqdn := range d.wantRecords {
		fqdns = append(fqdns, fqdn)
	}
	zone.requireAbsent(t, fqdns...)
}

func (d *testInstancesDNS) testCleanupDNS(t *testing.T) {
	require.NoError(t, setup(false))
	if cloud == "docker" {
		t.Skip("docker does not support dns")
		return
	}
	require.NoError(t, testBackend.RefreshChangedInventory())
	err := testBackend.CleanupDNS()
	require.NoError(t, err)
}

// dnsZone reads and writes A records in the hosted zone under test, straight
// through the provider API. The backends interface has no way to enumerate DNS
// records -- CleanupDNS() is the whole of it -- so the test talks to Route53 and
// Cloud DNS itself, using the same credentials the backend authenticates with.
type dnsZone struct {
	zoneID string
	region string
}

const (
	// dnsRecordWait bounds how long a record is given to show up in (or
	// disappear from) the zone, and dnsRecordPoll how often the zone is read
	// while waiting. Both providers apply a change before they acknowledge it,
	// so the first read normally already agrees; the wait only absorbs a
	// provider that is briefly behind, and a create or delete that never
	// happened still fails the test once it runs out.
	dnsRecordWait = 90 * time.Second
	dnsRecordPoll = 5 * time.Second
)

// newDNSZone binds to the configured zone, skipping the test when there is
// none.
func newDNSZone(t *testing.T) *dnsZone {
	t.Helper()
	zoneID, _, region := dnsConfig(t)
	return &dnsZone{zoneID: zoneID, region: region}
}

// suffix returns the zone's own DNS name, with the trailing dot the providers
// report it with.
func (z *dnsZone) suffix(t *testing.T) string {
	t.Helper()
	if cloud == "gcp" {
		svc, closeClient := gcpDNSService(t)
		defer closeClient()
		zone, err := svc.ManagedZones.Get(testCredentials.GCP.Project, z.zoneID).Do()
		require.NoError(t, err)
		return zone.DnsName
	}
	out, err := awsRoute53Client(t, z.region).GetHostedZone(context.Background(), &route53.GetHostedZoneInput{
		Id: aws.String(z.zoneID),
	})
	require.NoError(t, err)
	return aws.ToString(out.HostedZone.Name)
}

// records returns every A record in the zone, keyed by name without the trailing
// dot. The zone is not exclusively ours, so callers must look up the names they
// care about rather than compare the whole map.
func (z *dnsZone) records(t *testing.T) map[string][]string {
	t.Helper()
	out := map[string][]string{}
	if cloud == "gcp" {
		svc, closeClient := gcpDNSService(t)
		defer closeClient()
		err := svc.ResourceRecordSets.List(testCredentials.GCP.Project, z.zoneID).Pages(context.Background(), func(page *dns.ResourceRecordSetsListResponse) error {
			for _, record := range page.Rrsets {
				if record.Type != "A" {
					continue
				}
				out[strings.TrimSuffix(record.Name, ".")] = record.Rrdatas
			}
			return nil
		})
		require.NoError(t, err)
		return out
	}
	paginator := route53.NewListResourceRecordSetsPaginator(awsRoute53Client(t, z.region), &route53.ListResourceRecordSetsInput{
		HostedZoneId: aws.String(z.zoneID),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(context.Background())
		require.NoError(t, err)
		for _, record := range page.ResourceRecordSets {
			if record.Type != rtypes.RRTypeA {
				continue
			}
			values := make([]string, 0, len(record.ResourceRecords))
			for _, value := range record.ResourceRecords {
				values = append(values, aws.ToString(value.Value))
			}
			out[strings.TrimSuffix(aws.ToString(record.Name), ".")] = values
		}
	}
	return out
}

// requirePresent fails unless every FQDN in want resolves to exactly its
// expected address in the zone.
func (z *dnsZone) requirePresent(t *testing.T, want map[string]string) {
	t.Helper()
	var problems []string
	for deadline := time.Now().Add(dnsRecordWait); ; time.Sleep(dnsRecordPoll) {
		records := z.records(t)
		problems = nil
		for fqdn, ip := range want {
			got, ok := records[fqdn]
			switch {
			case !ok:
				problems = append(problems, fmt.Sprintf("%s: no A record in the zone", fqdn))
			case len(got) != 1 || got[0] != ip:
				problems = append(problems, fmt.Sprintf("%s: A record is %v, want [%s]", fqdn, got, ip))
			}
		}
		if len(problems) == 0 {
			return
		}
		if time.Now().After(deadline) {
			break
		}
	}
	sort.Strings(problems)
	require.Failf(t, "expected DNS records are not in zone "+z.zoneID, "after %s: %s", dnsRecordWait, strings.Join(problems, "; "))
}

// requireAbsent fails unless none of the given FQDNs has an A record left.
func (z *dnsZone) requireAbsent(t *testing.T, fqdns ...string) {
	t.Helper()
	var leftover []string
	for deadline := time.Now().Add(dnsRecordWait); ; time.Sleep(dnsRecordPoll) {
		records := z.records(t)
		leftover = nil
		for _, fqdn := range fqdns {
			if got, ok := records[fqdn]; ok {
				leftover = append(leftover, fmt.Sprintf("%s: still an A record %v", fqdn, got))
			}
		}
		if len(leftover) == 0 {
			return
		}
		if time.Now().After(deadline) {
			break
		}
	}
	sort.Strings(leftover)
	require.Failf(t, "DNS records were not deleted from zone "+z.zoneID, "after %s: %s", dnsRecordWait, strings.Join(leftover, "; "))
}

func (z *dnsZone) createRecord(t *testing.T, fqdn, ip string) {
	t.Helper()
	require.NoError(t, z.changeRecord(fqdn, ip, dnsRecordCreate))
}

// removeRecordQuietly deletes a record and ignores the outcome. It exists so a
// failed run does not leave a record behind in a zone the account also uses for
// other things; the record already being gone is the normal case, and a test
// that has finished has nothing to gain from failing over the sweep.
func (z *dnsZone) removeRecordQuietly(fqdn, ip string) {
	_ = z.changeRecord(fqdn, ip, dnsRecordDelete)
}

type dnsRecordChange int

const (
	dnsRecordCreate dnsRecordChange = iota
	dnsRecordDelete
)

func (z *dnsZone) changeRecord(fqdn, ip string, change dnsRecordChange) error {
	if cloud == "gcp" {
		svc, closeClient, err := newGCPDNSService()
		if err != nil {
			return err
		}
		defer closeClient()
		if change == dnsRecordDelete {
			_, err = svc.ResourceRecordSets.Delete(testCredentials.GCP.Project, z.zoneID, fqdn+".", "A").Do()
			return err
		}
		_, err = svc.ResourceRecordSets.Create(testCredentials.GCP.Project, z.zoneID, &dns.ResourceRecordSet{
			Kind:    "dns#resourceRecordSet",
			Name:    fqdn + ".",
			Rrdatas: []string{ip},
			Ttl:     10,
			Type:    "A",
		}).Do()
		return err
	}
	client, err := baws.GetRoute53Client(testCredentials, &z.region)
	if err != nil {
		return err
	}
	action := rtypes.ChangeActionUpsert
	if change == dnsRecordDelete {
		action = rtypes.ChangeActionDelete
	}
	_, err = client.ChangeResourceRecordSets(context.Background(), &route53.ChangeResourceRecordSetsInput{
		HostedZoneId: aws.String(z.zoneID),
		ChangeBatch: &rtypes.ChangeBatch{
			Changes: []rtypes.Change{{
				Action: action,
				ResourceRecordSet: &rtypes.ResourceRecordSet{
					Name:            aws.String(fqdn),
					Type:            rtypes.RRTypeA,
					TTL:             aws.Int64(10),
					ResourceRecords: []rtypes.ResourceRecord{{Value: aws.String(ip)}},
				},
			}},
		},
	})
	return err
}

func awsRoute53Client(t *testing.T, region string) *route53.Client {
	t.Helper()
	client, err := baws.GetRoute53Client(testCredentials, &region)
	require.NoError(t, err)
	return client
}

// newGCPDNSService returns a Cloud DNS client plus the function that releases
// its connections.
func newGCPDNSService() (*dns.Service, func(), error) {
	client, err := connect.GetClient(&testCredentials.GCP, nil)
	if err != nil {
		return nil, nil, err
	}
	svc, err := dns.NewService(context.Background(), option.WithHTTPClient(client))
	if err != nil {
		client.CloseIdleConnections()
		return nil, nil, err
	}
	return svc, client.CloseIdleConnections, nil
}

func gcpDNSService(t *testing.T) (*dns.Service, func()) {
	t.Helper()
	svc, closeClient, err := newGCPDNSService()
	require.NoError(t, err)
	return svc, closeClient
}
