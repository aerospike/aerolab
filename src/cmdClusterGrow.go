package main

type clusterGrowCmd struct {
	clusterCreateCmd
}

func init() {
	addBackendSwitch("cluster.grow", "aws", &a.opts.Cluster.Grow.Aws)
	addBackendSwitch("cluster.grow", "docker", &a.opts.Cluster.Grow.Docker)
	addBackendSwitch("cluster.grow", "gcp", &a.opts.Cluster.Grow.Gcp)
}

func (c *clusterGrowCmd) Execute(args []string) error {
	return c.realExecute(args, true)
}
