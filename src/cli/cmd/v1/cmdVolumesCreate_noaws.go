//go:build noaws

package cmd

func buildAWSVolumeParams(_ *VolumesCreateCmd) any {
	return nil
}

func updateAWSVolumePlacement(_ any, _ string) {}
