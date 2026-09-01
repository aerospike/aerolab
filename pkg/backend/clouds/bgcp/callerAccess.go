package bgcp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	compute "cloud.google.com/go/compute/apiv1"
	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/aerospike-community/aerolab/pkg/backend/backends"
	"github.com/aerospike-community/aerolab/pkg/backend/clouds/bgcp/connect"
	"github.com/lithammer/shortuuid"
	"google.golang.org/api/option"
)

// gcpMaxNameLen is the GCP limit on a resource name.
const gcpMaxNameLen = 63

// unroutableCidr stands in for the caller's address when AeroLab cannot work
// out what it is. A GCP ingress rule listing no source at all allows the whole
// internet, so the rule is pointed instead at an address range reserved for
// documentation (RFC 5737) which no real traffic can arrive from. The rule is
// re-pointed at the caller as soon as their address becomes known.
const unroutableCidr = "192.0.2.1/32"

// DefaultFirewallName is the per-user firewall rule AeroLab applies to every
// instance the user works with. Each user gets their own rule per VPC, so
// several people can share one GCP project without opening each other's
// instances to the internet.
func DefaultFirewallName(owner string, vpcName string) string {
	owner = SanitizeOwner(owner)
	if owner == "" {
		return LegacyDefaultFirewallName(vpcName)
	}
	return firewallName(TAG_FIREWALL_NAME_PREFIX_OWNER, owner, vpcName)
}

// DefaultInternalFirewallName is the per-user rule which lets a user's own
// instances talk to each other on every port.
func DefaultInternalFirewallName(owner string, vpcName string) string {
	owner = SanitizeOwner(owner)
	if owner == "" {
		return LegacyDefaultInternalFirewallName(vpcName)
	}
	return firewallName(TAG_FIREWALL_NAME_PREFIX_OWNER_INTERNAL, owner, vpcName)
}

// AGIFirewallName is the per-user rule serving AGI's web ports.
func AGIFirewallName(owner string, vpcName string) string {
	owner = SanitizeOwner(owner)
	if owner == "" {
		return LegacyAGIFirewallName(vpcName)
	}
	return firewallName(TAG_FIREWALL_NAME_PREFIX_OWNER_AGI, owner, vpcName)
}

// LegacyDefaultFirewallName is the shared, pre-per-user rule name. It is still
// recognised so instances created by older AeroLab versions keep working.
func LegacyDefaultFirewallName(vpcName string) string {
	return sanitize(TAG_FIREWALL_NAME_PREFIX+vpcName, false)
}

// LegacyDefaultInternalFirewallName is the shared, pre-per-user internal rule.
func LegacyDefaultInternalFirewallName(vpcName string) string {
	return sanitize(TAG_FIREWALL_NAME_PREFIX_INTERNAL+vpcName, false)
}

// LegacyAGIFirewallName is the shared, pre-per-user AGI rule.
func LegacyAGIFirewallName(vpcName string) string {
	return sanitize(TAG_FIREWALL_NAME_PREFIX_AGI+vpcName, false)
}

// SanitizeOwner strips anything from a username that GCP would reject in a
// resource name.
func SanitizeOwner(owner string) string {
	var out strings.Builder
	for _, r := range strings.ToLower(owner) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			out.WriteRune(r)
		}
	}
	return strings.Trim(out.String(), "-")
}

// firewallName builds a rule name within GCP's 63 character limit. Names that
// would overflow keep their prefix and gain a hash of the full name, so they
// stay both unique and derivable.
func firewallName(prefix string, owner string, vpcName string) string {
	full := sanitize(prefix+owner+"-"+vpcName, false)
	// sanitize() silently truncates at GCP's limit, which would let two long
	// network names collapse onto a single shared rule. A name that reaches
	// the limit may have been truncated, so hash it rather than risk it.
	if len(full) < gcpMaxNameLen {
		return full
	}
	sum := sha256.Sum256([]byte(prefix + owner + "-" + vpcName))
	suffix := "-" + hex.EncodeToString(sum[:])[:8]
	return sanitize(full[:gcpMaxNameLen-len(suffix)], false) + suffix
}

// callerCidrs resolves the source addresses the caller's firewalls should
// allow, or nil when no identity has been configured.
func (s *b) callerCidrs() ([]string, error) {
	return s.identity.Cidrs()
}

// findDefaultFirewall locates the caller's own rule for a VPC. The lookup is
// by metadata so a renamed rule is still found, falling back to the
// deterministic name for rules created before the metadata existed.
func (s *b) findDefaultFirewall(owner string, vpcName string) *backends.Firewall {
	owner = SanitizeOwner(owner)
	name := DefaultFirewallName(owner, vpcName)
	var byName *backends.Firewall
	for _, fw := range s.firewalls {
		if fw.Network == nil || fw.Network.Name != vpcName {
			continue
		}
		if fw.Tags[TAG_FIREWALL_ROLE] == backends.FirewallRoleDefault && SanitizeOwner(fw.Owner) == owner {
			return fw
		}
		if fw.Name == name {
			byName = fw
		}
	}
	return byName
}

// ensureDefaultFirewall returns the caller's own firewall rule for the given
// VPC, creating it if it does not exist yet.
func (s *b) ensureDefaultFirewall(owner string, vpc *backends.Network, cidrs []string, waitDur time.Duration) (*backends.Firewall, error) {
	owner = SanitizeOwner(owner)
	if owner == "" {
		return nil, errors.New("cannot determine the owner for the default firewall rule")
	}
	s.defaultFWCreateLock.Lock()
	defer s.defaultFWCreateLock.Unlock()

	if fw := s.findDefaultFirewall(owner, vpc.Name); fw != nil {
		return fw, nil
	}

	name := DefaultFirewallName(owner, vpc.Name)
	if len(cidrs) == 0 {
		cidrs = []string{unroutableCidr}
	}
	ports := []*backends.Port{}
	for _, cidr := range cidrs {
		ports = append(ports, &backends.Port{
			FromPort:   -1,
			ToPort:     -1,
			SourceCidr: cidr,
			Protocol:   backends.ProtocolAll,
		})
	}
	out, err := s.CreateFirewall(&backends.CreateFirewallInput{
		BackendType: backends.BackendTypeGCP,
		Name:        name,
		Description: "AeroLab default firewall for " + owner,
		Owner:       owner,
		Tags: map[string]string{
			TAG_FIREWALL_ROLE: backends.FirewallRoleDefault,
			TAG_CALLER_LOCKED: "true",
		},
		Ports:   ports,
		Network: vpc,
	}, waitDur)
	if err != nil {
		if !strings.Contains(err.Error(), "already exists") && !strings.Contains(err.Error(), "alreadyExists") {
			return nil, err
		}
		// Somebody else created it between our lookup and our create.
		if _, err := s.GetFirewalls(s.networks); err != nil {
			return nil, err
		}
		fw := s.findDefaultFirewall(owner, vpc.Name)
		if fw == nil {
			return nil, fmt.Errorf("firewall rule %s exists but could not be read back", name)
		}
		return fw, nil
	}
	s.firewalls = append(s.firewalls, out.Firewall)
	return out.Firewall, nil
}

// ensureCallerFirewalls returns the names of the caller's own firewall rules
// for a VPC - the externally reachable one locked to the caller's address, and
// the internal one which lets the caller's instances talk to each other -
// creating and reconciling them as needed.
//
// Failing to work out the caller's address is not fatal: the rules are still
// created so the cluster comes up, and the user is told how to open access by
// hand. That is a better outcome than either aborting an expensive create or
// falling back to allowing the world.
func (s *b) ensureCallerFirewalls(log logWarner, owner string, vpc *backends.Network, waitDur time.Duration) ([]string, error) {
	cidrs, err := s.callerCidrs()
	if err != nil {
		log.Warn("Could not determine your public address, so your firewall rule allows no inbound access yet: %s", err)
		log.Warn("Open access with: aerolab config gcp lock-firewall-rules -n %s -i <your-cidr>", DefaultFirewallName(owner, vpc.Name))
		cidrs = nil
	}
	fw, err := s.ensureDefaultFirewall(owner, vpc, cidrs, waitDur)
	if err != nil {
		return nil, err
	}
	if len(cidrs) > 0 {
		if err := s.reconcileCallerIngress(fw, cidrs); err != nil {
			return nil, err
		}
	}
	internal, err := s.ensureInternalFirewall(owner, vpc, waitDur)
	if err != nil {
		return nil, err
	}
	return []string{fw.Name, internal.Name}, nil
}

// ensureInternalFirewall returns the caller's rule which allows their own
// instances to reach each other on every port.
func (s *b) ensureInternalFirewall(owner string, vpc *backends.Network, waitDur time.Duration) (*backends.Firewall, error) {
	owner = SanitizeOwner(owner)
	if owner == "" {
		return nil, errors.New("cannot determine the owner for the internal firewall rule")
	}
	s.defaultFWCreateLock.Lock()
	defer s.defaultFWCreateLock.Unlock()

	name := DefaultInternalFirewallName(owner, vpc.Name)
	if existing := s.firewalls.WithName(name).Describe(); len(existing) > 0 {
		return existing[0], nil
	}
	out, err := s.CreateFirewall(&backends.CreateFirewallInput{
		BackendType: backends.BackendTypeGCP,
		Name:        name,
		Description: "AeroLab internal firewall for " + owner,
		Owner:       owner,
		Tags: map[string]string{
			TAG_FIREWALL_ROLE: backends.FirewallRoleInternal,
		},
		Ports: []*backends.Port{
			{
				FromPort:   -1,
				ToPort:     -1,
				SourceCidr: "",
				SourceId:   "self",
				Protocol:   backends.ProtocolAll,
			},
		},
		Network: vpc,
	}, waitDur)
	if err != nil {
		if !strings.Contains(err.Error(), "already exists") && !strings.Contains(err.Error(), "alreadyExists") {
			return nil, err
		}
		if _, err := s.GetFirewalls(s.networks); err != nil {
			return nil, err
		}
		existing := s.firewalls.WithName(name).Describe()
		if len(existing) == 0 {
			return nil, fmt.Errorf("firewall rule %s exists but could not be read back", name)
		}
		return existing[0], nil
	}
	s.firewalls = append(s.firewalls, out.Firewall)
	return out.Firewall, nil
}

// warnOnOpenLegacyFirewall points out a pre-per-user rule which still allows
// the whole internet in, since it now has to be dealt with by hand.
func (s *b) warnOnOpenLegacyFirewall(log logWarner, vpcName string) {
	name := LegacyDefaultFirewallName(vpcName)
	for _, fw := range s.firewalls.WithName(name).Describe() {
		for _, port := range fw.Ports {
			if port.SourceCidr != backends.AnyIPv4Cidr {
				continue
			}
			log.Warn("Firewall rule %s still allows %s in on port %d; it is no longer used for new instances. Restrict it with: aerolab config gcp lock-firewall-rules -n %s", fw.Name, port.SourceCidr, port.FromPort, fw.Name)
			return
		}
	}
}

// ownedCallerCidrs returns the source ranges currently on a caller-locked
// rule. A GCP rule carries a single source range list which AeroLab owns
// outright, so every range on it is ours to reconcile.
func ownedCallerCidrs(fw *backends.Firewall) []string {
	cidrs := []string{}
	for _, port := range fw.Ports {
		if port.SourceCidr == "" || slices.Contains(cidrs, port.SourceCidr) {
			continue
		}
		cidrs = append(cidrs, port.SourceCidr)
	}
	return cidrs
}

// callerAllowsAllProtocols reports whether every CIDR-sourced rule on the
// firewall already opens every protocol, matching CallerAllPorts.
func callerAllowsAllProtocols(fw *backends.Firewall) bool {
	sawCIDR := false
	for _, port := range fw.Ports {
		if port.SourceCidr == "" {
			continue
		}
		sawCIDR = true
		rule := backends.CanonicalCallerRule(backends.CallerRule{
			Protocol: port.Protocol,
			FromPort: port.FromPort,
			ToPort:   port.ToPort,
		})
		if rule.Protocol != backends.ProtocolAll || rule.FromPort != -1 {
			return false
		}
	}
	return sawCIDR
}

// reconcileCallerIngress makes a caller-locked rule allow exactly the given
// source addresses on every protocol. GCP has no per-range annotation, so the
// whole source range list of the rule is replaced; only rules AeroLab created
// for a single user are ever passed in here. Allowed is patched as well so a
// rule created as SSH-only is widened on the next autolock.
func (s *b) reconcileCallerIngress(fw *backends.Firewall, cidrs []string) error {
	existing := ownedCallerCidrs(fw)
	needsCidrs := !equalStringSets(existing, cidrs)
	needsPorts := !callerAllowsAllProtocols(fw)
	if !needsCidrs && !needsPorts {
		return nil
	}
	log := s.log.WithPrefix("reconcileCallerIngress: job=" + shortuuid.New() + " ")
	if needsPorts && needsCidrs {
		log.Info("Allowing all ports from %s into %s (was %s on a narrower port set)", strings.Join(cidrs, ", "), fw.Name, strings.Join(existing, ", "))
	} else if needsPorts {
		log.Info("Widening %s to all ports from %s", fw.Name, strings.Join(cidrs, ", "))
	} else {
		log.Info("Allowing %s into %s (was %s)", strings.Join(cidrs, ", "), fw.Name, strings.Join(existing, ", "))
	}
	cli, err := connect.GetClient(s.credentials, log.WithPrefix("AUTH: "))
	if err != nil {
		return err
	}
	defer cli.CloseIdleConnections()
	ctx := context.Background()
	client, err := compute.NewFirewallsRESTClient(ctx, option.WithHTTPClient(cli))
	if err != nil {
		return err
	}
	defer client.Close()
	defer s.invalidateCacheFunc(backends.CacheInvalidateFirewall) //nolint:errcheck
	all := "all"
	op, err := client.Patch(ctx, &computepb.PatchFirewallRequest{
		Firewall: fw.Name,
		Project:  s.credentials.Project,
		FirewallResource: &computepb.Firewall{
			Name:         &fw.Name,
			SourceRanges: cidrs,
			Allowed: []*computepb.Allowed{
				{IPProtocol: &all},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("could not update caller access on %s: %w", fw.Name, err)
	}
	waitCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	if err := op.Wait(waitCtx); err != nil {
		return fmt.Errorf("could not update caller access on %s: %w", fw.Name, err)
	}
	applyCallerAllPortsToInventory(fw, cidrs)
	return nil
}

// applyCallerAllPortsToInventory mirrors an applied change onto the cached
// firewall, so a second call in the same process does not repeat the work.
func applyCallerAllPortsToInventory(fw *backends.Firewall, cidrs []string) {
	ports := backends.PortsOut{}
	for _, port := range fw.Ports {
		if port.SourceCidr == "" {
			ports = append(ports, port)
		}
	}
	for _, cidr := range cidrs {
		ports = append(ports, &backends.PortOut{
			Port: backends.Port{
				FromPort:   -1,
				ToPort:     -1,
				SourceCidr: cidr,
				Protocol:   backends.ProtocolAll,
			},
		})
	}
	fw.Ports = ports
}

func equalStringSets(a []string, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for _, item := range a {
		if !slices.Contains(b, item) {
			return false
		}
	}
	return true
}

// EnsureCallerAccess makes sure the caller can reach the given instances: the
// caller's own firewall rule exists, allows their current address, and its
// network tag is on every instance in the list. Tags belonging to other people
// are never removed, so working on a colleague's cluster adds access rather
// than taking theirs away.
//
// Every step is best-effort: a failure is logged and the caller's original
// command still runs.
func (s *b) EnsureCallerAccess(instances backends.InstanceList) {
	if !s.identity.AutolockEnabled() {
		return
	}
	instances = instances.WithBackendType(backends.BackendTypeGCP).Describe()
	if len(instances) == 0 {
		return
	}
	log := s.log.WithPrefix("EnsureCallerAccess: job=" + shortuuid.New() + " ")
	cidrs, err := s.callerCidrs()
	if err != nil {
		log.Warn("Could not determine the caller address, leaving firewalls untouched: %s", err)
		return
	}
	if len(cidrs) == 0 {
		return
	}
	s.callerAccessLock.Lock()
	defer s.callerAccessLock.Unlock()
	if s.callerAccessReady == nil {
		s.callerAccessReady = make(map[string]bool)
	}

	// Group the instances by VPC: a firewall rule only applies inside the
	// network it was created in.
	byVpc := make(map[string]backends.InstanceList)
	for _, instance := range instances {
		vpcID := instanceNetworkID(instance)
		if vpcID == "" {
			continue
		}
		byVpc[vpcID] = append(byVpc[vpcID], instance)
	}

	for vpcID, vpcInstances := range byVpc {
		vpcs := s.networks.WithNetID(vpcID).Describe()
		if len(vpcs) == 0 {
			log.Detail("Network %s is not in the inventory, skipping firewall check", vpcID)
			continue
		}
		vpc := vpcs[0]
		var names []string
		if !s.callerAccessReady[vpcID] {
			var err error
			names, err = s.ensureCallerFirewalls(log, s.identity.Owner, vpc, time.Minute)
			if err != nil {
				log.Warn("Could not prepare your firewall rules in %s: %s", vpc.Name, err)
				continue
			}
			s.callerAccessReady[vpcID] = true
		} else {
			names = []string{
				DefaultFirewallName(s.identity.Owner, vpc.Name),
				DefaultInternalFirewallName(s.identity.Owner, vpc.Name),
			}
		}
		for _, name := range names {
			fws := s.firewalls.WithName(name).Describe()
			if len(fws) == 0 {
				continue
			}
			s.attachCallerFirewall(log, fws[0], vpcInstances)
		}
	}
}

// attachCallerFirewall adds the caller's network tag to any instance which
// does not already carry it, including instances someone else created.
func (s *b) attachCallerFirewall(log logWarner, fw *backends.Firewall, instances backends.InstanceList) {
	missing := backends.InstanceList{}
	for _, instance := range instances {
		if instance.InstanceState == backends.LifeCycleStateTerminated {
			continue
		}
		if !slices.Contains(getInstanceDetail(instance).FirewallTags, fw.Name) {
			missing = append(missing, instance)
		}
	}
	if len(missing) == 0 {
		return
	}
	log.Info("Attaching your firewall rule %s to %d instance(s) created by someone else", fw.Name, len(missing))
	if err := s.InstancesAssignFirewalls(missing, backends.FirewallList{fw}); err != nil {
		log.Warn("Could not attach your firewall rule %s: %s", fw.Name, err)
		return
	}
	for _, instance := range missing {
		detail := getInstanceDetail(instance)
		detail.FirewallTags = append(detail.FirewallTags, fw.Name)
		instance.Firewalls = append(instance.Firewalls, fw.Name)
	}
}

// logWarner is the slice of the logger this file needs.
type logWarner interface {
	Info(format string, args ...any)
	Warn(format string, args ...any)
	Detail(format string, args ...any)
}

// instanceNetworkID returns the network an instance lives in.
func instanceNetworkID(instance *backends.Instance) string {
	return getInstanceDetail(instance).NetworkID
}
