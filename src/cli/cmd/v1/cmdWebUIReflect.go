//go:build !nowebui

package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"
)

// splitCamelCase inserts spaces between camelCase/PascalCase words.
// Examples:
//
//	"NamespaceMemory" -> "Namespace Memory"
//	"ClusterName"     -> "Cluster Name"
//	"AGI"             -> "AGI"
//	"AWSProfile"      -> "AWS Profile"
//	"SubnetID"        -> "Subnet ID"
//	"SshKeyPath"      -> "Ssh Key Path"
func splitCamelCase(s string) string {
	if s == "" {
		return s
	}
	runes := []rune(s)
	var buf strings.Builder
	buf.Grow(len(s) + 4) // a few extra spaces
	for i, r := range runes {
		if i > 0 && unicode.IsUpper(r) {
			prev := runes[i-1]
			if unicode.IsLower(prev) {
				// transition: lower -> Upper  (e.g. "namespace|M")
				buf.WriteRune(' ')
			} else if unicode.IsUpper(prev) && i+1 < len(runes) && unicode.IsLower(runes[i+1]) {
				// transition: UPPER UPPER lower  (e.g. "AW|S P|rofile" → split before 'P')
				buf.WriteRune(' ')
			}
		}
		buf.WriteRune(r)
	}
	return buf.String()
}

// applyTagDefaults initializes a struct's fields to the values specified in
// their `default` struct tags. This is used to seed a zero-value struct so
// that unset fields match their declared defaults and are excluded from CLI
// generation (which compares field values against tag defaults).
func applyTagDefaults(v reflect.Value) {
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return
	}
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		fieldVal := v.Field(i)
		if !fieldVal.CanSet() || field.PkgPath != "" {
			continue
		}
		// Recurse into embedded/anonymous structs and group structs
		if fieldVal.Kind() == reflect.Struct {
			applyTagDefaults(fieldVal)
			continue
		}
		def := field.Tag.Get("default")
		if def == "" {
			continue
		}
		_ = setFieldValue(fieldVal, def)
	}
}

// BuildCommandTree builds the command tree from the Commands struct using reflection
func BuildCommandTree(opts *Commands) *CommandInfo {
	root := &CommandInfo{
		Name:        "aerolab",
		Path:        "",
		Description: "AeroLab CLI",
		HasChildren: true,
		SimpleMode:  true,
		Children:    []*CommandInfo{},
	}

	val := reflect.ValueOf(opts).Elem()
	typ := val.Type()

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		fieldVal := val.Field(i)

		// Check if this is a command
		cmdTag := field.Tag.Get("command")
		if cmdTag == "" {
			continue
		}

		// Skip the Help command
		if cmdTag == "help" {
			continue
		}

		child := buildCommandInfo(field, fieldVal, cmdTag, "")
		if child != nil {
			root.Children = append(root.Children, child)
		}
	}

	return root
}

// buildCommandInfo recursively builds CommandInfo for a command struct
func buildCommandInfo(field reflect.StructField, fieldVal reflect.Value, name string, parentPath string) *CommandInfo {
	tags := field.Tag

	path := name
	if parentPath != "" {
		path = parentPath + "/" + name
	}

	cmd := &CommandInfo{
		Name:        name,
		Path:        path,
		Description: tags.Get("description"),
		Icon:        tags.Get("webicon"),
		Hidden:      tags.Get("hidden") == "true",
		WebHidden:   tags.Get("webhidden") == "true",
		SimpleMode:  tags.Get("simplemode") != "false", // defaults to true
		InvWebForce: tags.Get("invwebforce") == "true",
		Children:    []*CommandInfo{},
		Parameters:  []ParameterInfo{},
		reflectType: field.Type,
	}

	// Set display name: prefer webname tag, fall back to Go field name with camelCase splitting
	if webname := tags.Get("webname"); webname != "" {
		cmd.DisplayName = webname
	} else {
		cmd.DisplayName = splitCamelCase(field.Name)
	}

	// Handle pointer types
	elemType := field.Type
	if elemType.Kind() == reflect.Pointer {
		elemType = elemType.Elem()
	}

	// If not a struct, return as-is
	if elemType.Kind() != reflect.Struct {
		return cmd
	}

	// Process struct fields
	for i := 0; i < elemType.NumField(); i++ {
		structField := elemType.Field(i)

		// Skip unexported fields
		if structField.PkgPath != "" {
			continue
		}

		// Check if this is a subcommand
		subCmdTag := structField.Tag.Get("command")
		if subCmdTag != "" {
			if subCmdTag == "help" {
				continue
			}
			// Get the field value for the subcommand
			var subFieldVal reflect.Value
			if fieldVal.Kind() == reflect.Pointer && !fieldVal.IsNil() {
				subFieldVal = fieldVal.Elem().Field(i)
			} else if fieldVal.Kind() == reflect.Struct {
				subFieldVal = fieldVal.Field(i)
			} else {
				subFieldVal = reflect.New(structField.Type).Elem()
			}

			child := buildCommandInfo(structField, subFieldVal, subCmdTag, path)
			if child != nil {
				cmd.Children = append(cmd.Children, child)
				cmd.HasChildren = true
			}
			continue
		}

		// Check if this is a group (embedded struct without command tag)
		if structField.Tag.Get("group") != "" {
			groupParams := extractGroupParameters(structField, path)
			cmd.Parameters = append(cmd.Parameters, groupParams...)
			continue
		}

		// Check for positional args
		if structField.Tag.Get("positional-args") == "true" {
			posParams := extractPositionalParameters(structField, path)
			cmd.Parameters = append(cmd.Parameters, posParams...)
			continue
		}

		// Handle anonymous embedded structs (e.g., ClusterGrowCmd embeds ClusterCreateCmd).
		// Recurse into the embedded struct to extract its parameters so they appear in the
		// command tree and web UI form.
		if structField.Anonymous && structField.Type.Kind() == reflect.Struct {
			extractEmbeddedParameters(structField.Type, cmd, path)
			continue
		}

		// This is a regular parameter
		param := extractParameter(structField)
		if param != nil {
			cmd.Parameters = append(cmd.Parameters, *param)
		}
	}

	return cmd
}

// extractEmbeddedParameters recursively extracts parameters from an anonymous embedded struct type.
func extractEmbeddedParameters(t reflect.Type, cmd *CommandInfo, path string) {
	for field := range t.Fields() {
		if field.PkgPath != "" {
			continue // skip unexported
		}
		if field.Tag.Get("command") != "" || field.Tag.Get("command") == "help" {
			continue // skip subcommands
		}
		if field.Tag.Get("group") != "" {
			groupParams := extractGroupParameters(field, path)
			cmd.Parameters = append(cmd.Parameters, groupParams...)
			continue
		}
		if field.Tag.Get("positional-args") == "true" {
			posParams := extractPositionalParameters(field, path)
			cmd.Parameters = append(cmd.Parameters, posParams...)
			continue
		}
		if field.Anonymous && field.Type.Kind() == reflect.Struct {
			extractEmbeddedParameters(field.Type, cmd, path)
			continue
		}
		param := extractParameter(field)
		if param != nil {
			cmd.Parameters = append(cmd.Parameters, *param)
		}
	}
}

// extractParameter extracts parameter info from a struct field
func extractParameter(field reflect.StructField) *ParameterInfo {
	tags := field.Tag

	// Skip if no long or short flag
	long := tags.Get("long")
	short := tags.Get("short")
	if long == "" && short == "" {
		// Could be an embedded struct or other non-parameter field
		return nil
	}

	param := &ParameterInfo{
		Name:        long,
		FieldName:   field.Name,
		Short:       short,
		Long:        long,
		Description: tags.Get("description"),
		Default:     tags.Get("default"),
		Required:    tags.Get("webrequired") == "true",
		WebType:     tags.Get("webtype"),
		Hidden:      tags.Get("hidden") == "true",
		WebHidden:   tags.Get("webhidden") == "true",
		SimpleMode:  tags.Get("simplemode") != "false", // defaults to true
		Group:       tags.Get("group"),
		Namespace:   tags.Get("namespace"),
		NoDefault:   tags.Get("no-default") == "true",
	}

	if param.Name == "" {
		param.Name = field.Name
	}

	// Determine type
	param.Type = getTypeName(field.Type)
	param.IsSlice = field.Type.Kind() == reflect.Slice

	// Detect flags.Filename fields (file path inputs)
	typeName := field.Type.String()
	if typeName == "flags.Filename" || typeName == "*flags.Filename" {
		param.IsFile = true
	}
	if field.Type.Kind() == reflect.Slice && field.Type.Elem().String() == "flags.Filename" {
		param.IsFile = true
	}

	// Mark pointer-type fields as optional - these represent parameters
	// where nil means "not set" (e.g., *string, *bool, *int in AgiRetriggerCmd).
	if field.Type.Kind() == reflect.Pointer {
		param.Optional = true
		param.NoDefault = true // ensures ToggleInput uses ON/UNSET/OFF for *bool
	}

	// Handle webchoice
	webchoice := tags.Get("webchoice")
	if webchoice != "" {
		if after, ok := strings.CutPrefix(webchoice, "method::"); ok {
			param.ChoicesMethod = after
		} else {
			param.Choices = strings.Split(webchoice, ",")
		}
	}

	// Set display name: prefer webname tag, fall back to Go field name with camelCase splitting
	webname := tags.Get("webname")
	if webname != "" {
		param.DisplayName = webname
	} else {
		param.DisplayName = splitCamelCase(field.Name)
	}

	return param
}

// extractGroupParameters extracts parameters from a group struct
func extractGroupParameters(field reflect.StructField, parentPath string) []ParameterInfo {
	params := []ParameterInfo{}

	groupName := field.Tag.Get("group")
	namespace := field.Tag.Get("namespace")
	groupDesc := field.Tag.Get("description")

	// Get the underlying type
	elemType := field.Type
	if elemType.Kind() == reflect.Pointer {
		elemType = elemType.Elem()
	}

	if elemType.Kind() != reflect.Struct {
		return params
	}

	for structField := range elemType.Fields() {

		// Skip unexported fields
		if structField.PkgPath != "" {
			continue
		}

		// Check for nested groups
		if structField.Tag.Get("group") != "" {
			nestedParams := extractGroupParameters(structField, parentPath)
			params = append(params, nestedParams...)
			continue
		}

		param := extractParameter(structField)
		if param != nil {
			if structField.Type.Kind() == reflect.Pointer {
				param.Optional = true
				param.NoDefault = true
			}
			param.Group = groupName
			if namespace != "" {
				param.Namespace = namespace
			}
			// Add group description context
			if groupDesc != "" && strings.HasPrefix(groupDesc, "backend-") {
				param.Group = groupName + " (" + groupDesc + ")"
			}
			params = append(params, *param)
		}
	}

	return params
}

// extractPositionalParameters extracts parameters from positional args struct
func extractPositionalParameters(field reflect.StructField, parentPath string) []ParameterInfo {
	params := []ParameterInfo{}

	elemType := field.Type
	if elemType.Kind() == reflect.Pointer {
		elemType = elemType.Elem()
	}

	if elemType.Kind() != reflect.Struct {
		return params
	}

	for structField := range elemType.Fields() {

		// Skip unexported fields
		if structField.PkgPath != "" {
			continue
		}

		param := &ParameterInfo{
			Name:         structField.Name,
			FieldName:    structField.Name,
			Description:  structField.Tag.Get("description"),
			Type:         getTypeName(structField.Type),
			WebType:      structField.Tag.Get("webtype"),
			IsSlice:      structField.Type.Kind() == reflect.Slice,
			IsPositional: true,
			SimpleMode:   structField.Tag.Get("simplemode") != "false",
		}
		// Detect flags.Filename fields (file path inputs)
		typeName := structField.Type.String()
		if typeName == "flags.Filename" || typeName == "*flags.Filename" {
			param.IsFile = true
		}
		if structField.Type.Kind() == reflect.Slice && structField.Type.Elem().String() == "flags.Filename" {
			param.IsFile = true
		}
		webname := structField.Tag.Get("webname")
		if webname != "" {
			param.DisplayName = webname
		} else {
			param.DisplayName = splitCamelCase(structField.Name)
		}

		params = append(params, *param)
	}

	return params
}

// filterBackendParameters removes parameters belonging to non-active backends
// from the command tree. Parameters are identified as backend-specific by their
// group name containing "(backend-X)" (e.g., "AWS (backend-aws)").
// This mirrors the CLI's ShowHideBackend behavior for the web UI.
func filterBackendParameters(root *CommandInfo, activeBackend string) {
	if root == nil {
		return
	}

	if len(root.Parameters) > 0 {
		filtered := make([]ParameterInfo, 0, len(root.Parameters))
		for _, p := range root.Parameters {
			backendName := extractBackendFromGroup(p.Group)
			if backendName != "" {
				if backendName != activeBackend {
					continue // Skip parameters for non-active backends
				}
				// Clean up group name: "AWS (backend-aws)" -> "AWS"
				p.Group = cleanBackendGroupName(p.Group)
			}
			filtered = append(filtered, p)
		}
		root.Parameters = filtered
	}

	// Recurse into children
	for _, child := range root.Children {
		filterBackendParameters(child, activeBackend)
	}
}

// extractBackendFromGroup extracts the backend name from a group label.
// e.g., "AWS (backend-aws)" returns "aws", "GCP (backend-gcp)" returns "gcp".
// Returns "" if the group is not backend-specific.
func extractBackendFromGroup(group string) string {
	_, after, ok := strings.Cut(group, "(backend-")
	if !ok {
		return ""
	}
	rest := after
	before, _, ok := strings.Cut(rest, ")")
	if !ok {
		return ""
	}
	return before
}

// cleanBackendGroupName removes the "(backend-X)" suffix from a group name.
// e.g., "AWS (backend-aws)" -> "AWS"
func cleanBackendGroupName(group string) string {
	before, _, ok := strings.Cut(group, " (backend-")
	if !ok {
		return group
	}
	return before
}

// getTypeName returns a human-readable type name
func getTypeName(t reflect.Type) string {
	switch t.Kind() {
	case reflect.String:
		return "string"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if t.String() == "time.Duration" {
			return "duration"
		}
		return "int"
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "uint"
	case reflect.Float32, reflect.Float64:
		return "float"
	case reflect.Bool:
		return "bool"
	case reflect.Slice:
		elemType := getTypeName(t.Elem())
		return "[]" + elemType
	case reflect.Pointer:
		return getTypeName(t.Elem())
	case reflect.Struct:
		return "object"
	default:
		return t.String()
	}
}

// ResolveDynamicChoices calls a method to get dynamic choices for a parameter.
// It searches the command struct (including nested group/namespace structs) for
// the field matching paramName/fieldName, then calls the specified method on it.
// Returns (values, labels, error) where labels may be nil if no display labels are available.
func ResolveDynamicChoices(system *System, cmdPath string, paramName string, methodName string, namespace string, fieldName string) ([]string, []string, error) {
	if system == nil {
		return nil, nil, fmt.Errorf("system not initialized")
	}

	// Find the command struct by path
	cmdVal, err := getCommandValueByPath(system.Opts, cmdPath)
	if err != nil {
		return nil, nil, err
	}

	// Try to find the field, searching into nested group structs if needed
	fieldVal, found := findFieldInStruct(cmdVal, paramName, fieldName, namespace)
	if !found {
		return nil, nil, fmt.Errorf("method %s not found for parameter %s", methodName, paramName)
	}

	return callListMethod(fieldVal, methodName, system, paramName)
}

// findFieldInStruct searches for a parameter field in a struct, including nested
// group/namespace structs. Returns the field value and whether it was found.
func findFieldInStruct(structVal reflect.Value, paramName string, fieldName string, namespace string) (reflect.Value, bool) {
	structType := structVal.Type()

	for i := 0; i < structType.NumField(); i++ {
		field := structType.Field(i)
		fVal := structVal.Field(i)

		// If this field has a "group" tag, it's a nested group struct - search inside it
		if field.Tag.Get("group") != "" {
			elemType := field.Type
			elemVal := fVal
			if elemType.Kind() == reflect.Pointer {
				if elemVal.IsNil() {
					continue
				}
				elemVal = elemVal.Elem()
				elemType = elemType.Elem()
			}
			if elemType.Kind() == reflect.Struct {
				// If namespace is specified, only search the matching group
				if namespace != "" && field.Tag.Get("namespace") != namespace {
					continue
				}
				if result, found := findFieldInStruct(elemVal, paramName, fieldName, ""); found {
					return result, true
				}
			}
			continue
		}

		// Match by long tag or field name
		if field.Tag.Get("long") == paramName || field.Name == fieldName {
			return fVal, true
		}
	}

	return reflect.Value{}, false
}

// callListMethod calls a named method on a field value that returns dynamic choices.
// The method signature is expected to be: func(system *System) ([][]string, string, error)
// Each inner []string is [value, displayLabel]. If only one element, the value is used as the label.
// Returns (values, labels, error).
func callListMethod(fieldVal reflect.Value, methodName string, system *System, paramName string) ([]string, []string, error) {
	method := fieldVal.MethodByName(methodName)
	if !method.IsValid() {
		// Try pointer receiver
		if fieldVal.CanAddr() {
			method = fieldVal.Addr().MethodByName(methodName)
		}
	}
	if !method.IsValid() {
		return nil, nil, fmt.Errorf("method %s not found for parameter %s", methodName, paramName)
	}

	// Call the method with system parameter
	results := method.Call([]reflect.Value{reflect.ValueOf(system)})
	if len(results) >= 2 {
		// Results are: [][]string, string (default), error
		if !results[len(results)-1].IsNil() {
			return nil, nil, results[len(results)-1].Interface().(error)
		}
		if results[0].Kind() == reflect.Slice {
			// Extract choices from [][]string
			// First element of each inner slice is the value, second (if present) is the display label
			choices := []string{}
			labels := []string{}
			hasLabels := false
			for i := 0; i < results[0].Len(); i++ {
				inner := results[0].Index(i)
				if inner.Kind() == reflect.Slice && inner.Len() > 0 {
					val := inner.Index(0).String()
					choices = append(choices, val)
					if inner.Len() > 1 {
						labels = append(labels, inner.Index(1).String())
						hasLabels = true
					} else {
						labels = append(labels, val)
					}
				}
			}
			if !hasLabels {
				labels = nil // don't send labels if they're all identical to values
			}
			return choices, labels, nil
		}
	}

	return nil, nil, fmt.Errorf("method %s returned unexpected results for parameter %s", methodName, paramName)
}

// getCommandValueByPath navigates to a command struct by path
func getCommandValueByPath(opts *Commands, path string) (reflect.Value, error) {
	parts := strings.Split(path, "/")

	current := reflect.ValueOf(opts).Elem()

	for _, part := range parts {
		found := false
		for i := 0; i < current.NumField(); i++ {
			field := current.Type().Field(i)
			if field.Tag.Get("command") == part {
				current = current.Field(i)
				if current.Kind() == reflect.Pointer {
					if current.IsNil() {
						current = reflect.New(current.Type().Elem()).Elem()
					} else {
						current = current.Elem()
					}
				}
				found = true
				break
			}
		}
		if !found {
			return reflect.Value{}, fmt.Errorf("command not found: %s", part)
		}
	}

	return current, nil
}

// ExecuteCommandByPath executes a command by its path with given parameters (synchronous)
func ExecuteCommandByPath(system *System, path string, params map[string]any) (*ExecuteResult, error) {
	// Find the command struct type by path
	cmdVal, err := getCommandValueByPath(system.Opts, path)
	if err != nil {
		return nil, err
	}

	// Create a new instance of the command
	cmdType := cmdVal.Type()
	newCmd := reflect.New(cmdType).Elem()

	// Copy defaults from the original
	newCmd.Set(cmdVal)

	// Apply parameters from the request
	if params != nil {
		if err := applyParameters(newCmd, params); err != nil {
			return nil, fmt.Errorf("failed to apply parameters: %w", err)
		}
	}

	// Get the Execute method
	cmdPtr := newCmd.Addr()
	executeMethod := cmdPtr.MethodByName("Execute")
	if !executeMethod.IsValid() {
		return nil, fmt.Errorf("command %s does not have an Execute method", path)
	}

	// Capture logs during execution
	logs := []string{}

	// Call Execute with empty args
	args := []string{}
	results := executeMethod.Call([]reflect.Value{reflect.ValueOf(args)})

	// Check for error
	if len(results) > 0 && !results[0].IsNil() {
		err := results[0].Interface().(error)
		return &ExecuteResult{Logs: logs}, err
	}

	return &ExecuteResult{
		Result: "Command executed successfully",
		Logs:   logs,
	}, nil
}

// ExecuteCommandByPathWithLogger executes a command by its path with given parameters,
// writing logs to the provided io.Writer (for async job execution)
func ExecuteCommandByPathWithLogger(system *System, path string, params map[string]any, logWriter io.Writer) error {
	// Find the command struct type by path
	cmdVal, err := getCommandValueByPath(system.Opts, path)
	if err != nil {
		if logWriter != nil {
			fmt.Fprintf(logWriter, "[ERROR] Failed to find command: %s\n", err) //nolint:errcheck
		}
		return err
	}

	// Create a new instance of the command
	cmdType := cmdVal.Type()
	newCmd := reflect.New(cmdType).Elem()

	// Copy defaults from the original
	newCmd.Set(cmdVal)

	// Apply parameters from the request
	if params != nil {
		if err := applyParameters(newCmd, params); err != nil {
			if logWriter != nil {
				fmt.Fprintf(logWriter, "[ERROR] Failed to apply parameters: %s\n", err) //nolint:errcheck
			}
			return fmt.Errorf("failed to apply parameters: %w", err)
		}
	}

	// Get the Execute method
	cmdPtr := newCmd.Addr()
	executeMethod := cmdPtr.MethodByName("Execute")
	if !executeMethod.IsValid() {
		errMsg := fmt.Sprintf("command %s does not have an Execute method", path)
		if logWriter != nil {
			fmt.Fprintf(logWriter, "[ERROR] %s\n", errMsg) //nolint:errcheck
		}
		return fmt.Errorf("%s", errMsg)
	}

	// Write start message
	if logWriter != nil {
		fmt.Fprintf(logWriter, "[INFO] Starting command: %s\n", path) //nolint:errcheck
		fmt.Fprintf(logWriter, "[INFO] Parameters: %v\n", params)     //nolint:errcheck
		fmt.Fprintf(logWriter, "[INFO] Time: %s\n", strings.Repeat("-", 50)) //nolint:errcheck
	}

	// Call Execute with empty args
	args := []string{}
	results := executeMethod.Call([]reflect.Value{reflect.ValueOf(args)})

	// Check for error
	if len(results) > 0 && !results[0].IsNil() {
		err := results[0].Interface().(error)
		if logWriter != nil {
			fmt.Fprintf(logWriter, "[ERROR] Command failed: %s\n", err) //nolint:errcheck
		}
		return err
	}

	if logWriter != nil {
		fmt.Fprintf(logWriter, "[INFO] Command completed successfully\n") //nolint:errcheck
	}

	return nil
}

// applyParameters applies a map of parameters to a command struct.
// It supports both nested maps ({"Aws": {"InstanceType": "t3.xlarge"}}) and
// flat maps ({"InstanceType": "t3.xlarge"}) for group parameters.
func applyParameters(cmdVal reflect.Value, params map[string]any) error {
	return applyParametersWithPrefix(cmdVal, params, "")
}

// applyParametersWithPrefix applies parameters with a namespace prefix.
// When prefix is "aws", param lookups for a field with long:"name" will try
// "aws-name" first, then "name", then the field name.
func applyParametersWithPrefix(cmdVal reflect.Value, params map[string]any, prefix string) error {
	cmdType := cmdVal.Type()

	for i := 0; i < cmdType.NumField(); i++ {
		field := cmdType.Field(i)
		fieldVal := cmdVal.Field(i)

		// Skip unexported fields
		if field.PkgPath != "" {
			continue
		}

		// Handle nested structs (groups) first, before the general lookup,
		// because group fields typically don't have long/short tags and
		// the frontend sends flat parameter maps where group sub-fields
		// are at the top level (e.g. {"InstanceType": "t3.xlarge"}).
		if field.Type.Kind() == reflect.Struct && field.Tag.Get("group") != "" {
			// First, check if the params have a nested map matching the group name
			groupKey := field.Tag.Get("long")
			if groupKey == "" {
				groupKey = field.Name
			}
			if mapVal, ok := params[groupKey]; ok {
				if nestedMap, mapOk := mapVal.(map[string]any); mapOk {
					namespace := field.Tag.Get("namespace")
					if err := applyParametersWithPrefix(fieldVal, nestedMap, namespace); err != nil {
						return err
					}
					continue
				}
			}
			// No nested map found - try flat params by recursing with the
			// same params map so individual fields within the group can be
			// matched directly (e.g. {"InstanceType": "t3.xlarge"}).
			// Propagate the namespace from the group tag for correct prefix matching.
			namespace := field.Tag.Get("namespace")
			newPrefix := prefix
			if namespace != "" {
				if newPrefix != "" {
					newPrefix = newPrefix + "-" + namespace
				} else {
					newPrefix = namespace
				}
			}
			if err := applyParametersWithPrefix(fieldVal, params, newPrefix); err != nil {
				return err
			}
			continue
		}

		// Handle embedded structs (anonymous fields without group tag)
		if field.Anonymous && field.Type.Kind() == reflect.Struct {
			if err := applyParametersWithPrefix(fieldVal, params, prefix); err != nil {
				return err
			}
			continue
		}

		// Get the parameter name from tags
		paramName := field.Tag.Get("long")
		if paramName == "" {
			paramName = field.Name
		}

		// Build the prefixed parameter name (matches what the frontend sends)
		prefixedName := paramName
		if prefix != "" {
			prefixedName = prefix + "-" + paramName
		}

		// Check if we have a value for this parameter.
		// When a prefix is active, only match the prefixed name to avoid
		// cross-contamination (e.g. InstanceDNS.Name "aws-name" should NOT
		// match the top-level "name" parameter).
		value, ok := params[prefixedName]
		if !ok && prefix == "" {
			// Only try bare field name and short name when there's no prefix
			value, ok = params[field.Name]
			if !ok {
				shortName := field.Tag.Get("short")
				if shortName != "" {
					value, ok = params[shortName]
				}
			}
		}
		if !ok {
			continue
		}

		// Set the field value
		if err := setFieldValue(fieldVal, value); err != nil {
			return fmt.Errorf("failed to set %s: %w", prefixedName, err)
		}
	}

	return nil
}

// setFieldValue sets a reflect.Value from an interface{}
func setFieldValue(fieldVal reflect.Value, value any) error {
	if !fieldVal.CanSet() {
		return fmt.Errorf("cannot set field")
	}

	// Handle nil
	if value == nil {
		return nil
	}

	// Convert JSON numbers to appropriate types
	switch fieldVal.Kind() {
	case reflect.String:
		if s, ok := value.(string); ok {
			fieldVal.SetString(s)
		} else {
			fieldVal.SetString(fmt.Sprintf("%v", value))
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		switch v := value.(type) {
		case float64:
			fieldVal.SetInt(int64(v))
		case int:
			fieldVal.SetInt(int64(v))
		case int64:
			fieldVal.SetInt(v)
		case string:
			// Empty strings mean "no value" / zero
			if v == "" {
				fieldVal.SetInt(0)
				return nil
			}
			// Try parsing as duration if the type is time.Duration
			if fieldVal.Type().String() == "time.Duration" {
				dur, err := ParseExtendedDuration(v)
				if err != nil {
					// Fall back to parsing as plain integer (nanoseconds)
					var n int64
					if _, scanErr := fmt.Sscanf(v, "%d", &n); scanErr != nil {
						return fmt.Errorf("cannot parse duration %q: %w", v, err)
					}
					fieldVal.SetInt(n)
				} else {
					fieldVal.SetInt(int64(dur))
				}
			} else {
				// Regular int from string
				var n int64
				if _, err := fmt.Sscanf(v, "%d", &n); err != nil {
					return fmt.Errorf("cannot convert string %q to int: %w", v, err)
				}
				fieldVal.SetInt(n)
			}
		default:
			return fmt.Errorf("cannot convert %T to int", value)
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		switch v := value.(type) {
		case float64:
			fieldVal.SetUint(uint64(v))
		case int:
			fieldVal.SetUint(uint64(v))
		case int64:
			fieldVal.SetUint(uint64(v))
		case string:
			if v == "" {
				fieldVal.SetUint(0)
				return nil
			}
			// HTML form inputs produce strings; parse them
			var n uint64
			if _, err := fmt.Sscanf(v, "%d", &n); err != nil {
				return fmt.Errorf("cannot convert string %q to uint: %w", v, err)
			}
			fieldVal.SetUint(n)
		default:
			return fmt.Errorf("cannot convert %T to uint", value)
		}
	case reflect.Float32, reflect.Float64:
		switch v := value.(type) {
		case float64:
			fieldVal.SetFloat(v)
		case int:
			fieldVal.SetFloat(float64(v))
		case string:
			if v == "" {
				fieldVal.SetFloat(0)
				return nil
			}
			// HTML form inputs produce strings; parse them
			var f float64
			if _, err := fmt.Sscanf(v, "%f", &f); err != nil {
				return fmt.Errorf("cannot convert string %q to float: %w", v, err)
			}
			fieldVal.SetFloat(f)
		default:
			return fmt.Errorf("cannot convert %T to float", value)
		}
	case reflect.Bool:
		switch v := value.(type) {
		case bool:
			fieldVal.SetBool(v)
		case string:
			fieldVal.SetBool(v == "true" || v == "yes" || v == "1" || v == "y")
		default:
			return fmt.Errorf("cannot convert %T to bool", value)
		}
	case reflect.Slice:
		// Handle slice types
		switch v := value.(type) {
		case []any:
			slice := reflect.MakeSlice(fieldVal.Type(), len(v), len(v))
			for i, elem := range v {
				if err := setFieldValue(slice.Index(i), elem); err != nil {
					return err
				}
			}
			fieldVal.Set(slice)
		case []string:
			slice := reflect.MakeSlice(fieldVal.Type(), len(v), len(v))
			for i, elem := range v {
				slice.Index(i).SetString(elem)
			}
			fieldVal.Set(slice)
		default:
			// Try JSON marshal/unmarshal
			data, err := json.Marshal(value)
			if err != nil {
				return err
			}
			newSlice := reflect.New(fieldVal.Type())
			if err := json.Unmarshal(data, newSlice.Interface()); err != nil {
				return err
			}
			fieldVal.Set(newSlice.Elem())
		}
	case reflect.Pointer:
		// Handle pointer types
		if fieldVal.IsNil() {
			fieldVal.Set(reflect.New(fieldVal.Type().Elem()))
		}
		return setFieldValue(fieldVal.Elem(), value)
	default:
		// Try JSON marshal/unmarshal for complex types
		data, err := json.Marshal(value)
		if err != nil {
			return err
		}
		return json.Unmarshal(data, fieldVal.Addr().Interface())
	}

	return nil
}

// WebUIConfig holds the external configuration file structure for WebUI parameter overrides.
type WebUIConfig struct {
	Overrides map[string]map[string]ParameterOverride `yaml:"overrides"`
}

// ParameterOverride defines the overridable properties for a single parameter.
// Pointer types are used so that unset fields can be distinguished from zero values.
type ParameterOverride struct {
	Choices     []string `yaml:"choices,omitempty"`
	Default     *string  `yaml:"default,omitempty"`
	Hidden      *bool    `yaml:"hidden,omitempty"`
	Required    *bool    `yaml:"required,omitempty"`
	Description *string  `yaml:"description,omitempty"`
}

// applyConfigOverrides reads a YAML config file and applies parameter overrides
// to the command tree. It warns (but does not fail) for unmatched command paths
// or parameter names, so the config is loosely coupled to command changes.
func applyConfigOverrides(root *CommandInfo, configPath string, system *System) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file %s: %w", configPath, err)
	}

	var cfg WebUIConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("failed to parse config file %s: %w", configPath, err)
	}

	if cfg.Overrides == nil {
		system.Logger.Info("Config file %s loaded but contains no overrides", configPath)
		return nil
	}

	applied := 0
	for cmdPath, paramOverrides := range cfg.Overrides {
		cmd := root.FindByPath(cmdPath)
		if cmd == nil {
			system.Logger.Warn("Config override: command path %q not found in command tree, skipping", cmdPath)
			continue
		}

		for paramName, override := range paramOverrides {
			paramIdx := findParameterByName(cmd.Parameters, paramName)
			if paramIdx < 0 {
				system.Logger.Warn("Config override: parameter %q not found in command %q, skipping", paramName, cmdPath)
				continue
			}

			p := &cmd.Parameters[paramIdx]

			if override.Choices != nil {
				p.Choices = override.Choices
			}
			if override.Default != nil {
				p.Default = *override.Default
			}
			if override.Hidden != nil {
				p.WebHidden = *override.Hidden
			}
			if override.Required != nil {
				p.Required = *override.Required
			}
			if override.Description != nil {
				p.Description = *override.Description
			}

			applied++
		}
	}

	system.Logger.Info("Applied %d parameter override(s) from config file %s", applied, configPath)
	return nil
}

// findParameterByName searches for a parameter by its long flag name (case-insensitive).
// Returns the index in the slice, or -1 if not found.
func findParameterByName(params []ParameterInfo, name string) int {
	for i := range params {
		if strings.EqualFold(params[i].Long, name) || strings.EqualFold(params[i].Name, name) {
			return i
		}
	}
	return -1
}
