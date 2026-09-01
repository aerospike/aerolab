package baws

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go"
	"github.com/citrusleaf/aerolab/pkg/backend/backends"
	"github.com/lithammer/shortuuid"
)

// awsMaxSecurityGroupNameLen is the EC2 limit on a security group name.
const awsMaxSecurityGroupNameLen = 255

// DefaultFirewallName is the per-user security group AeroLab attaches to every
// instance the user works with. Each user gets their own group in each VPC, so
// several people can share one AWS account without opening each other's
// instances to the internet.
func DefaultFirewallName(project string, owner string, vpcID string) string {
	owner = SanitizeOwner(owner)
	if owner == "" {
		return LegacyDefaultFirewallName(project, vpcID)
	}
	return truncateFirewallName(TAG_FIREWALL_NAME_PREFIX + project + "_" + owner + "_" + vpcID)
}

// LegacyDefaultFirewallName is the shared, pre-per-user group name. It is
// still recognised so that instances created by older AeroLab versions keep
// working.
func LegacyDefaultFirewallName(project string, vpcID string) string {
	return truncateFirewallName(TAG_FIREWALL_NAME_PREFIX + project + "_" + vpcID)
}

// AGIFirewallName is the per-user security group serving AGI's web ports.
func AGIFirewallName(project string, owner string, vpcID string) string {
	owner = SanitizeOwner(owner)
	if owner == "" {
		return LegacyAGIFirewallName(project, vpcID)
	}
	return truncateFirewallName(TAG_FIREWALL_NAME_PREFIX_AGI + project + "_" + owner + "_" + vpcID)
}

// LegacyAGIFirewallName is the shared, pre-per-user AGI group name.
func LegacyAGIFirewallName(project string, vpcID string) string {
	return truncateFirewallName(TAG_FIREWALL_NAME_PREFIX_AGI + project + "_" + vpcID)
}

// SanitizeOwner strips anything from a username that EC2 would reject in a
// security group name, and lowercases it so the same person always resolves to
// the same group.
func SanitizeOwner(owner string) string {
	var out strings.Builder
	for _, r := range strings.ToLower(owner) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			out.WriteRune(r)
		}
	}
	return out.String()
}

// truncateFirewallName keeps a name within the EC2 limit, preferring to lose
// characters from the middle so that both the prefix and the VPC id survive.
func truncateFirewallName(name string) string {
	if len(name) <= awsMaxSecurityGroupNameLen {
		return name
	}
	keep := awsMaxSecurityGroupNameLen / 2
	return name[:keep] + name[len(name)-(awsMaxSecurityGroupNameLen-keep):]
}

// callerCidrs resolves the source addresses the caller's firewalls should
// allow, or nil when no identity has been configured.
func (s *b) callerCidrs() ([]string, error) {
	return s.identity.Cidrs()
}

// findDefaultFirewall locates the caller's own group in the given VPC. The
// lookup is by tag so that a renamed group is still found, falling back to the
// deterministic name for groups created before the tag existed.
func (s *b) findDefaultFirewall(owner string, vpcID string) *backends.Firewall {
	owner = SanitizeOwner(owner)
	name := DefaultFirewallName(s.project, owner, vpcID)
	var byName *backends.Firewall
	for _, fw := range s.firewalls {
		if fw.Network == nil || fw.Network.NetworkId != vpcID {
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

// defaultFirewallForInstance resolves the AeroLab-managed group an instance
// reaches its own VPC through, which is what an EFS mount target has to accept
// NFS from. The group attached to the instance wins, since that is the one
// which actually applies, before falling back to the name the owner's group
// would have and then to the shared group older versions created.
func (s *b) defaultFirewallForInstance(instance *backends.Instance) (*backends.Firewall, error) {
	vpcID := instanceNetworkID(instance)
	if vpcID == "" {
		return nil, errors.New("cannot determine the VPC of instance " + instance.Name)
	}
	owner := SanitizeOwner(instance.Owner)
	var attachedDefault *backends.Firewall
	for _, fw := range s.firewalls {
		if fw.Network == nil || fw.Network.NetworkId != vpcID {
			continue
		}
		if !hasFirewall(instance, fw.FirewallID) {
			continue
		}
		if fw.Tags[TAG_FIREWALL_ROLE] != backends.FirewallRoleDefault {
			continue
		}
		if SanitizeOwner(fw.Owner) == owner {
			return fw, nil
		}
		if attachedDefault == nil {
			attachedDefault = fw
		}
	}
	if attachedDefault != nil {
		return attachedDefault, nil
	}
	if fw := s.findDefaultFirewall(owner, vpcID); fw != nil {
		return fw, nil
	}
	if fw := s.findLegacyDefaultFirewall(vpcID); fw != nil {
		return fw, nil
	}
	return nil, fmt.Errorf("no AeroLab security group found for instance %s in vpc %s", instance.Name, vpcID)
}

// findLegacyDefaultFirewall locates the shared group older AeroLab versions
// created for this project and VPC.
func (s *b) findLegacyDefaultFirewall(vpcID string) *backends.Firewall {
	name := LegacyDefaultFirewallName(s.project, vpcID)
	for _, fw := range s.firewalls {
		if fw.Network == nil || fw.Network.NetworkId != vpcID {
			continue
		}
		if fw.Name == name {
			return fw
		}
	}
	return nil
}

// ensureDefaultFirewall returns the caller's own security group for the given
// VPC, creating it if it does not exist yet. The group allows the caller's
// current source addresses in on every port, and allows instances that share
// the group to talk to each other on every port.
func (s *b) ensureDefaultFirewall(owner string, vpc *backends.Network, waitDur time.Duration) (*backends.Firewall, error) {
	owner = SanitizeOwner(owner)
	if owner == "" {
		return nil, errors.New("cannot determine the owner for the default security group")
	}
	s.defaultFWCreateLock.Lock()
	defer s.defaultFWCreateLock.Unlock()

	if fw := s.findDefaultFirewall(owner, vpc.NetworkId); fw != nil {
		if err := s.ensureSelfIngress(fw); err != nil {
			return nil, err
		}
		return fw, nil
	}

	name := DefaultFirewallName(s.project, owner, vpc.NetworkId)
	out, err := s.CreateFirewall(&backends.CreateFirewallInput{
		BackendType: backends.BackendTypeAWS,
		Name:        name,
		Description: "AeroLab default firewall for " + owner,
		Owner:       owner,
		Tags: map[string]string{
			TAG_FIREWALL_ROLE: backends.FirewallRoleDefault,
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
		var apiErr smithy.APIError
		if !errors.As(err, &apiErr) || apiErr.ErrorCode() != "InvalidGroup.Duplicate" {
			return nil, err
		}
		// Somebody else created it between our lookup and our create.
		if _, err := s.GetFirewalls(s.networks); err != nil {
			return nil, err
		}
		fw := s.findDefaultFirewall(owner, vpc.NetworkId)
		if fw == nil {
			return nil, fmt.Errorf("security group %s exists but could not be read back", name)
		}
		if err := s.ensureSelfIngress(fw); err != nil {
			return nil, err
		}
		return fw, nil
	}
	s.firewalls = append(s.firewalls, out.Firewall)
	return out.Firewall, nil
}

// hasSelfAllIngress reports whether the group already allows every protocol
// from its own ID (or the "self" placeholder used at create time).
func hasSelfAllIngress(fw *backends.Firewall) bool {
	for _, port := range fw.Ports {
		if port.Protocol != backends.ProtocolAll && port.Protocol != "all" {
			continue
		}
		if port.SourceId == fw.FirewallID || port.SourceId == "self" {
			return true
		}
	}
	return false
}

// ensureSelfIngress adds an all-protocols rule sourced from the group itself
// when one is missing, so instances sharing the group can reach each other.
func (s *b) ensureSelfIngress(fw *backends.Firewall) error {
	if hasSelfAllIngress(fw) {
		return nil
	}
	cli, err := getEc2Client(s.credentials, &fw.ZoneName)
	if err != nil {
		return err
	}
	perm := types.IpPermission{
		IpProtocol: aws.String(backends.ProtocolAll),
		UserIdGroupPairs: []types.UserIdGroupPair{
			{GroupId: aws.String(fw.FirewallID)},
		},
	}
	_, err = cli.AuthorizeSecurityGroupIngress(context.TODO(), &ec2.AuthorizeSecurityGroupIngressInput{
		GroupId:       aws.String(fw.FirewallID),
		IpPermissions: []types.IpPermission{perm},
	})
	if err != nil && !isDuplicateIngressRule(err) {
		return fmt.Errorf("could not authorize self-ingress on %s: %w", fw.Name, err)
	}
	defer s.invalidateCacheFunc(backends.CacheInvalidateFirewall) //nolint:errcheck
	fw.Ports = append(fw.Ports, &backends.PortOut{
		Port: backends.Port{
			FromPort: -1,
			ToPort:   -1,
			SourceId: fw.FirewallID,
			Protocol: backends.ProtocolAll,
		},
		BackendSpecific: perm,
	})
	return nil
}

// ensureCallerFirewall returns the caller's own security group for a VPC with
// its ingress brought in step with the caller's current address.
//
// Failing to work out the caller's address is not fatal: the group is still
// created with its instance-to-instance rule so the cluster comes up, and the
// user is told how to open access by hand. That is a better outcome than
// either aborting an expensive create or falling back to allowing the world.
func (s *b) ensureCallerFirewall(log logWarner, owner string, vpc *backends.Network, waitDur time.Duration) (*backends.Firewall, error) {
	fw, err := s.ensureDefaultFirewall(owner, vpc, waitDur)
	if err != nil {
		return nil, err
	}
	cidrs, err := s.callerCidrs()
	if err != nil {
		log.Warn("Could not determine your public address, so %s allows no inbound access yet: %s", fw.Name, err)
		log.Warn("Open access with: aerolab config aws lock-security-groups -n %s -i <your-cidr>", fw.Name)
		return fw, nil
	}
	if len(cidrs) == 0 {
		return fw, nil
	}
	if err := s.reconcileCallerIngress(fw, backends.CallerAllPorts(), cidrs); err != nil {
		return nil, err
	}
	return fw, nil
}

// warnOnOpenLegacyFirewall points out a pre-per-user security group which
// still allows the whole internet in, since it now has to be dealt with by
// hand.
func (s *b) warnOnOpenLegacyFirewall(log logWarner, vpcID string) {
	fw := s.findLegacyDefaultFirewall(vpcID)
	if fw == nil {
		return
	}
	for _, port := range fw.Ports {
		if port.SourceCidr != backends.AnyIPv4Cidr {
			continue
		}
		log.Warn("Security group %s still allows %s in on port %d; it is no longer used for new instances. Restrict it with: aerolab config aws lock-security-groups -n %s", fw.Name, port.SourceCidr, port.FromPort, fw.Name)
		return
	}
}

// ownedCallerRules returns the ingress rules on a firewall which AeroLab added
// on behalf of the caller. Rules added by anyone else carry no marker and are
// deliberately excluded, so reconciliation never revokes them.
func ownedCallerRules(fw *backends.Firewall) []backends.CallerRule {
	rules := []backends.CallerRule{}
	for _, port := range fw.Ports {
		perm, ok := getIpPermission(port)
		if !ok {
			continue
		}
		for _, ipRange := range perm.IpRanges {
			if aws.ToString(ipRange.Description) != backends.CallerRuleDescription {
				continue
			}
			rules = append(rules, backends.CallerRule{
				Protocol: port.Protocol,
				FromPort: port.FromPort,
				ToPort:   port.ToPort,
				Cidr:     aws.ToString(ipRange.CidrIp),
			})
		}
	}
	return rules
}

// reconcileCallerIngress makes the firewall allow exactly the given source
// CIDRs on the given ports, revoking any address AeroLab previously authorised
// for the caller which no longer applies. Rules AeroLab does not own are left
// alone.
func (s *b) reconcileCallerIngress(fw *backends.Firewall, ports []backends.CallerPort, cidrs []string) error {
	desired := backends.BuildCallerRules(ports, cidrs)
	add, remove := backends.DiffCallerRules(ownedCallerRules(fw), desired)
	if len(add) == 0 && len(remove) == 0 {
		return nil
	}
	log := s.log.WithPrefix("reconcileCallerIngress: job=" + shortuuid.New() + " ")
	cli, err := getEc2Client(s.credentials, &fw.ZoneName)
	if err != nil {
		return err
	}
	defer s.invalidateCacheFunc(backends.CacheInvalidateFirewall) //nolint:errcheck
	if len(remove) > 0 {
		log.Detail("%s: revoking %s", fw.Name, backends.DescribeCallerRules(remove))
		_, err := cli.RevokeSecurityGroupIngress(context.TODO(), &ec2.RevokeSecurityGroupIngressInput{
			GroupId:       aws.String(fw.FirewallID),
			IpPermissions: callerIpPermissions(remove),
		})
		if err != nil && !isMissingIngressRule(err) {
			return fmt.Errorf("could not revoke stale rules on %s: %w", fw.Name, err)
		}
	}
	if len(add) > 0 {
		log.Info("Allowing %s into %s", backends.DescribeCallerRules(add), fw.Name)
		_, err := cli.AuthorizeSecurityGroupIngress(context.TODO(), &ec2.AuthorizeSecurityGroupIngressInput{
			GroupId:       aws.String(fw.FirewallID),
			IpPermissions: callerIpPermissions(add),
		})
		if err != nil && !isDuplicateIngressRule(err) {
			return fmt.Errorf("could not authorize caller access on %s: %w", fw.Name, err)
		}
	}
	applyCallerRulesToInventory(fw, add, remove)
	return nil
}

// callerIpPermissions converts rules into EC2 permissions, marking each range
// so a later reconciliation knows the rule belongs to AeroLab.
func callerIpPermissions(rules []backends.CallerRule) []types.IpPermission {
	perms := make([]types.IpPermission, 0, len(rules))
	for _, rule := range rules {
		rule = backends.CanonicalCallerRule(rule)
		perm := types.IpPermission{
			IpProtocol: aws.String(rule.Protocol),
			IpRanges: []types.IpRange{
				{
					CidrIp:      aws.String(rule.Cidr),
					Description: aws.String(backends.CallerRuleDescription),
				},
			},
		}
		// Protocol -1 (all) must not carry a port range; EC2 rejects it.
		if rule.Protocol != backends.ProtocolAll {
			perm.FromPort = aws.Int32(int32(rule.FromPort))
			perm.ToPort = aws.Int32(int32(rule.ToPort))
		}
		perms = append(perms, perm)
	}
	return perms
}

// applyCallerRulesToInventory mirrors an applied change onto the cached
// firewall, so a second call in the same process does not repeat the work.
func applyCallerRulesToInventory(fw *backends.Firewall, add []backends.CallerRule, remove []backends.CallerRule) {
	kept := backends.PortsOut{}
	for _, port := range fw.Ports {
		perm, ok := getIpPermission(port)
		drop := false
		if ok {
			for _, ipRange := range perm.IpRanges {
				if aws.ToString(ipRange.Description) != backends.CallerRuleDescription {
					continue
				}
				candidate := backends.CallerRule{
					Protocol: port.Protocol,
					FromPort: port.FromPort,
					ToPort:   port.ToPort,
					Cidr:     aws.ToString(ipRange.CidrIp),
				}
				for _, removed := range remove {
					if candidate == removed {
						drop = true
					}
				}
			}
		}
		if !drop {
			kept = append(kept, port)
		}
	}
	for _, rule := range add {
		kept = append(kept, &backends.PortOut{
			Port: backends.Port{
				FromPort:   rule.FromPort,
				ToPort:     rule.ToPort,
				SourceCidr: rule.Cidr,
				Protocol:   rule.Protocol,
			},
			BackendSpecific: types.IpPermission{
				FromPort:   aws.Int32(int32(rule.FromPort)),
				ToPort:     aws.Int32(int32(rule.ToPort)),
				IpProtocol: aws.String(rule.Protocol),
				IpRanges: []types.IpRange{
					{
						CidrIp:      aws.String(rule.Cidr),
						Description: aws.String(backends.CallerRuleDescription),
					},
				},
			},
		})
	}
	fw.Ports = kept
}

// EnsureCallerAccess makes sure the caller can reach the given instances: the
// caller's own security group exists, allows their current address, and is
// attached to every instance in the list. Groups belonging to other people are
// never detached, so working on a colleague's cluster adds access rather than
// taking theirs away.
//
// Every step is best-effort. A caller who lacks permission to modify somebody
// else's instance, or who has hit the per-interface security group limit, gets
// a warning and their original command still runs.
func (s *b) EnsureCallerAccess(instances backends.InstanceList) {
	if !s.identity.AutolockEnabled() {
		return
	}
	instances = instances.WithBackendType(backends.BackendTypeAWS).Describe()
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

	// Group the instances by VPC: a security group only applies inside the VPC
	// it was created in.
	byVpc := make(map[string]backends.InstanceList)
	for _, instance := range instances {
		vpcID := instanceNetworkID(instance)
		if vpcID == "" {
			continue
		}
		byVpc[vpcID] = append(byVpc[vpcID], instance)
	}

	for vpcID, vpcInstances := range byVpc {
		vpc := s.networks.WithNetID(vpcID).Describe()
		if len(vpc) == 0 {
			log.Detail("VPC %s is not in the inventory, skipping firewall check", vpcID)
			continue
		}
		fw, err := s.ensureDefaultFirewall(s.identity.Owner, vpc[0], time.Minute)
		if err != nil {
			log.Warn("Could not prepare your security group in %s: %s", vpcID, err)
			continue
		}
		if !s.callerAccessReady[vpcID] {
			if err := s.reconcileCallerIngress(fw, backends.CallerAllPorts(), cidrs); err != nil {
				log.Warn("Could not update your security group %s: %s", fw.Name, err)
			} else {
				s.callerAccessReady[vpcID] = true
			}
		}
		s.attachCallerFirewall(log, fw, vpcInstances)
	}
}

// attachCallerFirewall adds the caller's group to any instance which does not
// already carry it, including instances someone else created.
func (s *b) attachCallerFirewall(log logWarner, fw *backends.Firewall, instances backends.InstanceList) {
	missing := backends.InstanceList{}
	for _, instance := range instances {
		if instance.InstanceState == backends.LifeCycleStateTerminated {
			continue
		}
		if !hasFirewall(instance, fw.FirewallID) {
			missing = append(missing, instance)
		}
	}
	if len(missing) == 0 {
		return
	}
	log.Info("Attaching your security group %s to %d instance(s) created by someone else", fw.Name, len(missing))
	if err := s.InstancesAssignFirewalls(missing, backends.FirewallList{fw}); err != nil {
		log.Warn("Could not attach your security group %s: %s", fw.Name, err)
		return
	}
	for _, instance := range missing {
		instance.Firewalls = append(instance.Firewalls, fw.FirewallID)
	}
}

// logWarner is the slice of the logger this file needs, kept narrow so the
// helpers can be exercised without a real logger.
type logWarner interface {
	Info(format string, args ...any)
	Warn(format string, args ...any)
	Detail(format string, args ...any)
}

// instanceNetworkID returns the VPC an instance lives in.
func instanceNetworkID(instance *backends.Instance) string {
	return getInstanceDetail(instance).NetworkID
}

func hasFirewall(instance *backends.Instance, firewallID string) bool {
	for _, existing := range instance.Firewalls {
		if existing == firewallID {
			return true
		}
	}
	return false
}

// isDuplicateIngressRule reports whether EC2 rejected an authorize call only
// because the rule was already there, which is not a failure for us.
func isDuplicateIngressRule(err error) bool {
	var apiErr smithy.APIError
	return errors.As(err, &apiErr) && apiErr.ErrorCode() == "InvalidPermission.Duplicate"
}

// isMissingIngressRule reports whether EC2 rejected a revoke call only because
// the rule had already gone.
func isMissingIngressRule(err error) bool {
	var apiErr smithy.APIError
	return errors.As(err, &apiErr) && apiErr.ErrorCode() == "InvalidPermission.NotFound"
}
