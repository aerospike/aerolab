package bgcp

import (
	"strings"
	"testing"

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
		{"-leading-and-trailing-", "leading-and-trailing"},
		{"", ""},
		{"!!!", ""},
	}
	for _, c := range cases {
		if got := SanitizeOwner(c.in); got != c.want {
			t.Errorf("SanitizeOwner(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFirewallNames(t *testing.T) {
	if got, want := DefaultFirewallName("R.Glonek", "default"), "aerolab-o-rglonek-default"; got != want {
		t.Errorf("DefaultFirewallName = %q, want %q", got, want)
	}
	if got, want := DefaultInternalFirewallName("rglonek", "default"), "aerolab-oi-rglonek-default"; got != want {
		t.Errorf("DefaultInternalFirewallName = %q, want %q", got, want)
	}
	if got, want := AGIFirewallName("rglonek", "default"), "aerolab-oagi-rglonek-default"; got != want {
		t.Errorf("AGIFirewallName = %q, want %q", got, want)
	}

	// With no owner to key on, the shared name older versions used is the only
	// name we can produce.
	if got, want := DefaultFirewallName("!!!", "default"), LegacyDefaultFirewallName("default"); got != want {
		t.Errorf("DefaultFirewallName without an owner = %q, want the legacy name %q", got, want)
	}

	// The per-user names must never collide with the shared ones.
	if strings.HasPrefix(LegacyDefaultFirewallName("default"), TAG_FIREWALL_NAME_PREFIX_OWNER) {
		t.Error("the legacy name is indistinguishable from a per-user name")
	}
}

func TestFirewallNameStaysWithinGcpLimit(t *testing.T) {
	long := DefaultFirewallName("a-very-long-corporate-username-indeed", strings.Repeat("v", 60))
	if len(long) > gcpMaxNameLen {
		t.Fatalf("name is %d characters, GCP allows %d: %q", len(long), gcpMaxNameLen, long)
	}
	// Two different overlong inputs must not truncate onto the same name.
	other := DefaultFirewallName("a-very-long-corporate-username-indeed", strings.Repeat("v", 60)+"-other")
	if long == other {
		t.Errorf("two networks truncated onto the same rule name %q", long)
	}
	// The same input always produces the same name, or it could not be found
	// again on the next run.
	if again := DefaultFirewallName("a-very-long-corporate-username-indeed", strings.Repeat("v", 60)); again != long {
		t.Errorf("name is not stable: %q then %q", long, again)
	}
}

// cidrPort builds a firewall port as GetFirewalls would return it.
func cidrPort(protocol string, from int, to int, cidr string) *backends.PortOut {
	return &backends.PortOut{
		Port: backends.Port{
			FromPort:   from,
			ToPort:     to,
			SourceCidr: cidr,
			Protocol:   protocol,
		},
	}
}

func TestCallerCidrReconciliationOnAddressChange(t *testing.T) {
	fw := &backends.Firewall{
		Name: "aerolab-o-rglonek-default",
		Ports: backends.PortsOut{
			cidrPort(backends.ProtocolTCP, 22, 22, "203.0.113.7/32"),
		},
	}
	if got := ownedCallerCidrs(fw); len(got) != 1 || got[0] != "203.0.113.7/32" {
		t.Fatalf("ownedCallerCidrs = %v, want the current address", got)
	}
	if callerAllowsAllProtocols(fw) {
		t.Error("an SSH-only rule should not count as all-ports")
	}

	applyCallerAllPortsToInventory(fw, []string{"192.0.2.9/32", "198.51.100.0/24"})
	got := ownedCallerCidrs(fw)
	if len(got) != 2 || got[0] != "192.0.2.9/32" || got[1] != "198.51.100.0/24" {
		t.Errorf("after applying, source ranges = %v, want both new addresses", got)
	}
	if len(fw.Ports) != 2 {
		t.Errorf("the rule now has %d ports, want one per address", len(fw.Ports))
	}
	for _, port := range fw.Ports {
		if port.FromPort != -1 || port.ToPort != -1 || port.Protocol != backends.ProtocolAll {
			t.Errorf("port %v should have been widened to all protocols", port.Port)
		}
	}
	if !callerAllowsAllProtocols(fw) {
		t.Error("after widening, the rule should allow all protocols")
	}
}

func TestCallerCidrReconciliationIsSkippedWhenAddressUnchanged(t *testing.T) {
	fw := &backends.Firewall{
		Ports: backends.PortsOut{
			cidrPort(backends.ProtocolTCP, 22, 22, "198.51.100.0/24"),
			cidrPort(backends.ProtocolTCP, 22, 22, "203.0.113.7/32"),
		},
	}
	// Order must not matter: the rule holds a set of sources, not a sequence.
	if !equalStringSets(ownedCallerCidrs(fw), []string{"203.0.113.7/32", "198.51.100.0/24"}) {
		t.Error("an unchanged pair of addresses should need no API call")
	}
}

// fwFixture builds a firewall rule as the inventory would hold it.
func fwFixture(name string, owner string, role string, vpcName string) *backends.Firewall {
	fw := &backends.Firewall{
		FirewallID: name,
		Name:       name,
		Owner:      owner,
		Network:    &backends.Network{Name: vpcName},
		Tags:       map[string]string{},
	}
	if role != "" {
		fw.Tags[TAG_FIREWALL_ROLE] = role
	}
	return fw
}

func TestFindDefaultFirewall(t *testing.T) {
	mine := fwFixture("renamed-by-hand", "R.Glonek", backends.FirewallRoleDefault, "default")
	theirs := fwFixture(DefaultFirewallName("someone", "default"), "someone", backends.FirewallRoleDefault, "default")
	otherVpc := fwFixture(DefaultFirewallName("rglonek", "other"), "rglonek", backends.FirewallRoleDefault, "other")
	untagged := fwFixture(DefaultFirewallName("notag", "default"), "", "", "default")
	s := &b{firewalls: backends.FirewallList{theirs, otherVpc, untagged, mine}}

	if got := s.findDefaultFirewall("rglonek", "default"); got != mine {
		t.Errorf("findDefaultFirewall matched %v, want the rule tagged with my name even though it was renamed", got)
	}
	// A rule predating the role tag is still found by its deterministic name.
	if got := s.findDefaultFirewall("notag", "default"); got != untagged {
		t.Errorf("findDefaultFirewall matched %v, want the rule found by name", got)
	}
	if got := s.findDefaultFirewall("nobody", "default"); got != nil {
		t.Errorf("findDefaultFirewall matched %v, want no match", got)
	}
}
