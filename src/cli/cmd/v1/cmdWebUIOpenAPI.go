package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// OpenAPI spec structures (OpenAPI 3.0.3)

// OpenAPISpec represents the root OpenAPI document
type OpenAPISpec struct {
	OpenAPI    string                     `json:"openapi"`
	Info       OpenAPIInfo                `json:"info"`
	Servers    []OpenAPIServer            `json:"servers,omitempty"`
	Paths      map[string]OpenAPIPathItem `json:"paths"`
	Components OpenAPIComponents          `json:"components"`
	Tags       []OpenAPITag               `json:"tags,omitempty"`
}

// OpenAPIInfo contains API metadata
type OpenAPIInfo struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Version     string `json:"version"`
}

// OpenAPIServer represents a server URL
type OpenAPIServer struct {
	URL         string `json:"url"`
	Description string `json:"description,omitempty"`
}

// OpenAPITag represents a tag for grouping operations
type OpenAPITag struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// OpenAPIPathItem represents operations on a path
type OpenAPIPathItem struct {
	Get    *OpenAPIOperation `json:"get,omitempty"`
	Post   *OpenAPIOperation `json:"post,omitempty"`
	Put    *OpenAPIOperation `json:"put,omitempty"`
	Delete *OpenAPIOperation `json:"delete,omitempty"`
}

// OpenAPIOperation represents a single API operation
type OpenAPIOperation struct {
	Tags        []string                   `json:"tags,omitempty"`
	Summary     string                     `json:"summary,omitempty"`
	Description string                     `json:"description,omitempty"`
	OperationID string                     `json:"operationId,omitempty"`
	Parameters  []OpenAPIParameter         `json:"parameters,omitempty"`
	RequestBody *OpenAPIRequestBody        `json:"requestBody,omitempty"`
	Responses   map[string]OpenAPIResponse `json:"responses"`
	Security    []map[string][]string      `json:"security,omitempty"`
}

// OpenAPIParameter represents an operation parameter
type OpenAPIParameter struct {
	Name        string         `json:"name"`
	In          string         `json:"in"` // query, header, path, cookie
	Description string         `json:"description,omitempty"`
	Required    bool           `json:"required,omitempty"`
	Schema      *OpenAPISchema `json:"schema,omitempty"`
}

// OpenAPIRequestBody represents a request body
type OpenAPIRequestBody struct {
	Description string                      `json:"description,omitempty"`
	Required    bool                        `json:"required,omitempty"`
	Content     map[string]OpenAPIMediaType `json:"content"`
}

// OpenAPIMediaType represents a media type
type OpenAPIMediaType struct {
	Schema *OpenAPISchema `json:"schema,omitempty"`
}

// OpenAPIResponse represents an API response
type OpenAPIResponse struct {
	Description string                      `json:"description"`
	Content     map[string]OpenAPIMediaType `json:"content,omitempty"`
}

// OpenAPISchema represents a JSON Schema
type OpenAPISchema struct {
	Type        string                    `json:"type,omitempty"`
	Format      string                    `json:"format,omitempty"`
	Description string                    `json:"description,omitempty"`
	Default     interface{}               `json:"default,omitempty"`
	Enum        []string                  `json:"enum,omitempty"`
	Items       *OpenAPISchema            `json:"items,omitempty"`
	Properties  map[string]*OpenAPISchema `json:"properties,omitempty"`
	Required    []string                  `json:"required,omitempty"`
	Ref         string                    `json:"$ref,omitempty"`
	OneOf       []*OpenAPISchema          `json:"oneOf,omitempty"`
}

// OpenAPIComponents contains reusable components
type OpenAPIComponents struct {
	Schemas         map[string]*OpenAPISchema         `json:"schemas,omitempty"`
	SecuritySchemes map[string]*OpenAPISecurityScheme `json:"securitySchemes,omitempty"`
}

// OpenAPISecurityScheme represents a security scheme
type OpenAPISecurityScheme struct {
	Type         string `json:"type"`
	Scheme       string `json:"scheme,omitempty"`
	BearerFormat string `json:"bearerFormat,omitempty"`
	Name         string `json:"name,omitempty"`
	In           string `json:"in,omitempty"`
	Description  string `json:"description,omitempty"`
}

// handleOpenAPI handles GET /api/openapi - returns the OpenAPI spec
func (c *WebUICmd) handleOpenAPI(w http.ResponseWriter, r *http.Request) {
	if !c.checkAuth(w, r) {
		return
	}

	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	spec := c.generateOpenAPISpec(r)

	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(spec)
}

// generateOpenAPISpec generates the complete OpenAPI specification
func (c *WebUICmd) generateOpenAPISpec(r *http.Request) *OpenAPISpec {
	// Build server URL from request
	scheme := "http"
	if c.HTTPS || r.TLS != nil {
		scheme = "https"
	}
	serverURL := fmt.Sprintf("%s://%s%s", scheme, r.Host, c.rootPath)

	spec := &OpenAPISpec{
		OpenAPI: "3.0.3",
		Info: OpenAPIInfo{
			Title:       "AeroLab REST API",
			Description: "REST API for AeroLab - Aerospike Lab Management Tool. This API provides programmatic access to all aerolab commands, enabling command exploration, asynchronous execution with job tracking, file upload/download, and log streaming.",
			Version:     getVersion(),
		},
		Servers: []OpenAPIServer{
			{URL: serverURL, Description: "Current server"},
		},
		Paths: make(map[string]OpenAPIPathItem),
		Components: OpenAPIComponents{
			Schemas:         make(map[string]*OpenAPISchema),
			SecuritySchemes: make(map[string]*OpenAPISecurityScheme),
		},
		Tags: []OpenAPITag{
			{Name: "exploration", Description: "Command exploration and discovery"},
			{Name: "execution", Description: "Command execution"},
			{Name: "jobs", Description: "Job management and monitoring"},
			{Name: "health", Description: "Health and status checks"},
		},
	}

	// Add security schemes based on configuration
	if c.isBasicAuth {
		spec.Components.SecuritySchemes["basicAuth"] = &OpenAPISecurityScheme{
			Type:        "http",
			Scheme:      "basic",
			Description: "HTTP Basic Authentication",
		}
	}
	if c.isTokenAuth {
		spec.Components.SecuritySchemes["tokenAuth"] = &OpenAPISecurityScheme{
			Type:        "apiKey",
			In:          "header",
			Name:        "X-Auth-Token",
			Description: "Token-based authentication via X-Auth-Token header",
		}
	}

	// Add common schemas
	c.addCommonSchemas(spec)

	// Add static API endpoints
	c.addHealthEndpoint(spec)
	c.addExplorationEndpoints(spec)
	c.addJobEndpoints(spec)
	c.addGenerateCLIEndpoint(spec)
	c.addOpenAPIEndpoint(spec)

	// Add command endpoints recursively
	c.addCommandEndpoints(spec, c.commandTree)

	return spec
}

// addCommonSchemas adds reusable schemas to the spec
func (c *WebUICmd) addCommonSchemas(spec *OpenAPISpec) {
	// Job status enum
	spec.Components.Schemas["JobStatus"] = &OpenAPISchema{
		Type:        "string",
		Enum:        []string{"pending", "running", "completed", "error", "failed"},
		Description: "Current status of a job",
	}

	// Job schema
	spec.Components.Schemas["Job"] = &OpenAPISchema{
		Type: "object",
		Properties: map[string]*OpenAPISchema{
			"id":               {Type: "string", Description: "Unique job identifier"},
			"user":             {Type: "string", Description: "User who submitted the job"},
			"commandPath":      {Type: "string", Description: "Path of the command being executed"},
			"parameters":       {Type: "object", Description: "Parameters passed to the command"},
			"cliCommand":       {Type: "string", Description: "Equivalent CLI command"},
			"status":           {Ref: "#/components/schemas/JobStatus"},
			"createdAt":        {Type: "string", Format: "date-time", Description: "Job creation timestamp"},
			"startedAt":        {Type: "string", Format: "date-time", Description: "Job start timestamp"},
			"completedAt":      {Type: "string", Format: "date-time", Description: "Job completion timestamp"},
			"error":            {Type: "string", Description: "Error message if job failed"},
			"pid":              {Type: "integer", Description: "Process ID of the running job"},
			"exitCode":         {Type: "integer", Description: "Exit code of completed job"},
			"cancelled":        {Type: "boolean", Description: "Whether the job was cancelled"},
			"timedOut":         {Type: "boolean", Description: "Whether the job timed out"},
			"refreshInventory": {Type: "boolean", Description: "Whether to refresh inventory after completion"},
		},
	}

	// Job submit response
	spec.Components.Schemas["JobSubmitResponse"] = &OpenAPISchema{
		Type: "object",
		Properties: map[string]*OpenAPISchema{
			"jobId":         {Type: "string", Description: "Unique job identifier"},
			"user":          {Type: "string", Description: "User who submitted the job"},
			"commandPath":   {Type: "string", Description: "Path of the command"},
			"cliCommand":    {Type: "string", Description: "Equivalent CLI command"},
			"status":        {Ref: "#/components/schemas/JobStatus"},
			"createdAt":     {Type: "string", Format: "date-time"},
			"statusUrl":     {Type: "string", Format: "uri", Description: "URL to check job status"},
			"logsUrl":       {Type: "string", Format: "uri", Description: "URL to get job logs"},
			"logsStreamUrl": {Type: "string", Format: "uri", Description: "URL to stream job logs via SSE"},
		},
	}

	// Job list response
	spec.Components.Schemas["JobListResponse"] = &OpenAPISchema{
		Type: "object",
		Properties: map[string]*OpenAPISchema{
			"jobs":  {Type: "array", Items: &OpenAPISchema{Ref: "#/components/schemas/Job"}},
			"count": {Type: "integer", Description: "Total number of jobs"},
		},
	}

	// Command info
	spec.Components.Schemas["CommandInfo"] = &OpenAPISchema{
		Type: "object",
		Properties: map[string]*OpenAPISchema{
			"name":        {Type: "string", Description: "Command name"},
			"path":        {Type: "string", Description: "Full command path"},
			"description": {Type: "string", Description: "Command description"},
			"icon":        {Type: "string", Description: "Icon identifier for web UI"},
			"hidden":      {Type: "boolean", Description: "Whether command is hidden"},
			"webHidden":   {Type: "boolean", Description: "Whether command is hidden in web UI"},
			"simpleMode":  {Type: "boolean", Description: "Whether to show in simple mode"},
			"hasChildren": {Type: "boolean", Description: "Whether command has subcommands"},
			"children":    {Type: "array", Items: &OpenAPISchema{Ref: "#/components/schemas/CommandInfo"}},
			"parameters":  {Type: "array", Items: &OpenAPISchema{Ref: "#/components/schemas/ParameterInfo"}},
		},
	}

	// Parameter info
	spec.Components.Schemas["ParameterInfo"] = &OpenAPISchema{
		Type: "object",
		Properties: map[string]*OpenAPISchema{
			"name":          {Type: "string", Description: "Parameter name"},
			"fieldName":     {Type: "string", Description: "Go struct field name"},
			"short":         {Type: "string", Description: "Short flag (single character)"},
			"long":          {Type: "string", Description: "Long flag name"},
			"description":   {Type: "string", Description: "Parameter description"},
			"type":          {Type: "string", Description: "Parameter type (string, int, bool, etc.)"},
			"default":       {Type: "string", Description: "Default value"},
			"required":      {Type: "boolean", Description: "Whether parameter is required"},
			"webType":       {Type: "string", Description: "Web UI input type"},
			"choices":       {Type: "array", Items: &OpenAPISchema{Type: "string"}, Description: "Available choices"},
			"choicesMethod": {Type: "string", Description: "Method name for dynamic choices"},
			"hidden":        {Type: "boolean", Description: "Whether parameter is hidden"},
			"webHidden":     {Type: "boolean", Description: "Whether parameter is hidden in web UI"},
			"simpleMode":    {Type: "boolean", Description: "Whether to show in simple mode"},
			"group":         {Type: "string", Description: "Parameter group name"},
			"namespace":     {Type: "string", Description: "Parameter namespace"},
			"noDefault":     {Type: "boolean", Description: "Whether to omit default value"},
			"isSlice":       {Type: "boolean", Description: "Whether parameter accepts multiple values"},
			"isPositional":  {Type: "boolean", Description: "Whether parameter is positional"},
		},
	}

	// Error response
	spec.Components.Schemas["ErrorResponse"] = &OpenAPISchema{
		Type: "object",
		Properties: map[string]*OpenAPISchema{
			"error": {Type: "string", Description: "Error message"},
		},
	}

	// Health response
	spec.Components.Schemas["HealthResponse"] = &OpenAPISchema{
		Type: "object",
		Properties: map[string]*OpenAPISchema{
			"status":  {Type: "string", Description: "Health status"},
			"version": {Type: "string", Description: "API version"},
		},
	}
}

// addHealthEndpoint adds the health check endpoint
func (c *WebUICmd) addHealthEndpoint(spec *OpenAPISpec) {
	spec.Paths["/api/health"] = OpenAPIPathItem{
		Get: &OpenAPIOperation{
			Tags:        []string{"health"},
			Summary:     "Health check",
			Description: "Returns the health status and version of the API",
			OperationID: "getHealth",
			Responses: map[string]OpenAPIResponse{
				"200": {
					Description: "Server is healthy",
					Content: map[string]OpenAPIMediaType{
						"application/json": {Schema: &OpenAPISchema{Ref: "#/components/schemas/HealthResponse"}},
					},
				},
			},
		},
	}
}

// addExplorationEndpoints adds command exploration endpoints
func (c *WebUICmd) addExplorationEndpoints(spec *OpenAPISpec) {
	// GET /api/commands
	spec.Paths["/api/commands"] = OpenAPIPathItem{
		Get: &OpenAPIOperation{
			Tags:        []string{"exploration"},
			Summary:     "List all commands",
			Description: "Returns the complete command tree with all available commands, subcommands, and their parameters",
			OperationID: "listCommands",
			Security:    c.getSecurityRequirement(),
			Responses: map[string]OpenAPIResponse{
				"200": {
					Description: "Command tree",
					Content: map[string]OpenAPIMediaType{
						"application/json": {Schema: &OpenAPISchema{Ref: "#/components/schemas/CommandInfo"}},
					},
				},
				"401": {Description: "Unauthorized"},
			},
		},
	}

	// GET /api/commands/{path}
	spec.Paths["/api/commands/{path}"] = OpenAPIPathItem{
		Get: &OpenAPIOperation{
			Tags:        []string{"exploration"},
			Summary:     "Get command details",
			Description: "Returns detailed information about a specific command including its parameters and subcommands",
			OperationID: "getCommand",
			Security:    c.getSecurityRequirement(),
			Parameters: []OpenAPIParameter{
				{
					Name:        "path",
					In:          "path",
					Required:    true,
					Description: "Command path (e.g., cluster/create)",
					Schema:      &OpenAPISchema{Type: "string"},
				},
			},
			Responses: map[string]OpenAPIResponse{
				"200": {
					Description: "Command details",
					Content: map[string]OpenAPIMediaType{
						"application/json": {Schema: &OpenAPISchema{Ref: "#/components/schemas/CommandInfo"}},
					},
				},
				"401": {Description: "Unauthorized"},
				"404": {Description: "Command not found"},
			},
		},
	}
}

// addJobEndpoints adds job management endpoints
func (c *WebUICmd) addJobEndpoints(spec *OpenAPISpec) {
	// GET /api/jobs
	spec.Paths["/api/jobs"] = OpenAPIPathItem{
		Get: &OpenAPIOperation{
			Tags:        []string{"jobs"},
			Summary:     "List jobs",
			Description: "Returns a list of jobs with optional filtering by status",
			OperationID: "listJobs",
			Security:    c.getSecurityRequirement(),
			Parameters: []OpenAPIParameter{
				{
					Name:        "status",
					In:          "query",
					Description: "Filter by job status",
					Schema:      &OpenAPISchema{Ref: "#/components/schemas/JobStatus"},
				},
				{
					Name:        "all",
					In:          "query",
					Description: "Show jobs from all users (admin only)",
					Schema:      &OpenAPISchema{Type: "boolean"},
				},
			},
			Responses: map[string]OpenAPIResponse{
				"200": {
					Description: "List of jobs",
					Content: map[string]OpenAPIMediaType{
						"application/json": {Schema: &OpenAPISchema{Ref: "#/components/schemas/JobListResponse"}},
					},
				},
				"401": {Description: "Unauthorized"},
			},
		},
	}

	// GET/DELETE /api/jobs/{jobId}
	spec.Paths["/api/jobs/{jobId}"] = OpenAPIPathItem{
		Get: &OpenAPIOperation{
			Tags:        []string{"jobs"},
			Summary:     "Get job details",
			Description: "Returns detailed information about a specific job",
			OperationID: "getJob",
			Security:    c.getSecurityRequirement(),
			Parameters: []OpenAPIParameter{
				{
					Name:        "jobId",
					In:          "path",
					Required:    true,
					Description: "Job identifier",
					Schema:      &OpenAPISchema{Type: "string"},
				},
				{
					Name:        "all",
					In:          "query",
					Description: "Allow viewing jobs from other users (admin only)",
					Schema:      &OpenAPISchema{Type: "boolean"},
				},
			},
			Responses: map[string]OpenAPIResponse{
				"200": {
					Description: "Job details",
					Content: map[string]OpenAPIMediaType{
						"application/json": {Schema: &OpenAPISchema{Ref: "#/components/schemas/Job"}},
					},
				},
				"401": {Description: "Unauthorized"},
				"404": {Description: "Job not found"},
			},
		},
		Delete: &OpenAPIOperation{
			Tags:        []string{"jobs"},
			Summary:     "Cancel job",
			Description: "Cancels a running job by sending SIGTERM (or SIGKILL with force=true)",
			OperationID: "cancelJob",
			Security:    c.getSecurityRequirement(),
			Parameters: []OpenAPIParameter{
				{
					Name:        "jobId",
					In:          "path",
					Required:    true,
					Description: "Job identifier",
					Schema:      &OpenAPISchema{Type: "string"},
				},
				{
					Name:        "force",
					In:          "query",
					Description: "Send SIGKILL instead of SIGTERM",
					Schema:      &OpenAPISchema{Type: "boolean"},
				},
				{
					Name:        "all",
					In:          "query",
					Description: "Allow cancelling jobs from other users (admin only)",
					Schema:      &OpenAPISchema{Type: "boolean"},
				},
			},
			Responses: map[string]OpenAPIResponse{
				"200": {Description: "Job cancellation initiated"},
				"400": {Description: "Job is not running"},
				"401": {Description: "Unauthorized"},
				"404": {Description: "Job not found"},
			},
		},
	}

	// GET /api/jobs/{jobId}/logs
	spec.Paths["/api/jobs/{jobId}/logs"] = OpenAPIPathItem{
		Get: &OpenAPIOperation{
			Tags:        []string{"jobs"},
			Summary:     "Get job logs",
			Description: "Returns the complete log output of a job",
			OperationID: "getJobLogs",
			Security:    c.getSecurityRequirement(),
			Parameters: []OpenAPIParameter{
				{
					Name:        "jobId",
					In:          "path",
					Required:    true,
					Description: "Job identifier",
					Schema:      &OpenAPISchema{Type: "string"},
				},
				{
					Name:        "all",
					In:          "query",
					Description: "Allow viewing logs from other users' jobs (admin only)",
					Schema:      &OpenAPISchema{Type: "boolean"},
				},
			},
			Responses: map[string]OpenAPIResponse{
				"200": {
					Description: "Job logs",
					Content: map[string]OpenAPIMediaType{
						"application/json": {
							Schema: &OpenAPISchema{
								Type: "object",
								Properties: map[string]*OpenAPISchema{
									"jobId":  {Type: "string"},
									"status": {Ref: "#/components/schemas/JobStatus"},
									"logs":   {Type: "string"},
								},
							},
						},
					},
				},
				"401": {Description: "Unauthorized"},
				"404": {Description: "Job not found"},
			},
		},
	}

	// GET /api/jobs/{jobId}/logs/stream
	spec.Paths["/api/jobs/{jobId}/logs/stream"] = OpenAPIPathItem{
		Get: &OpenAPIOperation{
			Tags:        []string{"jobs"},
			Summary:     "Stream job logs",
			Description: "Streams job logs in real-time using Server-Sent Events (SSE). Events include 'data' for log lines, 'status' for status updates, 'error' for errors, and 'complete' when the job finishes.",
			OperationID: "streamJobLogs",
			Security:    c.getSecurityRequirement(),
			Parameters: []OpenAPIParameter{
				{
					Name:        "jobId",
					In:          "path",
					Required:    true,
					Description: "Job identifier",
					Schema:      &OpenAPISchema{Type: "string"},
				},
				{
					Name:        "all",
					In:          "query",
					Description: "Allow streaming logs from other users' jobs (admin only)",
					Schema:      &OpenAPISchema{Type: "boolean"},
				},
			},
			Responses: map[string]OpenAPIResponse{
				"200": {
					Description: "SSE stream of log events",
					Content: map[string]OpenAPIMediaType{
						"text/event-stream": {
							Schema: &OpenAPISchema{Type: "string", Description: "Server-Sent Events stream"},
						},
					},
				},
				"401": {Description: "Unauthorized"},
				"404": {Description: "Job not found"},
			},
		},
	}
}

// addGenerateCLIEndpoint adds the generate-cli endpoint
func (c *WebUICmd) addGenerateCLIEndpoint(spec *OpenAPISpec) {
	spec.Paths["/api/generate-cli"] = OpenAPIPathItem{
		Post: &OpenAPIOperation{
			Tags:        []string{"execution"},
			Summary:     "Generate CLI command",
			Description: "Generates the equivalent aerolab CLI command for given parameters without executing it. Uses reflection to properly handle default values, nested parameter groups, and shell escaping. Parameters that match their default values are omitted from the output.",
			OperationID: "generateCLI",
			Security:    c.getSecurityRequirement(),
			RequestBody: &OpenAPIRequestBody{
				Required: true,
				Content: map[string]OpenAPIMediaType{
					"application/json": {
						Schema: &OpenAPISchema{
							Type: "object",
							Properties: map[string]*OpenAPISchema{
								"commandPath": {Type: "string", Description: "Command path (e.g., cluster/create)"},
								"parameters":  {Type: "object", Description: "Command parameters as key-value pairs"},
								"preferShort": {Type: "boolean", Description: "If true, use short flags (-n) instead of long flags (--name) where available. Default: false"},
							},
							Required: []string{"commandPath"},
						},
					},
				},
			},
			Responses: map[string]OpenAPIResponse{
				"200": {
					Description: "Generated CLI command",
					Content: map[string]OpenAPIMediaType{
						"application/json": {
							Schema: &OpenAPISchema{
								Type: "object",
								Properties: map[string]*OpenAPISchema{
									"cli": {Type: "string", Description: "The generated CLI command with proper shell escaping"},
								},
							},
						},
					},
				},
				"400": {Description: "Invalid request"},
				"401": {Description: "Unauthorized"},
				"404": {Description: "Command not found"},
			},
		},
	}
}

// addOpenAPIEndpoint adds the openapi spec endpoint itself
func (c *WebUICmd) addOpenAPIEndpoint(spec *OpenAPISpec) {
	spec.Paths["/api/openapi"] = OpenAPIPathItem{
		Get: &OpenAPIOperation{
			Tags:        []string{"exploration"},
			Summary:     "Get OpenAPI specification",
			Description: "Returns this OpenAPI specification document",
			OperationID: "getOpenAPISpec",
			Security:    c.getSecurityRequirement(),
			Responses: map[string]OpenAPIResponse{
				"200": {
					Description: "OpenAPI specification",
					Content: map[string]OpenAPIMediaType{
						"application/json": {
							Schema: &OpenAPISchema{Type: "object", Description: "OpenAPI 3.0.3 specification"},
						},
					},
				},
				"401": {Description: "Unauthorized"},
			},
		},
	}
}

// addCommandEndpoints recursively adds endpoints for all commands
func (c *WebUICmd) addCommandEndpoints(spec *OpenAPISpec, cmd *CommandInfo) {
	// Skip hidden commands and the root
	if cmd.Hidden || cmd.WebHidden {
		return
	}

	// Add endpoint for this command if it has parameters (is executable)
	if len(cmd.Parameters) > 0 || !cmd.HasChildren {
		c.addCommandExecutionEndpoint(spec, cmd)
	}

	// Recurse into children
	for _, child := range cmd.Children {
		c.addCommandEndpoints(spec, child)
	}
}

// addCommandExecutionEndpoint adds an execution endpoint for a specific command
func (c *WebUICmd) addCommandExecutionEndpoint(spec *OpenAPISpec, cmd *CommandInfo) {
	if cmd.Path == "" {
		return
	}

	path := "/" + cmd.Path

	// Build request schema from parameters
	requestSchema := c.buildCommandRequestSchema(cmd)
	schemaName := c.sanitizeSchemaName(cmd.Path) + "Request"
	spec.Components.Schemas[schemaName] = requestSchema

	// Determine the tag from the first part of the path
	tag := strings.Split(cmd.Path, "/")[0]

	// Check if tag already exists
	tagExists := false
	for _, t := range spec.Tags {
		if t.Name == tag {
			tagExists = true
			break
		}
	}
	if !tagExists && tag != "" {
		spec.Tags = append(spec.Tags, OpenAPITag{
			Name:        tag,
			Description: fmt.Sprintf("Commands under %s", tag),
		})
	}

	pathItem := OpenAPIPathItem{
		// GET returns command info
		Get: &OpenAPIOperation{
			Tags:        []string{"exploration", tag},
			Summary:     fmt.Sprintf("Get %s command info", cmd.Name),
			Description: cmd.Description,
			OperationID: "get_" + c.sanitizeOperationID(cmd.Path),
			Security:    c.getSecurityRequirement(),
			Responses: map[string]OpenAPIResponse{
				"200": {
					Description: "Command information",
					Content: map[string]OpenAPIMediaType{
						"application/json": {Schema: &OpenAPISchema{Ref: "#/components/schemas/CommandInfo"}},
					},
				},
				"401": {Description: "Unauthorized"},
				"404": {Description: "Command not found"},
			},
		},
		// POST/PUT executes the command
		Post: &OpenAPIOperation{
			Tags:        []string{"execution", tag},
			Summary:     fmt.Sprintf("Execute %s", cmd.Name),
			Description: cmd.Description + "\n\nThis command is executed asynchronously. The response contains a job ID that can be used to monitor progress.\n\nSet `dryRun=true` query parameter to generate the equivalent CLI command without executing it.",
			OperationID: "execute_" + c.sanitizeOperationID(cmd.Path),
			Security:    c.getSecurityRequirement(),
			Parameters: []OpenAPIParameter{
				{
					Name:        "dryRun",
					In:          "query",
					Description: "If true, returns the equivalent CLI command without executing. Useful for previewing what would be run.",
					Schema:      &OpenAPISchema{Type: "boolean", Default: false},
				},
				{
					Name:        "preferShort",
					In:          "query",
					Description: "If true (and dryRun=true), use short flags (-n) instead of long flags (--name) in the generated CLI command.",
					Schema:      &OpenAPISchema{Type: "boolean", Default: false},
				},
			},
			RequestBody: &OpenAPIRequestBody{
				Description: "Command parameters",
				Content: map[string]OpenAPIMediaType{
					"application/json": {Schema: &OpenAPISchema{Ref: "#/components/schemas/" + schemaName}},
				},
			},
			Responses: map[string]OpenAPIResponse{
				"200": {
					Description: "Dry run result (when dryRun=true)",
					Content: map[string]OpenAPIMediaType{
						"application/json": {
							Schema: &OpenAPISchema{
								Type: "object",
								Properties: map[string]*OpenAPISchema{
									"dryRun":      {Type: "boolean", Description: "Always true for dry run responses"},
									"commandPath": {Type: "string", Description: "The command path"},
									"cli":         {Type: "string", Description: "The generated CLI command"},
									"parameters":  {Type: "object", Description: "The parameters that were provided"},
								},
							},
						},
					},
				},
				"202": {
					Description: "Job submitted successfully (when dryRun=false or not specified)",
					Content: map[string]OpenAPIMediaType{
						"application/json": {Schema: &OpenAPISchema{Ref: "#/components/schemas/JobSubmitResponse"}},
					},
				},
				"400": {Description: "Invalid parameters"},
				"401": {Description: "Unauthorized"},
				"404": {Description: "Command not found"},
			},
		},
	}

	spec.Paths[path] = pathItem
}

// buildCommandRequestSchema builds a JSON schema for a command's parameters
func (c *WebUICmd) buildCommandRequestSchema(cmd *CommandInfo) *OpenAPISchema {
	schema := &OpenAPISchema{
		Type:       "object",
		Properties: make(map[string]*OpenAPISchema),
		Required:   []string{},
	}

	for _, param := range cmd.Parameters {
		// Skip hidden and web-hidden parameters
		if param.Hidden || param.WebHidden {
			continue
		}

		propSchema := c.parameterToSchema(&param)
		schema.Properties[param.Name] = propSchema

		if param.Required {
			schema.Required = append(schema.Required, param.Name)
		}
	}

	// Remove required array if empty
	if len(schema.Required) == 0 {
		schema.Required = nil
	}

	return schema
}

// parameterToSchema converts a ParameterInfo to an OpenAPI schema
func (c *WebUICmd) parameterToSchema(param *ParameterInfo) *OpenAPISchema {
	schema := &OpenAPISchema{
		Description: param.Description,
	}

	// Set default if present
	if param.Default != "" && !param.NoDefault {
		schema.Default = param.Default
	}

	// Set choices/enum if available
	if len(param.Choices) > 0 {
		schema.Enum = param.Choices
	}

	// Determine type
	switch {
	case param.IsSlice:
		schema.Type = "array"
		elemType := strings.TrimPrefix(param.Type, "[]")
		schema.Items = &OpenAPISchema{Type: c.mapTypeToOpenAPI(elemType)}
	case param.Type == "bool":
		schema.Type = "boolean"
	case param.Type == "int" || param.Type == "uint":
		schema.Type = "integer"
	case param.Type == "float":
		schema.Type = "number"
	case param.Type == "duration":
		schema.Type = "string"
		schema.Description += " (e.g., '30s', '5m', '1h')"
	case param.Type == "object":
		schema.Type = "object"
	default:
		schema.Type = "string"
	}

	// Handle special web types
	switch param.WebType {
	case "upload":
		schema.Type = "string"
		schema.Format = "binary"
		schema.Description += " (file upload)"
	case "download":
		schema.Type = "string"
		schema.Description += " (file download path)"
	case "textarea":
		schema.Type = "string"
		schema.Description += " (multi-line text)"
	case "password":
		schema.Type = "string"
		schema.Format = "password"
	}

	return schema
}

// mapTypeToOpenAPI maps internal type names to OpenAPI types
func (c *WebUICmd) mapTypeToOpenAPI(typeName string) string {
	switch typeName {
	case "bool":
		return "boolean"
	case "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64":
		return "integer"
	case "float32", "float64":
		return "number"
	default:
		return "string"
	}
}

// sanitizeOperationID creates a valid operation ID from a path
func (c *WebUICmd) sanitizeOperationID(path string) string {
	return strings.ReplaceAll(path, "/", "_")
}

// sanitizeSchemaName creates a valid schema name from a path
func (c *WebUICmd) sanitizeSchemaName(path string) string {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if len(part) > 0 {
			parts[i] = strings.ToUpper(part[:1]) + part[1:]
		}
	}
	return strings.Join(parts, "")
}

// getSecurityRequirement returns the security requirement based on auth config
func (c *WebUICmd) getSecurityRequirement() []map[string][]string {
	if !c.isBasicAuth && !c.isTokenAuth {
		return nil // No auth required
	}

	requirements := []map[string][]string{}
	if c.isBasicAuth {
		requirements = append(requirements, map[string][]string{"basicAuth": {}})
	}
	if c.isTokenAuth {
		requirements = append(requirements, map[string][]string{"tokenAuth": {}})
	}
	return requirements
}
