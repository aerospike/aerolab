package main

import (
	"errors"
	"fmt"
)

func (c *config) F_nukeTemplate() (err error, ret int64) {
	// get backend
	b, err := getBackend(c.NukeTemplate.DeployOn, c.NukeTemplate.RemoteHost, c.NukeTemplate.AccessPublicKeyFilePath)
	if err != nil {
		ret = E_BACKEND_ERROR
		return err, ret
	}

	// check template exists
	versions, err := b.ListTemplates()
	if err != nil {
		ret = E_BACKEND_ERROR
		return err, ret
	}

	if c.NukeTemplate.DistroName != "all" && c.NukeTemplate.DistroVersion != "all" && c.NukeTemplate.AerospikeVersion != "all" {
		v := version{c.NukeTemplate.DistroName, c.NukeTemplate.DistroVersion, c.NukeTemplate.AerospikeVersion}

		if inArray(versions, v) == -1 {
			err = errors.New(fmt.Sprintf("Template does not exist"))
			ret = E_BACKEND_ERROR
			return err, ret
		}

		err = b.TemplateDestroy(v)
		if err != nil {
			ret = E_BACKEND_ERROR
		}
		return err, ret
	} else {
		var nerr error
		for _, v := range versions {
			if c.NukeTemplate.DistroName == "all" || c.NukeTemplate.DistroName == v.distroName {
				if c.NukeTemplate.DistroVersion == "all" || c.NukeTemplate.DistroVersion == v.distroVersion {
					if c.NukeTemplate.AerospikeVersion == "all" || c.NukeTemplate.AerospikeVersion == v.aerospikeVersion {
						err = b.TemplateDestroy(v)
						if err != nil {
							ret = E_BACKEND_ERROR
							nerr = err
						}
					}
				}
			}
		}
		return nerr, ret
	}
}
