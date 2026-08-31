package baws

import (
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/citrusleaf/aerolab/pkg/backend/backends"
)

func TestSanitizeOwner(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"rglonek", "rglonek"},
		{"R.Glonek", "rglonek"},
		{"first.last@aerospike.com", "firstlastaerospikecom"},
		{"CI-Runner_7", "ci-runner7"},
		{"", ""},
		{"!!!", ""},
	}
	for _, c := range cases {
		if got := SanitizeOwner(c.in); got != c.want {
			t.Errorf("SanitizeOwner(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestDefaultFirewallName(t *testing.T) {
	got := DefaultFirewallName("default", "R.Glonek", "vpc-123")
	want := TAG_FIREWALL_NAME_PREFIX + "default_rglonek_vpc-123"
	if got != want {
		t.Errorf("DefaultFirewallName = %q, want %q", got, want)
	}

	// With no owner to key on, the shared name older versions used is the only
	// name we can produce.
	got = DefaultFirewallName("default", "!!!", "vpc-123")
	want = LegacyDefaultFirewallName("default", "vpc-123")
	if got != want {
		t.Errorf("DefaultFirewallName without an owner = %q, want the legacy name %q", got, want)
	}

	long := DefaultFirewallName(strings.Repeat("p", 400), "owner", "vpc-123")
	if len(long) != awsMaxSecurityGroupNameLen {
		t.Errorf("DefaultFirewallName length = %d, want it truncated to %d", len(long), awsMaxSecurityGroupNameLen)
	}
	if !strings.HasSuffix(long, "vpc-123") {
		t.Errorf("truncated name %q lost the VPC id", long[len(long)-20:])
	}
}

// callerPort builds a firewall port as GetFirewalls would return it, marked as
// AeroLab's when description says so.
func callerPort(protocol string, from int, to int, cidr string, description string) *backends.PortOut {
	ipRange := types.IpRange{CidrIp: aws.String(cidr)}
	if description != "" {
		ipRange.Description = aws.String(description)
	}
	return &backends.PortOut{
		Port: backends.Port{
			FromPort:   from,
			ToPort:     to,
			SourceCidr: cidr,
			Protocol:   protocol,
		},
		BackendSpecific: types.IpPermission{
			FromPort:   aws.Int32(int32(from)),
			ToPort:     aws.Int32(int32(to)),
			IpProtocol: aws.String(protocol),
			IpRanges:   []types.IpRange{ipRange},
		},
	}
}

func TestOwnedCallerRulesIgnoresRulesAeroLabDidNotAdd(t *testing.T) {
	fw := &backends.Firewall{
		Ports: backends.PortsOut{
			callerPort(backends.ProtocolTCP, 22, 22, "203.0.113.7/32", backends.CallerRuleDescription),
			callerPort(backends.ProtocolTCP, 22, 22, "198.51.100.0/24", "opened by hand"),
			callerPort(backends.ProtocolTCP, 3000, 3002, "0.0.0.0/0", ""),
			{Port: backends.Port{FromPort: 443, ToPort: 443, Protocol: backends.ProtocolTCP}},
		},
	}
	owned := ownedCallerRules(fw)
	if len(owned) != 1 {
		t.Fatalf("ownedCallerRules returned %d rules, want only the marked one: %v", len(owned), owned)
	}
	want := backends.CallerRule{Protocol: backends.ProtocolTCP, FromPort: 22, ToPort: 22, Cidr: "203.0.113.7/32"}
	if owned[0] != want {
		t.Errorf("ownedCallerRules = %v, want %v", owned[0], want)
	}
}

func TestCallerIngressDiffOnAddressChange(t *testing.T) {
	fw := &backends.Firewall{
		Ports: backends.PortsOut{
			callerPort(backends.ProtocolAll, -1, -1, "203.0.113.7/32", backends.CallerRuleDescription),
			callerPort(backends.ProtocolTCP, 22, 22, "198.51.100.0/24", "opened by hand"),
		},
	}
	desired := backends.BuildCallerRules(backends.CallerAllPorts(), []string{"192.0.2.9/32"})
	add, remove := backends.DiffCallerRules(ownedCallerRules(fw), desired)
	if len(add) != 1 || add[0].Cidr != "192.0.2.9/32" {
		t.Errorf("add = %v, want the new address", add)
	}
	if len(remove) != 1 || remove[0].Cidr != "203.0.113.7/32" {
		t.Errorf("remove = %v, want only the address AeroLab previously added", remove)
	}

	applyCallerRulesToInventory(fw, add, remove)
	owned := ownedCallerRules(fw)
	if len(owned) != 1 || owned[0].Cidr != "192.0.2.9/32" {
		t.Errorf("after applying, owned rules = %v, want just the new address", owned)
	}
	// The hand-added rule has to survive the swap.
	found := false
	for _, port := range fw.Ports {
		if port.SourceCidr == "198.51.100.0/24" {
			found = true
		}
	}
	if !found {
		t.Error("reconciliation dropped a rule AeroLab did not add")
	}
}

func TestCallerIngressDiffWidensSSHToAllPorts(t *testing.T) {
	fw := &backends.Firewall{
		Ports: backends.PortsOut{
			callerPort(backends.ProtocolTCP, 22, 22, "203.0.113.7/32", backends.CallerRuleDescription),
			callerPort(backends.ProtocolAll, -1, -1, "", ""),
		},
	}
	desired := backends.BuildCallerRules(backends.CallerAllPorts(), []string{"203.0.113.7/32"})
	add, remove := backends.DiffCallerRules(ownedCallerRules(fw), desired)
	if len(add) != 1 || add[0].Protocol != backends.ProtocolAll {
		t.Errorf("add = %v, want an all-ports rule", add)
	}
	if len(remove) != 1 || remove[0].FromPort != 22 {
		t.Errorf("remove = %v, want the old SSH rule", remove)
	}
}

func TestCallerIngressDiffIsEmptyWhenAddressUnchanged(t *testing.T) {
	fw := &backends.Firewall{
		Ports: backends.PortsOut{
			callerPort(backends.ProtocolAll, -1, -1, "203.0.113.7/32", backends.CallerRuleDescription),
		},
	}
	desired := backends.BuildCallerRules(backends.CallerAllPorts(), []string{"203.0.113.7/32"})
	add, remove := backends.DiffCallerRules(ownedCallerRules(fw), desired)
	if len(add) != 0 || len(remove) != 0 {
		t.Errorf("an unchanged address should need no API calls, got add=%v remove=%v", add, remove)
	}
}

func TestHasSelfAllIngress(t *testing.T) {
	fw := fwFixture("sg-1", "mine", "rglonek", backends.FirewallRoleDefault, "vpc-1")
	if hasSelfAllIngress(fw) {
		t.Error("empty group should not report self-ingress")
	}
	fw.Ports = backends.PortsOut{
		{Port: backends.Port{FromPort: -1, ToPort: -1, SourceId: "self", Protocol: backends.ProtocolAll}},
	}
	if !hasSelfAllIngress(fw) {
		t.Error("create-time SourceId=self should count")
	}
	fw.Ports = backends.PortsOut{
		{Port: backends.Port{FromPort: -1, ToPort: -1, SourceId: "sg-1", Protocol: backends.ProtocolAll}},
	}
	if !hasSelfAllIngress(fw) {
		t.Error("SourceId matching the group ID should count")
	}
	fw.Ports = backends.PortsOut{
		callerPort(backends.ProtocolTCP, 22, 22, "203.0.113.7/32", backends.CallerRuleDescription),
	}
	if hasSelfAllIngress(fw) {
		t.Error("a CIDR SSH rule is not self-ingress")
	}
}

// fwFixture builds a security group as the inventory would hold it.
func fwFixture(id string, name string, owner string, role string, vpcID string) *backends.Firewall {
	fw := &backends.Firewall{
		FirewallID: id,
		Name:       name,
		Owner:      owner,
		Network:    &backends.Network{NetworkId: vpcID},
		Tags:       map[string]string{},
	}
	if role != "" {
		fw.Tags[TAG_FIREWALL_ROLE] = role
	}
	return fw
}

func TestFindDefaultFirewall(t *testing.T) {
	mine := fwFixture("sg-1", "renamed-by-hand", "R.Glonek", backends.FirewallRoleDefault, "vpc-1")
	theirs := fwFixture("sg-2", DefaultFirewallName("default", "someone", "vpc-1"), "someone", backends.FirewallRoleDefault, "vpc-1")
	otherVpc := fwFixture("sg-3", "elsewhere", "rglonek", backends.FirewallRoleDefault, "vpc-2")
	untagged := fwFixture("sg-4", DefaultFirewallName("default", "notag", "vpc-1"), "", "", "vpc-1")
	s := &b{project: "default", firewalls: backends.FirewallList{theirs, otherVpc, untagged, mine}}

	if got := s.findDefaultFirewall("rglonek", "vpc-1"); got != mine {
		t.Errorf("findDefaultFirewall matched %v, want the group tagged with my name even though it was renamed", got)
	}
	// A group predating the role tag is still found by its deterministic name.
	if got := s.findDefaultFirewall("notag", "vpc-1"); got != untagged {
		t.Errorf("findDefaultFirewall matched %v, want the group found by name", got)
	}
	if got := s.findDefaultFirewall("nobody", "vpc-1"); got != nil {
		t.Errorf("findDefaultFirewall matched %v, want no match", got)
	}
}

// instFixture builds an instance as the inventory would hold it.
func instFixture(name string, owner string, vpcID string, firewallIDs ...string) *backends.Instance {
	return &backends.Instance{
		Name:            name,
		Owner:           owner,
		Firewalls:       firewallIDs,
		BackendSpecific: &InstanceDetail{NetworkID: vpcID},
	}
}

func TestDefaultFirewallForInstance(t *testing.T) {
	mine := fwFixture("sg-1", DefaultFirewallName("default", "rglonek", "vpc-1"), "rglonek", backends.FirewallRoleDefault, "vpc-1")
	theirs := fwFixture("sg-2", DefaultFirewallName("default", "someone", "vpc-1"), "someone", backends.FirewallRoleDefault, "vpc-1")
	legacy := fwFixture("sg-3", LegacyDefaultFirewallName("default", "vpc-1"), "", "", "vpc-1")
	s := &b{project: "default", firewalls: backends.FirewallList{mine, theirs, legacy}}

	// An instance carrying both groups mounts EFS through its owner's.
	got, err := s.defaultFirewallForInstance(instFixture("node1", "rglonek", "vpc-1", "sg-2", "sg-1"))
	if err != nil || got != mine {
		t.Errorf("defaultFirewallForInstance = (%v, %v), want my own group", got, err)
	}

	// Working on somebody else's node, the group actually attached wins over
	// the one my name would resolve to.
	got, err = s.defaultFirewallForInstance(instFixture("node2", "unknown-person", "vpc-1", "sg-2"))
	if err != nil || got != theirs {
		t.Errorf("defaultFirewallForInstance = (%v, %v), want the attached group", got, err)
	}

	// Nothing attached and no per-user group: an older shared group still works.
	got, err = s.defaultFirewallForInstance(instFixture("node3", "nobody", "vpc-1"))
	if err != nil || got != legacy {
		t.Errorf("defaultFirewallForInstance = (%v, %v), want the legacy group", got, err)
	}

	if _, err := s.defaultFirewallForInstance(instFixture("node4", "rglonek", "")); err == nil {
		t.Error("defaultFirewallForInstance should refuse an instance with no VPC")
	}
	if _, err := s.defaultFirewallForInstance(instFixture("node5", "rglonek", "vpc-9")); err == nil {
		t.Error("defaultFirewallForInstance should refuse a VPC with no AeroLab group")
	}
}
