package cmd

import "strings"

type NetUnblockCmd struct {
	NetBlockCmd
}

func (c *NetUnblockCmd) Execute(args []string) error {
	cmd := []string{"net", "unblock"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	err = c.blockUnblock(system, system.Backend.GetInventory(), system.Logger, args, "unblock", "-D")
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}
