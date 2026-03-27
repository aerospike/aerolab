//go:build nogcp

package cmd

func buildGCPVolumeParams(_ *VolumesCreateCmd) any {
	return nil
}
