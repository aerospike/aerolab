package main

import "strings"

func (c *config) F_makeXdrClusters() (ret int64, err error) {

	ret, err = chDir(c.MakeXdrClusters.ChDir)
	if err != nil {
		return ret, err
	}

	c.log.Info("--> Deploying %s", c.MakeXdrClusters.SourceClusterName)
	c.MakeCluster.ClusterName = c.MakeXdrClusters.SourceClusterName
	c.MakeCluster.NodeCount = c.MakeXdrClusters.SourceNodeCount
	c.MakeCluster.AccessPublicKeyFilePath = c.MakeXdrClusters.AccessPublicKeyFilePath
	c.MakeCluster.AerospikeVersion = c.MakeXdrClusters.AerospikeVersion
	c.MakeCluster.AutoStartAerospike = c.MakeXdrClusters.AutoStartAerospike
	c.MakeCluster.CustomConfigFilePath = c.MakeXdrClusters.CustomConfigFilePath
	c.MakeCluster.DeployOn = c.MakeXdrClusters.DeployOn
	c.MakeCluster.DistroName = c.MakeXdrClusters.DistroName
	c.MakeCluster.DistroVersion = c.MakeXdrClusters.DistroVersion
	c.MakeCluster.FeaturesFilePath = c.MakeXdrClusters.FeaturesFilePath
	c.MakeCluster.HeartbeatMode = "mesh"
	c.MakeCluster.RemoteHost = c.MakeXdrClusters.RemoteHost
	c.MakeCluster.Username = c.MakeXdrClusters.Username
	c.MakeCluster.Password = c.MakeXdrClusters.Password
	c.MakeCluster.Privileged = c.MakeXdrClusters.Privileged
	ret, err = c.F_makeCluster()
	if err != nil {
		return
	}

	for _, destination := range strings.Split(c.MakeXdrClusters.DestinationClusterNames, ",") {
		c.log.Info("--> Deploying %s", destination)
		c.MakeCluster.ClusterName = destination
		c.MakeCluster.NodeCount = c.MakeXdrClusters.DestinationNodeCount
		c.MakeCluster.AccessPublicKeyFilePath = c.MakeXdrClusters.AccessPublicKeyFilePath
		c.MakeCluster.AerospikeVersion = c.MakeXdrClusters.AerospikeVersion
		c.MakeCluster.AutoStartAerospike = c.MakeXdrClusters.AutoStartAerospike
		c.MakeCluster.CustomConfigFilePath = c.MakeXdrClusters.CustomConfigFilePath
		c.MakeCluster.DeployOn = c.MakeXdrClusters.DeployOn
		c.MakeCluster.DistroName = c.MakeXdrClusters.DistroName
		c.MakeCluster.DistroVersion = c.MakeXdrClusters.DistroVersion
		c.MakeCluster.FeaturesFilePath = c.MakeXdrClusters.FeaturesFilePath
		c.MakeCluster.HeartbeatMode = "mesh"
		c.MakeCluster.RemoteHost = c.MakeXdrClusters.RemoteHost
		c.MakeCluster.Username = c.MakeXdrClusters.Username
		c.MakeCluster.Password = c.MakeXdrClusters.Password
		c.MakeCluster.Privileged = c.MakeXdrClusters.Privileged
		ret, err = c.F_makeCluster()
		if err != nil {
			return
		}
	}

	c.log.Info("--> xdrConnect running")
	c.XdrConnect.SourceClusterName = c.MakeXdrClusters.SourceClusterName
	c.XdrConnect.DestinationClusterNames = c.MakeXdrClusters.DestinationClusterNames
	c.XdrConnect.Namespaces = c.MakeXdrClusters.Namespaces
	c.XdrConnect.RemoteHost = c.MakeXdrClusters.RemoteHost
	c.XdrConnect.DeployOn = c.MakeXdrClusters.DeployOn
	c.XdrConnect.AccessPublicKeyFilePath = c.MakeXdrClusters.AccessPublicKeyFilePath
	c.XdrConnect.Restart = c.MakeXdrClusters.Restart
	c.XdrConnect.Version = "5"
	if strings.HasPrefix(c.MakeXdrClusters.AerospikeVersion, "3.") || strings.HasPrefix(c.MakeXdrClusters.AerospikeVersion, "4.") {
		c.XdrConnect.Version = "4"
	}
	ret, err = c.F_xdrConnect()
	return
}
