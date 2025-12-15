package eks

import (
	"bytes"
	"embed"
	"io/fs"
)

//go:embed ekctl-templates/*.yaml
var templates embed.FS

// GetTemplates returns all EKS YAML templates with the AWS region placeholder replaced.
//
// Parameters:
//   - awsRegion: The AWS region to use in the templates (replaces {AWS-REGION})
//
// Returns:
//   - map[string][]byte: Map of filename to file contents
//   - error: nil on success, or an error if reading templates fails
//
// Usage:
//
//	templates, err := eks.GetTemplates("us-east-1")
//	if err != nil {
//	    log.Fatal(err)
//	}
func GetTemplates(awsRegion string) (map[string][]byte, error) {
	result := make(map[string][]byte)

	err := fs.WalkDir(templates, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		contents, err := templates.ReadFile(path)
		if err != nil {
			return err
		}

		// Replace {AWS-REGION} placeholder
		contents = bytes.ReplaceAll(contents, []byte("{AWS-REGION}"), []byte(awsRegion))

		result[d.Name()] = contents
		return nil
	})

	return result, err
}
