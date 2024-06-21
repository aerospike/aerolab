package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/bestmethod/inslice"
)

type clusterStartStopDestroyCmd struct {
}

// typeName == "agi" or "cluster"
func (c *clusterStartStopDestroyCmd) getBasicData(clusterName string, Nodes string, typeName string) (cList []string, nodes map[string][]int, err error) {
	// get cluster list
	inv, err := b.Inventory("", []int{InventoryItemClusters})
	if err != nil {
		return nil, nil, err
	}
	var clusterList []string
	for _, i := range inv.Clusters {
		switch typeName {
		case "agi":
			if i.Features&ClusterFeatureAGI > 0 {
				clusterList = append(clusterList, i.ClusterName)
			}
		default:
			if i.Features&ClusterFeatureAGI <= 0 {
				clusterList = append(clusterList, i.ClusterName)
			}
		}
	}
	// check cluster exists
	if clusterName != "all" && clusterName != "ALL" {
		cList = strings.Split(clusterName, ",")
	} else {
		cList = clusterList
	}
	for _, clusterName = range cList {
		if !inslice.HasString(clusterList, clusterName) {
			err = fmt.Errorf("cluster does not exist: %s", clusterName)
			return nil, nil, err
		}
	}
	nodes = make(map[string][]int)
	var nodesC []int
	if Nodes == "" || Nodes == "all" || Nodes == "ALL" {
		for _, clusterName = range cList {
			nodesC, err = b.NodeListInCluster(clusterName)
			if err != nil {
				return nil, nil, err
			}
			nodes[clusterName] = nodesC
		}
	} else {
		for _, nodeString := range strings.Split(Nodes, ",") {
			nodeInt, err := strconv.Atoi(nodeString)
			if err != nil {
				return nil, nil, err
			}
			nodesC = append(nodesC, nodeInt)
		}
		for _, clusterName = range cList {
			nodes[clusterName] = nodesC
		}
	}
	for _, clusterName = range cList {
		if len(nodes[clusterName]) == 0 {
			err = errors.New("found 0 nodes in cluster")
			return nil, nil, err
		}
	}
	return
}
