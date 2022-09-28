package main

import (
	"errors"
	"sort"
	"strconv"
	"strings"

	"github.com/bestmethod/inslice"
)

// expands a node listing in format of:
// 1-100,-5,150 (1-100, not 5, 150) to a comma-separated listing
func (t *TypeMachines) ExpandNodes(clusterName string) error {
	a, err := expandNodes(string(*t), clusterName)
	if err != nil {
		return err
	}
	b := TypeMachines(a)
	*t = b
	return nil
}

func (t *TypeNodes) ExpandNodes(clusterName string) error {
	a, err := expandNodes(string(*t), clusterName)
	if err != nil {
		return err
	}
	b := TypeNodes(a)
	*t = b
	return nil
}

func (t *TypeNodesPlusAllOption) ExpandNodes(clusterName string) error {
	a, err := expandNodes(string(*t), clusterName)
	if err != nil {
		return err
	}
	b := TypeNodesPlusAllOption(a)
	*t = b
	return nil
}

func expandNodes(nodes string, clusterName string) (string, error) {
	list := []int{}
	if nodes == "" {
		return nodes, nil
	}
	for _, item := range strings.Split(nodes, ",") {
		if strings.ToUpper(item) == "ALL" {
			n, err := b.NodeListInCluster(clusterName)
			if err != nil {
				return "", err
			}
			for _, i := range n {
				if !inslice.HasInt(list, i) {
					list = append(list, i)
				}
			}
		} else if strings.HasPrefix(item, "-") {
			itemNo, err := strconv.Atoi(strings.TrimPrefix(item, "-"))
			if err != nil {
				return "", err
			}
			ind := inslice.Int(list, itemNo, 1)
			if len(ind) == 0 {
				continue
			}
			list = append(list[:ind[0]], list[ind[0]+1:]...)
		} else if strings.Contains(item, "-") {
			itemRange := strings.Split(item, "-")
			if len(itemRange) != 2 {
				return "", errors.New("badly formatted range")
			}
			start, err := strconv.Atoi(itemRange[0])
			if err != nil {
				return "", err
			}
			end, err := strconv.Atoi(itemRange[1])
			if err != nil {
				return "", err
			}
			if start < 1 || end < start {
				return "", errors.New("range is incorrect")
			}
			for start <= end {
				list = append(list, start)
				start++
			}
		} else {
			itemNo, err := strconv.Atoi(item)
			if err != nil {
				return "", err
			}
			list = append(list, itemNo)
		}
	}
	sort.Ints(list)
	return intSliceToString(list, ","), nil
}
