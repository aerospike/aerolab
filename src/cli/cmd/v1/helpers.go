package cmd

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/termutil"
	"github.com/aerospike/aerolab/pkg/utils/shutdown"
	"github.com/mattn/go-isatty"
)

func GetSelfPath() (string, error) {
	// Get the absolute path of the current executable
	cur, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path of self: %s", err)
	}

	// Resolve the symlink to the current executable
	for {
		if st, err := os.Stat(cur); err != nil {
			return "", fmt.Errorf("failed to stat self: %s", err)
		} else {
			if st.Mode()&os.ModeSymlink > 0 {
				cur, err = filepath.EvalSymlinks(cur)
				if err != nil {
					return "", fmt.Errorf("error resolving symlink source to self: %s", err)
				}
			} else {
				break
			}
		}
	}
	return cur, nil
}

type osSelectorCmd struct {
	DistroName    TypeDistro        `short:"d" long:"distro" description:"Linux distro, one of: debian|ubuntu|centos|rocky|amazon" default:"ubuntu" webchoice:"debian,ubuntu,rocky,centos,amazon"`
	DistroVersion TypeDistroVersion `short:"i" long:"distro-version" description:"ubuntu:24.04|22.04|20.04|18.04 rocky:10,9,8 centos:10,9,7 amazon:2|2023 debian:13|12|11|10|9|8" default:"latest" webchoice:"latest,24.04,22.04,20.04,18.04,2023,2,13,12,11,10,9,8,7"`
}

type aerospikeVersionCmd struct {
	AerospikeVersion TypeAerospikeVersion `short:"v" long:"aerospike-version" description:"Aerospike server version; add 'c' to the end for community edition, or 'f' for federal edition" default:"latest"`
}

type aerospikeVersionSelectorCmd struct {
	osSelectorCmd
	aerospikeVersionCmd
}

// string format: [protocol:]from[-to]
func parsePortRange(port string) (string, int, int, error) {
	protocol := "tcp"
	parts := strings.Split(port, ":")
	if len(parts) > 1 {
		protocol = parts[0]
		port = parts[1]
	}
	parts = strings.Split(port, "-")
	if len(parts) == 1 {
		port, err := strconv.Atoi(parts[0])
		return protocol, port, port, err
	}
	from, err := strconv.Atoi(parts[0])
	if err != nil {
		return protocol, 0, 0, err
	}
	to, err := strconv.Atoi(parts[1])
	if err != nil {
		return protocol, 0, 0, err
	}
	if from > to {
		return protocol, 0, 0, errors.New("from port must be less than to port")
	}
	return protocol, from, to, nil
}

/*
func getip2_old() string {
	type IP struct {
		Query string
	}
	req, err := http.Get("http://ip-api.com/json/")
	if err != nil {
		return err.Error()
	}
	defer req.Body.Close()

	body, err := io.ReadAll(req.Body)
	if err != nil {
		return err.Error()
	}

	var ip IP
	json.Unmarshal(body, &ip)

	return ip.Query
}
*/

func getip2() string {
	req, err := http.Get("https://api.ipify.org?format=json")
	if err != nil {
		return err.Error()
	}
	defer req.Body.Close()

	body, err := io.ReadAll(req.Body)
	if err != nil {
		return err.Error()
	}

	type ret struct {
		IP string `json:"ip"`
	}

	var ip ret
	json.Unmarshal(body, &ip)

	return ip.IP
}

func IsInteractive() bool {
	return os.Getenv("AEROLAB_NONINTERACTIVE") == "" && (isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd())) && termutil.IsForegroundNoError(os.Stdout.Fd(), true)
}

func AskForString(prompt string) (string, error) {
	if IsInteractive() {
		fmt.Printf("%s: ", prompt)
		reader := bufio.NewReader(os.Stdin)
		return reader.ReadString('\n')
	}
	return "", errors.New("not interactive")
}

func AskForInt(prompt string) (int, error) {
	s, err := AskForString(prompt)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(s))
}

func AskForFloat(prompt string) (float64, error) {
	s, err := AskForString(prompt)
	if err != nil {
		return 0, err
	}
	return strconv.ParseFloat(strings.TrimSpace(s), 64)
}

func updateDiskCacheDo(system *System) {
	if system == nil || !system.Opts.Config.Backend.InventoryCache {
		return
	}
	system.Logger.Info("Updating disk cache")
	err := system.Backend.RefreshChangedInventory()
	if err != nil {
		system.Logger.Error("Failed to update disk cache: %s", err)
	}
}

// UpdateDiskCacheNow updates the disk cache immediately without registering a shutdown hook.
// Use this for non-deferred calls where you want to update the cache right away.
func UpdateDiskCacheNow(system *System) {
	updateDiskCacheDo(system)
}

// UpdateDiskCache registers a disk cache update to run on both normal exit (via defer)
// and on Ctrl+C interrupt (via shutdown handler). It returns a function that should be deferred.
//
// Usage:
//
//	defer UpdateDiskCache(system)()
//
// This ensures disk cache is updated whether the command completes normally or is interrupted.
func UpdateDiskCache(system *System) func() {
	if system == nil || !system.Opts.Config.Backend.InventoryCache {
		return func() {}
	}
	shutdown.AddLateCleanupJob("disk-cache-update", func(isSignal bool) {
		updateDiskCacheDo(system)
	})
	return func() {
		// Remove the cleanup job to prevent double execution on normal exit
		shutdown.DeleteLateCleanupJob("disk-cache-update")
		updateDiskCacheDo(system)
	}
}

// shellEscape escapes a string for safe use in a shell command.
// Uses single quotes and escapes embedded single quotes as '\”
func shellEscape(s string) string {
	// If the string is empty, return empty quoted string
	if s == "" {
		return "''"
	}

	// Check if escaping is needed
	needsQuoting := false
	for _, c := range s {
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' ||
			c == '$' || c == '`' || c == '\\' || c == '"' ||
			c == '\'' || c == '!' || c == '*' || c == '?' ||
			c == '[' || c == ']' || c == '{' || c == '}' ||
			c == '(' || c == ')' || c == '<' || c == '>' ||
			c == '|' || c == '&' || c == ';' || c == '#' ||
			c == '~' {
			needsQuoting = true
			break
		}
	}

	if !needsQuoting {
		return s
	}

	// Use single quotes and escape embedded single quotes as '\''
	var result strings.Builder
	result.WriteByte('\'')
	for _, c := range s {
		if c == '\'' {
			result.WriteString("'\\''")
		} else {
			result.WriteRune(c)
		}
	}
	result.WriteByte('\'')
	return result.String()
}

// ReconstructCommandLine builds a command line string from a command struct using reflection.
// It reads struct tags (short, long, default) to format flags appropriately.
//
// Parameters:
//   - cmdPath: the command path components (e.g., []string{"cluster", "create"})
//   - cmdStruct: the command struct instance with current values
//   - preferShort: if true, use short flags (-n) when available; otherwise use long flags (--name)
//   - includeDefaults: if true, include flags even when their value matches the default
//
// Returns a shell-safe command string like: aerolab cluster create --name=mydc --count=3
func ReconstructCommandLine(cmdPath []string, cmdStruct any, preferShort bool, includeDefaults ...bool) string {
	showDefaults := len(includeDefaults) > 0 && includeDefaults[0]
	return ReconstructCommandLineForBackend(cmdPath, cmdStruct, preferShort, showDefaults, "")
}

// ReconstructCommandLineForBackend is like ReconstructCommandLine but also filters out
// parameters belonging to non-active backends. Groups with description:"backend-X" where
// X does not match activeBackend are omitted from the output.
// If activeBackend is empty, no backend filtering is applied.
func ReconstructCommandLineForBackend(cmdPath []string, cmdStruct any, preferShort bool, includeDefaults bool, activeBackend string) string {
	var parts []string
	parts = append(parts, "aerolab")
	parts = append(parts, cmdPath...)

	flags := extractFlags(reflect.ValueOf(cmdStruct), "", preferShort, includeDefaults, activeBackend)
	parts = append(parts, flags...)

	return strings.Join(parts, " ")
}

// extractFlags recursively extracts flags from a struct value.
// activeBackend filters out group structs whose description tag contains "backend-X"
// where X does not match the active backend. Pass "" to disable filtering.
func extractFlags(val reflect.Value, namespacePrefix string, preferShort bool, includeDefaults bool, activeBackend string) []string {
	var flags []string

	// Handle pointer types
	if val.Kind() == reflect.Pointer {
		if val.IsNil() {
			return flags
		}
		val = val.Elem()
	}

	// Must be a struct
	if val.Kind() != reflect.Struct {
		return flags
	}

	typ := val.Type()

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		fieldVal := val.Field(i)

		// Skip unexported fields
		if field.PkgPath != "" {
			continue
		}

		// Skip command and help fields
		if field.Tag.Get("command") != "" {
			continue
		}

		// Handle groups/namespaces (nested structs)
		if field.Tag.Get("group") != "" {
			// Filter out non-active backend groups.
			// Backend groups have description:"backend-X" (e.g. "backend-aws").
			if activeBackend != "" {
				desc := field.Tag.Get("description")
				if after, ok := strings.CutPrefix(desc, "backend-"); ok {
					backendName := after
					if backendName != activeBackend {
						continue // Skip this entire group
					}
				}
			}

			namespace := field.Tag.Get("namespace")
			newPrefix := namespacePrefix
			if namespace != "" {
				if newPrefix != "" {
					newPrefix = newPrefix + "-" + namespace
				} else {
					newPrefix = namespace
				}
			}
			nestedFlags := extractFlags(fieldVal, newPrefix, preferShort, includeDefaults, activeBackend)
			flags = append(flags, nestedFlags...)
			continue
		}

		// Handle embedded structs (no group tag but is a struct)
		if field.Anonymous && fieldVal.Kind() == reflect.Struct {
			nestedFlags := extractFlags(fieldVal, namespacePrefix, preferShort, includeDefaults, activeBackend)
			flags = append(flags, nestedFlags...)
			continue
		}

		// Get flag names
		longName := field.Tag.Get("long")
		shortName := field.Tag.Get("short")
		defaultVal := field.Tag.Get("default")

		// Skip if no flag names
		if longName == "" && shortName == "" {
			continue
		}

		// Apply namespace prefix to long name
		if namespacePrefix != "" && longName != "" {
			longName = namespacePrefix + "-" + longName
		}

		// Get the current value as string and check if it differs from default
		valueStr, shouldInclude := getFieldValueString(fieldVal, defaultVal)
		if !shouldInclude && !includeDefaults {
			continue
		}

		// When includeDefaults is set but getFieldValueString returned false,
		// we still need the value string for the flag
		if !shouldInclude && includeDefaults {
			if fieldVal.Kind() == reflect.Bool {
				// For booleans in defaults mode, show explicit =true or =false
				valueStr = fmt.Sprintf("%t", fieldVal.Bool())
			} else {
				valueStr = getFieldValueStringForced(fieldVal)
			}
		}

		// Build the flag string
		var flagStr string
		if includeDefaults && fieldVal.Kind() == reflect.Bool {
			// When showing defaults, render booleans with explicit value (--flag=true / --flag=false)
			flagStr = buildFlagStringExplicit(shortName, longName, valueStr, preferShort)
		} else {
			flagStr = buildFlagString(shortName, longName, valueStr, fieldVal, preferShort)
		}
		if flagStr != "" {
			flags = append(flags, flagStr)
		}
	}

	return flags
}

// getFieldValueStringForced returns the string representation of a field value
// without checking against defaults. Used when includeDefaults is true.
func getFieldValueStringForced(fieldVal reflect.Value) string {
	if fieldVal.Kind() == reflect.Pointer {
		if fieldVal.IsNil() {
			return ""
		}
		fieldVal = fieldVal.Elem()
	}
	switch fieldVal.Kind() {
	case reflect.Bool:
		if fieldVal.Bool() {
			return ""
		}
		return ""
	case reflect.String:
		return fieldVal.String()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if fieldVal.Type().String() == "time.Duration" {
			return time.Duration(fieldVal.Int()).String()
		}
		return strconv.FormatInt(fieldVal.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(fieldVal.Uint(), 10)
	case reflect.Float32, reflect.Float64:
		return strconv.FormatFloat(fieldVal.Float(), 'f', -1, 64)
	case reflect.Slice:
		var elems []string
		for j := 0; j < fieldVal.Len(); j++ {
			elems = append(elems, fmt.Sprintf("%v", fieldVal.Index(j).Interface()))
		}
		return strings.Join(elems, ",")
	default:
		return fmt.Sprintf("%v", fieldVal.Interface())
	}
}

// getFieldValueString converts a field value to string and determines if it should be included
// Returns the string representation and whether the flag should be included in output
func getFieldValueString(fieldVal reflect.Value, defaultVal string) (string, bool) {
	// Handle pointer types
	if fieldVal.Kind() == reflect.Pointer {
		if fieldVal.IsNil() {
			return "", false
		}
		fieldVal = fieldVal.Elem()
	}

	switch fieldVal.Kind() {
	case reflect.Bool:
		boolVal := fieldVal.Bool()
		// For bool flags, only include if true (and default is not "true")
		if boolVal {
			defaultBool := defaultVal == "true"
			if !defaultBool {
				return "", true // Boolean flags don't need a value
			}
		}
		return "", false

	case reflect.String:
		strVal := fieldVal.String()
		if strVal == defaultVal {
			return "", false
		}
		return strVal, true

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		// Check if it's a time.Duration
		if fieldVal.Type().String() == "time.Duration" {
			durVal := time.Duration(fieldVal.Int())
			durStr := durVal.String()
			// Compare parsed Duration values to avoid format mismatches
			// (e.g. "30h0m0s" vs tag default "30h" represent the same duration)
			if defaultVal == "" && durVal == 0 {
				return "", false
			}
			if defaultVal != "" {
				if defaultDur, err := time.ParseDuration(defaultVal); err == nil {
					if durVal == defaultDur {
						return "", false
					}
				} else if durStr == defaultVal {
					return "", false
				}
			}
			return durStr, true
		}
		intVal := fieldVal.Int()
		intStr := strconv.FormatInt(intVal, 10)
		// When no default tag is set, the Go zero value is the implicit default
		if defaultVal == "" && intVal == 0 {
			return "", false
		}
		if intStr == defaultVal {
			return "", false
		}
		return intStr, true

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		uintVal := fieldVal.Uint()
		uintStr := strconv.FormatUint(uintVal, 10)
		// When no default tag is set, the Go zero value is the implicit default
		if defaultVal == "" && uintVal == 0 {
			return "", false
		}
		if uintStr == defaultVal {
			return "", false
		}
		return uintStr, true

	case reflect.Float32, reflect.Float64:
		floatVal := fieldVal.Float()
		floatStr := strconv.FormatFloat(floatVal, 'f', -1, 64)
		// When no default tag is set, the Go zero value is the implicit default
		if defaultVal == "" && floatVal == 0 {
			return "", false
		}
		if floatStr == defaultVal {
			return "", false
		}
		return floatStr, true

	case reflect.Slice:
		// Handle []string and similar slice types
		if fieldVal.Len() == 0 {
			return "", false
		}
		// Check if a single-element slice matches the tag default value.
		// go-flags initializes []string fields with default:"X" as ["X"].
		if defaultVal != "" && fieldVal.Len() == 1 {
			elem := fieldVal.Index(0)
			elemStr := fmt.Sprintf("%v", elem.Interface())
			if elemStr == defaultVal {
				return "", false
			}
		}
		// For slices, we'll return a special marker and handle it in buildFlagString
		return "<<SLICE>>", true

	default:
		// For other types, try to use Stringer interface
		if stringer, ok := fieldVal.Interface().(fmt.Stringer); ok {
			strVal := stringer.String()
			if strVal == defaultVal {
				return "", false
			}
			return strVal, true
		}
		// Fallback: use %v formatting
		strVal := fmt.Sprintf("%v", fieldVal.Interface())
		if strVal == defaultVal {
			return "", false
		}
		return strVal, true
	}
}

// buildFlagString creates the flag string with proper formatting
func buildFlagString(shortName, longName, valueStr string, fieldVal reflect.Value, preferShort bool) string {
	// Handle slice types specially - emit multiple flags
	if valueStr == "<<SLICE>>" {
		return buildSliceFlags(shortName, longName, fieldVal, preferShort)
	}

	// Choose flag format
	var flagName string
	if preferShort && shortName != "" {
		flagName = "-" + shortName
	} else if longName != "" {
		flagName = "--" + longName
	} else if shortName != "" {
		flagName = "-" + shortName
	} else {
		return ""
	}

	// Boolean flags don't need a value
	if fieldVal.Kind() == reflect.Bool {
		return flagName
	}

	// Other flags need a value
	escapedValue := shellEscape(valueStr)
	return flagName + "=" + escapedValue
}

// buildFlagStringExplicit creates a flag string with an explicit value (used for booleans in includeDefaults mode)
func buildFlagStringExplicit(shortName, longName, valueStr string, preferShort bool) string {
	var flagName string
	if preferShort && shortName != "" {
		flagName = "-" + shortName
	} else if longName != "" {
		flagName = "--" + longName
	} else if shortName != "" {
		flagName = "-" + shortName
	} else {
		return ""
	}
	return flagName + "=" + valueStr
}

// buildSliceFlags builds flag strings for slice fields (multiple values)
func buildSliceFlags(shortName, longName string, fieldVal reflect.Value, preferShort bool) string {
	if fieldVal.Len() == 0 {
		return ""
	}

	var flagParts []string
	var flagName string
	if preferShort && shortName != "" {
		flagName = "-" + shortName
	} else if longName != "" {
		flagName = "--" + longName
	} else if shortName != "" {
		flagName = "-" + shortName
	} else {
		return ""
	}

	for i := 0; i < fieldVal.Len(); i++ {
		elem := fieldVal.Index(i)
		var elemStr string

		// Handle pointer elements
		if elem.Kind() == reflect.Pointer {
			if elem.IsNil() {
				continue
			}
			elem = elem.Elem()
		}

		// Get string representation of element
		switch elem.Kind() {
		case reflect.String:
			elemStr = elem.String()
		default:
			if stringer, ok := elem.Interface().(fmt.Stringer); ok {
				elemStr = stringer.String()
			} else {
				elemStr = fmt.Sprintf("%v", elem.Interface())
			}
		}

		escapedValue := shellEscape(elemStr)
		flagParts = append(flagParts, flagName+"="+escapedValue)
	}

	return strings.Join(flagParts, " ")
}
