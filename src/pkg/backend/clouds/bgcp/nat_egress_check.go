package bgcp

import (
	"context"
	"fmt"
	"os"
	"sync"

	compute "cloud.google.com/go/compute/apiv1"
	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/aerospike/aerolab/pkg/backend/clouds/bgcp/connect"
	"github.com/lithammer/shortuuid"
	"github.com/rglonek/logger"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

// natEgressCache stores the result of NAT-coverage lookups for the lifetime
// of the process, keyed by (project, region, network, subnet). aerolab's
// create flows can call CreateInstances multiple times back-to-back (multi-AZ
// clusters, sequential template + cluster + client), and we don't want to
// re-query routers.list for the same tuple every time.
type natEgressCache struct {
	mu      sync.Mutex
	results map[string]natEgressResult
}

type natEgressResult struct {
	covered bool
	// err is set when we could not determine coverage (auth, IAM, transient
	// API errors). Treated as soft-fail at the call site: the create proceeds
	// because we cannot prove the operator is missing egress, only that we
	// could not check.
	err error
}

var natEgressCacheGlobal = &natEgressCache{results: map[string]natEgressResult{}}

// checkNATCoversSubnet aborts a create when the operator opted for no-public-IP
// VMs but no Cloud NAT in the target region/network covers the chosen subnet.
//
// Hard-fail behaviour: returning an error is what stops the create. Without
// outbound internet, install scripts (apt-get / yum / pip / curl from
// download.aerospike.com) hang for minutes before a TCP timeout, which is a
// far worse user experience than failing fast with an actionable message.
//
// Soft-fail cases (returns nil, never blocks the create):
//   - AEROLAB_SKIP_NAT_CHECK=1 -- explicit operator opt-out, used when egress
//     is provided by something the routers.list API cannot see (VPN, VPC
//     peering through a transit VPC, an internal proxy, a hand-rolled NAT
//     VM, etc.).
//   - The routers.list API call itself errors (auth, IAM, transient): we
//     cannot prove the operator lacks egress, only that we could not check,
//     so we let the create proceed and a Detail log records why.
func (s *b) checkNATCoversSubnet(ctx context.Context, region, networkSelfLink, subnetSelfLink string) error {
	if v := os.Getenv("AEROLAB_SKIP_NAT_CHECK"); v == "1" || v == "true" || v == "TRUE" {
		return nil
	}
	log := s.log.WithPrefix("checkNATCoversSubnet: job=" + shortuuid.New() + " ")

	key := s.credentials.Project + "|" + region + "|" + networkSelfLink + "|" + subnetSelfLink
	natEgressCacheGlobal.mu.Lock()
	cached, ok := natEgressCacheGlobal.results[key]
	natEgressCacheGlobal.mu.Unlock()

	res := cached
	if !ok {
		covered, err := s.computeNATCoverage(ctx, log, region, networkSelfLink, subnetSelfLink)
		res = natEgressResult{covered: covered, err: err}
		natEgressCacheGlobal.mu.Lock()
		natEgressCacheGlobal.results[key] = res
		natEgressCacheGlobal.mu.Unlock()
	}

	switch {
	case res.err != nil:
		// Soft-fail: do not block the create on our own inability to check.
		log.Detail("could not determine NAT coverage (allowing create): %s", res.err)
		return nil
	case res.covered:
		log.Detail("Cloud NAT covers %s/%s in %s", getValueFromURL(networkSelfLink), getValueFromURL(subnetSelfLink), region)
		return nil
	default:
		net := getValueFromURL(networkSelfLink)
		sub := getValueFromURL(subnetSelfLink)
		return fmt.Errorf(
			"VMs with no public IP have no outbound internet by default and Cloud NAT is NOT configured "+
				"for region=%s network=%s subnet=%s. Aborting create -- without egress the install script "+
				"would hang for minutes on apt-get / curl from download.aerospike.com before timing out.\n"+
				"Configure NAT once with:\n"+
				"    gcloud compute routers create aerolab-router --network=%s --region=%s --project=%s\n"+
				"    gcloud compute routers nats create aerolab-nat --router=aerolab-router --region=%s "+
				"--auto-allocate-nat-external-ips --nat-all-subnet-ip-ranges --enable-logging --project=%s\n"+
				"If egress is already provided through VPN, VPC peering, an internal proxy, or another "+
				"mechanism aerolab cannot detect via compute.routers.list, set AEROLAB_SKIP_NAT_CHECK=1 to "+
				"bypass this check.",
			region, net, sub,
			net, region, s.credentials.Project,
			region, s.credentials.Project,
		)
	}
}

// computeNATCoverage returns true if any Cloud NAT defined on a Cloud Router
// in (project, region) covers the supplied subnet on the supplied network.
//
// "Coverage" follows GCP's three nat-source modes:
//   - ALL_SUBNETWORKS_ALL_IP_RANGES         -> covers everything in the network
//   - ALL_SUBNETWORKS_ALL_PRIMARY_IP_RANGES -> covers every subnet's primary range
//   - LIST_OF_SUBNETWORKS                   -> covers only listed subnets
//
// We do NOT verify that the NAT actually has IPs allocated -- if the user
// provisioned a Cloud NAT, we assume Google's auto-allocation will handle IPs.
// Manually-misconfigured NATs with zero IPs are unusual enough that catching
// them isn't worth the false-positive risk against AUTO_ONLY NATs.
func (s *b) computeNATCoverage(ctx context.Context, log *logger.Logger, region, networkSelfLink, subnetSelfLink string) (bool, error) {
	cli, err := connect.GetClient(s.credentials, log.WithPrefix("AUTH: "))
	if err != nil {
		return false, fmt.Errorf("auth: %w", err)
	}
	defer cli.CloseIdleConnections()
	client, err := compute.NewRoutersRESTClient(ctx, option.WithHTTPClient(cli))
	if err != nil {
		return false, fmt.Errorf("routers client: %w", err)
	}
	defer client.Close()

	targetNetwork := getValueFromURL(networkSelfLink)
	targetSubnet := getValueFromURL(subnetSelfLink)

	it := client.List(ctx, &computepb.ListRoutersRequest{
		Project: s.credentials.Project,
		Region:  region,
	})
	for {
		r, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return false, fmt.Errorf("list routers: %w", err)
		}
		if getValueFromURL(r.GetNetwork()) != targetNetwork {
			continue
		}
		for _, n := range r.GetNats() {
			mode := n.GetSourceSubnetworkIpRangesToNat()
			if mode == "ALL_SUBNETWORKS_ALL_IP_RANGES" || mode == "ALL_SUBNETWORKS_ALL_PRIMARY_IP_RANGES" {
				return true, nil
			}
			if mode == "LIST_OF_SUBNETWORKS" {
				for _, sn := range n.GetSubnetworks() {
					if getValueFromURL(sn.GetName()) == targetSubnet {
						return true, nil
					}
				}
			}
		}
	}
	return false, nil
}
