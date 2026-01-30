package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const openAPIURL = "https://api.aerospike.cloud/v1/openapi.yaml"

type CloudGenConfTemplatesCmd struct {
	OutputDir string  `short:"d" long:"output-dir" description:"Directory to save template files" default:"."`
	Help      HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *CloudGenConfTemplatesCmd) Execute(args []string) error {
	cmd := []string{"cloud", "gen-conf-templates"}
	system, err := Initialize(&Init{InitBackend: false, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	err = c.GenerateTemplates(system)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *CloudGenConfTemplatesCmd) GenerateTemplates(system *System) error {
	logger := system.Logger

	// Create output directory if it doesn't exist
	if err := os.MkdirAll(c.OutputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Download OpenAPI spec
	logger.Info("Downloading OpenAPI spec from %s", openAPIURL)
	spec, err := c.downloadOpenAPISpec()
	if err != nil {
		return fmt.Errorf("failed to download OpenAPI spec: %w", err)
	}
	logger.Info("OpenAPI spec downloaded successfully")

	// Parse and extract schemas
	logger.Info("Parsing OpenAPI spec and extracting schemas")
	schemas, err := c.extractSchemas(spec)
	if err != nil {
		return fmt.Errorf("failed to extract schemas: %w", err)
	}

	// Generate templates
	templates := []struct {
		name       string
		schemaName string
		serverOnly bool
	}{
		{"create-full.json", "database_spec", false},
		{"create-aerospike-server.json", "database_spec", true},
		{"update-full.json", "database_spec_patch", false},
		{"update-aerospike-server.json", "database_spec_patch", true},
	}

	for _, t := range templates {
		logger.Info("Generating template: %s", t.name)
		template, err := c.generateTemplate(schemas, t.schemaName, t.serverOnly)
		if err != nil {
			return fmt.Errorf("failed to generate template %s: %w", t.name, err)
		}

		// Write to file
		filePath := filepath.Join(c.OutputDir, t.name)
		var buf bytes.Buffer
		encoder := json.NewEncoder(&buf)
		encoder.SetEscapeHTML(false)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(template); err != nil {
			return fmt.Errorf("failed to marshal template %s: %w", t.name, err)
		}

		if err := os.WriteFile(filePath, buf.Bytes(), 0644); err != nil {
			return fmt.Errorf("failed to write template %s: %w", t.name, err)
		}
		logger.Info("Template saved to: %s", filePath)
	}

	return nil
}

func (c *CloudGenConfTemplatesCmd) downloadOpenAPISpec() (map[string]interface{}, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(openAPIURL)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP request failed with status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var spec map[string]interface{}
	if err := yaml.Unmarshal(body, &spec); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	return spec, nil
}

func (c *CloudGenConfTemplatesCmd) extractSchemas(spec map[string]interface{}) (map[string]interface{}, error) {
	components, ok := spec["components"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("components not found in OpenAPI spec")
	}

	schemas, ok := components["schemas"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("schemas not found in components")
	}

	return schemas, nil
}

func (c *CloudGenConfTemplatesCmd) generateTemplate(schemas map[string]interface{}, schemaName string, serverOnly bool) (interface{}, error) {
	schema, ok := schemas[schemaName].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("schema %s not found", schemaName)
	}

	template := c.schemaToTemplate(schemas, schema, 0)

	if serverOnly {
		// Extract just the aerospikeServer section
		if templateMap, ok := template.(map[string]interface{}); ok {
			if aerospikeServer, exists := templateMap["aerospikeServer"]; exists {
				return aerospikeServer, nil
			}
			return nil, fmt.Errorf("aerospikeServer not found in schema")
		}
	}

	return template, nil
}

func (c *CloudGenConfTemplatesCmd) schemaToTemplate(schemas map[string]interface{}, schema map[string]interface{}, depth int) interface{} {
	// Prevent infinite recursion
	if depth > 10 {
		return nil
	}

	// Handle $ref
	if ref, ok := schema["$ref"].(string); ok {
		refName := c.extractRefName(ref)
		if refSchema, ok := schemas[refName].(map[string]interface{}); ok {
			return c.schemaToTemplate(schemas, refSchema, depth+1)
		}
		return nil
	}

	// Handle anyOf/oneOf by taking the first option
	if anyOf, ok := schema["anyOf"].([]interface{}); ok && len(anyOf) > 0 {
		if firstOption, ok := anyOf[0].(map[string]interface{}); ok {
			return c.schemaToTemplate(schemas, firstOption, depth+1)
		}
	}
	if oneOf, ok := schema["oneOf"].([]interface{}); ok && len(oneOf) > 0 {
		if firstOption, ok := oneOf[0].(map[string]interface{}); ok {
			return c.schemaToTemplate(schemas, firstOption, depth+1)
		}
	}

	// Handle allOf by merging all schemas
	if allOf, ok := schema["allOf"].([]interface{}); ok {
		merged := make(map[string]interface{})
		for _, item := range allOf {
			if itemSchema, ok := item.(map[string]interface{}); ok {
				itemTemplate := c.schemaToTemplate(schemas, itemSchema, depth+1)
				if itemMap, ok := itemTemplate.(map[string]interface{}); ok {
					for k, v := range itemMap {
						merged[k] = v
					}
				}
			}
		}
		if len(merged) > 0 {
			return merged
		}
	}

	schemaType, _ := schema["type"].(string)

	switch schemaType {
	case "object":
		return c.objectToTemplate(schemas, schema, depth)
	case "array":
		return c.arrayToTemplate(schemas, schema, depth)
	case "string":
		return c.stringToTemplate(schema)
	case "integer", "number":
		return c.numberToTemplate(schema)
	case "boolean":
		return c.booleanToTemplate(schema)
	default:
		// If type is not specified but has properties, treat as object
		if _, hasProps := schema["properties"]; hasProps {
			return c.objectToTemplate(schemas, schema, depth)
		}
		return nil
	}
}

func (c *CloudGenConfTemplatesCmd) objectToTemplate(schemas map[string]interface{}, schema map[string]interface{}, depth int) map[string]interface{} {
	result := make(map[string]interface{})

	properties, ok := schema["properties"].(map[string]interface{})
	if !ok {
		return result
	}

	for name, prop := range properties {
		propSchema, ok := prop.(map[string]interface{})
		if !ok {
			continue
		}

		// Skip readOnly properties
		if readOnly, _ := propSchema["readOnly"].(bool); readOnly {
			continue
		}

		value := c.schemaToTemplate(schemas, propSchema, depth+1)
		if value != nil {
			result[name] = value
		}
	}

	return result
}

func (c *CloudGenConfTemplatesCmd) arrayToTemplate(schemas map[string]interface{}, schema map[string]interface{}, depth int) []interface{} {
	items, ok := schema["items"].(map[string]interface{})
	if !ok {
		return []interface{}{}
	}

	itemTemplate := c.schemaToTemplate(schemas, items, depth+1)
	if itemTemplate != nil {
		return []interface{}{itemTemplate}
	}
	return []interface{}{}
}

func (c *CloudGenConfTemplatesCmd) stringToTemplate(schema map[string]interface{}) string {
	// Use default if available
	if defaultVal, ok := schema["default"].(string); ok {
		return defaultVal
	}

	// Use first enum value if available
	if enum, ok := schema["enum"].([]interface{}); ok && len(enum) > 0 {
		if enumStr, ok := enum[0].(string); ok {
			return enumStr
		}
	}

	// Use pattern hint if available
	if pattern, ok := schema["pattern"].(string); ok {
		return fmt.Sprintf("<%s>", pattern)
	}

	return ""
}

func (c *CloudGenConfTemplatesCmd) numberToTemplate(schema map[string]interface{}) interface{} {
	// Use default if available
	if defaultVal, ok := schema["default"]; ok {
		return defaultVal
	}

	// Use minimum if available
	if minVal, ok := schema["minimum"]; ok {
		return minVal
	}

	return 0
}

func (c *CloudGenConfTemplatesCmd) booleanToTemplate(schema map[string]interface{}) bool {
	// Use default if available
	if defaultVal, ok := schema["default"].(bool); ok {
		return defaultVal
	}
	return false
}

func (c *CloudGenConfTemplatesCmd) extractRefName(ref string) string {
	// Extract schema name from "#/components/schemas/SchemaName"
	parts := strings.Split(ref, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return ""
}
