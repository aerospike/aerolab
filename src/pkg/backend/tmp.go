package backend

import "log"

func usageExample() {
	// get inventory for project bob
	b, err := Init("myproject", &Credentials{})
	if err != nil {
		log.Fatal(err)
	}
	b.AddRegion("us-west-2", "us-east-1")
	b.RemoveRegion("eu-west-1")
	b.ListEnabledRegions()

	inv, err := b.GetInventory()
	if err != nil {
		log.Fatal(err)
	}

	//  select volumes in AWS backend, in us-west-2 and us-east-1
	awsBob := inv.Volumes.WithBackendType([]BackendType{BackendTypeAWS}).WithZoneName([]string{"us-west-2", "us-east-1"})

	// how many volumes did we find
	if awsBob.Count() == 0 {
		log.Fatal("not found")
	}

	// add/override a tag for a volume
	err = awsBob.AddTags(map[string]string{
		"someName": "someValue",
	})
	if err != nil {
		log.Fatal(err)
	}

	// we can also cast to a list that can be marshalled
	// after the Describe, vols can be marshalled to either json or yaml if needed
	vols := awsBob.Describe()

	// not only can the vols list be marshalled, but it also implements the Volumes interface
	vols.Count()

	// not only does it implement a Volumes interface, but each item of the list also implements a VolumeAction interface
	vols[0].Action.DeleteVolumes()

	// attach
	inst := inv.Instances.WithBackendType([]BackendType{BackendTypeAWS}).WithZoneName([]string{"us-west-2", "us-east-1"}).WithName([]string{"bob"})
	if inst.Count() != 1 {
		log.Fatal("instance not found")
	}
	vols[0].Action.Attach(inst.Describe()[0], nil)
}

/*
* ALL inventory actions should:
  * check if a Pre* or Post* exists in the Roles library (eg role "aerospike" has PreStop)
  * if a hook exists in the interface for a given Rule, execute it
  * have a force which will continue even if role fails
*/

// add backend
// remove backend
// aerolab config backend add -t aws -r us-west-2 --name primary --default
// aerolab config backend add -t aws -r us-west-1 --name failover
// aerolab config backend add -t gcp -o test-project --name agi

// cluster: bob role: aerospike
// cluster: bob role: tools
// if cluster bob has both roles, GetCluster will return bob for either role selection
// if cluster bob has just one role, it will only get returned if that one role matches
// role is optional, if not specified, will return the cluster always (so in the future we can have clusterName as a single unique entity, not cluster-role pair)

// WE WILL NEED TAG TRANSLATION FUNCTION THAT TRANSLATES ALL TAGS TO NEW TAGS AND BACK, FOR BACKWARDS COMPATIBLITY - REMOVE IN v8
