package vagrant

// ProvisioningOption is used to set whether provisioning should be forced,
// disabled, or use the default.
type ProvisioningOption uint8

const (
	// DefaultProvisioning will cause vagrant to run the provisioners only if
	// they haven't already been run.
	DefaultProvisioning ProvisioningOption = iota

	// ForceProvisioning will force the provisioners to run.
	ForceProvisioning

	// DisableProvisioning will disable provisioners.
	DisableProvisioning
)

// ProvisioningArgument adds Provisioning and Provisioners arguments to a
// Command.
type ProvisioningArgument struct {
	ProvisionersArgument

	// Enable or disable provisioning
	Provisioning ProvisioningOption
}

func (parg *ProvisioningArgument) buildArguments() []string {
	args := parg.ProvisionersArgument.buildArguments()
	if parg.Provisioning == ForceProvisioning {
		args = append(args, "--provision")
	} else if parg.Provisioning == DisableProvisioning {
		args = append(args, "--no-provision")
	}
	return args
}
