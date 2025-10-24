package cloud

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"reflect"
)

// WebUI server configuration
type WebUIServer struct {
	Port   int    `long:"port" description:"Port to run the web UI on" default:"8080"`
	Host   string `long:"host" description:"Host to bind the web UI to" default:"localhost"`
	Client *Client
}

// CommandInfo represents a command and its parameters
type CommandInfo struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  []ParameterInfo `json:"parameters"`
	SubCommands []CommandInfo   `json:"subcommands,omitempty"`
	Path        string          `json:"path"`
	Method      string          `json:"method,omitempty"`
}

// ParameterInfo represents a command parameter
type ParameterInfo struct {
	Name        string `json:"name"`
	Short       string `json:"short,omitempty"`
	Long        string `json:"long,omitempty"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
	Default     string `json:"default,omitempty"`
	Type        string `json:"type"`
}

// APIResponse represents the response from an API call
type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// WebUI HTML template
const webUITemplate = `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Aerospike Cloud CLI - Web UI</title>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            min-height: 100vh;
            padding: 20px;
        }
        
        .container {
            max-width: 1200px;
            margin: 0 auto;
            background: white;
            border-radius: 12px;
            box-shadow: 0 20px 40px rgba(0,0,0,0.1);
            overflow: hidden;
        }
        
        .header {
            background: linear-gradient(135deg, #2c3e50 0%, #34495e 100%);
            color: white;
            padding: 30px;
            text-align: center;
        }
        
        .header h1 {
            font-size: 2.5em;
            margin-bottom: 10px;
            font-weight: 300;
        }
        
        .header p {
            opacity: 0.8;
            font-size: 1.1em;
        }
        
        .content {
            display: flex;
            min-height: 600px;
        }
        
        .sidebar {
            width: 300px;
            background: #f8f9fa;
            border-right: 1px solid #e9ecef;
            overflow-y: auto;
            position: relative;
            z-index: 10;
        }
        
        .main-content {
            flex: 1;
            padding: 30px;
        }
        
        .tree-container {
            padding: 10px 0;
        }
        
        .tree-node {
            position: relative;
        }
        
        .tree-item {
            display: flex;
            align-items: center;
            padding: 8px 0;
            cursor: pointer;
            transition: background 0.2s;
            border-radius: 4px;
            margin: 2px 0;
        }
        
        .tree-item:hover {
            background: #f8f9fa;
        }
        
        .tree-item.active {
            background: #e3f2fd;
            border-left: 4px solid #007bff;
        }
        
        .tree-expand {
            width: 20px;
            height: 20px;
            display: flex;
            align-items: center;
            justify-content: center;
            margin-right: 8px;
            cursor: pointer;
            border-radius: 3px;
            transition: background 0.2s;
        }
        
        .tree-expand:hover {
            background: #e9ecef;
        }
        
        .tree-expand-icon {
            width: 0;
            height: 0;
            border-left: 5px solid #6c757d;
            border-top: 4px solid transparent;
            border-bottom: 4px solid transparent;
            transition: transform 0.2s;
        }
        
        .tree-expand-icon.expanded {
            transform: rotate(90deg);
        }
        
        .tree-expand-icon.collapsed {
            transform: rotate(0deg);
        }
        
        .tree-content {
            flex: 1;
            display: flex;
            flex-direction: column;
        }
        
        .tree-children {
            margin-left: 28px;
            display: none;
        }
        
        .tree-children.expanded {
            display: block;
        }
        
        .tree-indent {
            width: 20px;
            flex-shrink: 0;
        }
        
        .command-name {
            font-weight: 500;
            color: #212529;
        }
        
        .command-description {
            font-size: 0.9em;
            color: #6c757d;
            margin-top: 4px;
        }
        
        .command-form {
            background: #f8f9fa;
            border-radius: 8px;
            padding: 25px;
            margin-bottom: 20px;
        }
        
        .form-group {
            margin-bottom: 20px;
        }
        
        .form-group label {
            display: block;
            margin-bottom: 8px;
            font-weight: 500;
            color: #495057;
        }
        
        .form-group input, .form-group select, .form-group textarea {
            width: 100%;
            padding: 12px;
            border: 1px solid #ced4da;
            border-radius: 6px;
            font-size: 14px;
            transition: border-color 0.2s;
        }
        
        .form-group input:focus, .form-group select:focus, .form-group textarea:focus {
            outline: none;
            border-color: #007bff;
            box-shadow: 0 0 0 3px rgba(0,123,255,0.1);
        }
        
        .required {
            color: #dc3545;
        }
        
        .btn {
            background: #007bff;
            color: white;
            border: none;
            padding: 12px 24px;
            border-radius: 6px;
            cursor: pointer;
            font-size: 14px;
            font-weight: 500;
            transition: background 0.2s;
        }
        
        .btn:hover {
            background: #0056b3;
        }
        
        .btn:disabled {
            background: #6c757d;
            cursor: not-allowed;
        }
        
        .response {
            background: #f8f9fa;
            border: 1px solid #e9ecef;
            border-radius: 6px;
            padding: 20px;
            margin-top: 20px;
            font-family: 'Monaco', 'Menlo', monospace;
            font-size: 13px;
            white-space: pre-wrap;
            max-height: 400px;
            overflow-y: auto;
            word-break: break-all;
            position: relative;
            z-index: 1;
        }
        
        .response.success {
            border-color: #28a745;
            background: #d4edda;
        }
        
        .response.error {
            border-color: #dc3545;
            background: #f8d7da;
        }
        
        .loading {
            display: none;
            text-align: center;
            padding: 20px;
            color: #6c757d;
        }
        
        .loading.active {
            display: block;
        }
        
        .spinner {
            border: 3px solid #f3f3f3;
            border-top: 3px solid #007bff;
            border-radius: 50%;
            width: 30px;
            height: 30px;
            animation: spin 1s linear infinite;
            margin: 0 auto 10px;
        }
        
        @keyframes spin {
            0% { transform: rotate(0deg); }
            100% { transform: rotate(360deg); }
        }
        
        .parameter-type {
            font-size: 0.8em;
            color: #6c757d;
            font-style: italic;
        }
        
        .subcommand {
            margin-left: 20px;
            border-left: 2px solid #e9ecef;
            padding-left: 10px;
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>Aerospike Cloud CLI</h1>
            <p>Interactive Web Interface</p>
        </div>
        
        <div class="content">
            <div class="sidebar">
                <div id="command-tree" class="tree-container"></div>
            </div>
            
            <div class="main-content">
                <div id="command-form" class="command-form" style="display: none;">
                    <h3 id="command-title"></h3>
                    <form id="api-form">
                        <div id="parameters"></div>
                        <button type="submit" class="btn">Execute Command</button>
                    </form>
                </div>
                
                <div id="loading" class="loading">
                    <div class="spinner"></div>
                    <div>Executing command...</div>
                </div>
                
                <div id="response" class="response" style="display: none;"></div>
            </div>
        </div>
    </div>

    <script>
        let commands = JSON.parse({{.Commands}});
        let currentCommand = null;
        
        // Initialize the UI
        function init() {
            renderCommandTree();
            setupEventListeners();
        }
        
        // Render command tree in sidebar
        function renderCommandTree() {
            const container = document.getElementById('command-tree');
            const tree = buildCommandTree();
            
            // Render the tree
            renderTreeNode(container, tree);
        }
        
        // Build hierarchical tree structure from commands
        function buildCommandTree() {
            const tree = {};
            
            commands.forEach(cmd => {
                const parts = cmd.path.split(' ');
                let current = tree;
                
                // Build nested structure
                for (let i = 0; i < parts.length; i++) {
                    const part = parts[i];
                    if (!current[part]) {
                        current[part] = {
                            name: part,
                            children: {},
                            command: null,
                            expanded: false
                        };
                    }
                    current = current[part].children;
                }
                
                // Set the command on the leaf node
                const leafParts = cmd.path.split(' ');
                let leaf = tree;
                for (let i = 0; i < leafParts.length; i++) {
                    if (i === leafParts.length - 1) {
                        leaf[leafParts[i]].command = cmd;
                    } else {
                        leaf = leaf[leafParts[i]].children;
                    }
                }
            });
            
            return tree;
        }
        
        // Render a tree node and its children
        function renderTreeNode(container, node, level = 0) {
            Object.values(node).forEach(item => {
                const nodeDiv = document.createElement('div');
                nodeDiv.className = 'tree-node';
                
                const itemDiv = document.createElement('div');
                itemDiv.className = 'tree-item';
                if (item.command) {
                    itemDiv.onclick = () => selectCommand(item.command);
                }
                
                // Add indentation
                const indent = document.createElement('div');
                indent.className = 'tree-indent';
                indent.style.width = (level * 20) + 'px';
                itemDiv.appendChild(indent);
                
                // Add expand/collapse button if has children
                if (Object.keys(item.children).length > 0) {
                    const expandDiv = document.createElement('div');
                    expandDiv.className = 'tree-expand';
                    expandDiv.onclick = (e) => {
                        e.stopPropagation();
                        toggleTreeNode(item.name);
                    };
                    
                    const expandIcon = document.createElement('div');
                    expandIcon.className = 'tree-expand-icon ' + (item.expanded ? 'expanded' : 'collapsed');
                    expandDiv.appendChild(expandIcon);
                    itemDiv.appendChild(expandDiv);
                } else {
                    const spacer = document.createElement('div');
                    spacer.className = 'tree-expand';
                    spacer.style.visibility = 'hidden';
                    itemDiv.appendChild(spacer);
                }
                
                // Add content
                const contentDiv = document.createElement('div');
                contentDiv.className = 'tree-content';
                
                const nameDiv = document.createElement('div');
                nameDiv.className = 'command-name';
                nameDiv.textContent = item.name;
                
                const descDiv = document.createElement('div');
                descDiv.className = 'command-description';
                descDiv.textContent = item.command ? item.command.description : '';
                
                contentDiv.appendChild(nameDiv);
                contentDiv.appendChild(descDiv);
                itemDiv.appendChild(contentDiv);
                
                nodeDiv.appendChild(itemDiv);
                
                // Add children container
                if (Object.keys(item.children).length > 0) {
                    const childrenDiv = document.createElement('div');
                    childrenDiv.className = 'tree-children';
                    if (item.expanded) {
                        childrenDiv.classList.add('expanded');
                    }
                    childrenDiv.id = 'children-' + item.name;
                    renderTreeNode(childrenDiv, item.children, level + 1);
                    nodeDiv.appendChild(childrenDiv);
                }
                
                container.appendChild(nodeDiv);
            });
        }
        
        // Toggle tree node expansion
        function toggleTreeNode(nodeName) {
            const children = document.getElementById('children-' + nodeName);
            const expandIcon = children.previousElementSibling.querySelector('.tree-expand-icon');
            
            if (children.classList.contains('expanded')) {
                children.classList.remove('expanded');
                expandIcon.classList.remove('expanded');
                expandIcon.classList.add('collapsed');
            } else {
                children.classList.add('expanded');
                expandIcon.classList.remove('collapsed');
                expandIcon.classList.add('expanded');
            }
        }
        
        // Select a command
        function selectCommand(cmd) {
            // Update active states
            document.querySelectorAll('.tree-item').forEach(item => {
                item.classList.remove('active');
            });
            event.target.classList.add('active');
            
            currentCommand = cmd;
            renderCommandForm(cmd);
            
            // Hide previous command output
            hideResponse();
            
            // Auto-scroll to the top of the page
            window.scrollTo({
                top: 0,
                behavior: 'smooth'
            });
        }
        
        // Render command form
        function renderCommandForm(cmd) {
            const form = document.getElementById('command-form');
            const title = document.getElementById('command-title');
            const params = document.getElementById('parameters');
            
            title.textContent = cmd.path;
            params.innerHTML = '';
            
            cmd.parameters.forEach(param => {
                const group = document.createElement('div');
                group.className = 'form-group';
                
                const label = document.createElement('label');
                label.textContent = param.long || param.short || param.name;
                if (param.required) {
                    label.innerHTML += ' <span class="required">*</span>';
                }
                
                let input;
                if (param.type === 'bool') {
                    input = document.createElement('select');
                    input.innerHTML = '<option value="false">False</option><option value="true">True</option>';
                } else if (param.type === '[]string') {
                    input = document.createElement('textarea');
                    input.placeholder = 'Enter values separated by commas';
                } else {
                    input = document.createElement('input');
                    input.type = 'text';
                }
                
                input.name = param.long || param.short || param.name;
                input.placeholder = param.description;
                if (param.default) {
                    input.value = param.default;
                }
                
                const typeInfo = document.createElement('div');
                typeInfo.className = 'parameter-type';
                typeInfo.textContent = param.type;
                
                group.appendChild(label);
                group.appendChild(input);
                group.appendChild(typeInfo);
                params.appendChild(group);
            });
            
            form.style.display = 'block';
        }
        
        // Setup event listeners
        function setupEventListeners() {
            document.getElementById('api-form').addEventListener('submit', handleSubmit);
        }
        
        // Handle form submission
        async function handleSubmit(e) {
            e.preventDefault();
            
            const form = e.target;
            const formData = new FormData(form);
            const params = {};
            
            for (let [key, value] of formData.entries()) {
                if (value) {
                    params[key] = value;
                }
            }
            
            showLoading(true);
            hideResponse();
            
            try {
                const response = await fetch('/api/execute', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json',
                    },
                    body: JSON.stringify({
                        command: currentCommand.path,
                        parameters: params
                    })
                });
                
                const result = await response.json();
                showResponse(result);
            } catch (error) {
                showResponse({
                    success: false,
                    error: error.message
                });
            } finally {
                showLoading(false);
            }
        }
        
        // Show/hide loading
        function showLoading(show) {
            const loading = document.getElementById('loading');
            if (show) {
                loading.classList.add('active');
            } else {
                loading.classList.remove('active');
            }
        }
        
        // Show response
        function showResponse(result) {
            const response = document.getElementById('response');
            response.style.display = 'block';
            response.className = 'response ' + (result.success ? 'success' : 'error');
            response.textContent = result.success ? 
                JSON.stringify(result.data, null, 2) : 
                result.error;
            
            // Ensure sidebar remains visible
            const sidebar = document.querySelector('.sidebar');
            if (sidebar) {
                sidebar.style.display = 'block';
                sidebar.style.position = 'relative';
                sidebar.style.zIndex = '10';
            }
        }
        
        // Hide response
        function hideResponse() {
            const response = document.getElementById('response');
            response.style.display = 'none';
        }
        
        // Initialize when page loads
        document.addEventListener('DOMContentLoaded', init);
    </script>
</body>
</html>
`

// Execute the webui command
func (c *WebUIServer) Execute(args []string) error {
	// Get client for API calls
	client, err := NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}
	c.Client = client

	// Start web server
	addr := fmt.Sprintf("%s:%d", c.Host, c.Port)
	fmt.Printf("Starting Aerospike Cloud CLI Web UI on http://%s\n", addr)
	fmt.Println("Press Ctrl+C to stop the server")

	// Setup routes
	http.HandleFunc("/", c.handleIndex)
	http.HandleFunc("/api/execute", c.handleExecute)

	// Start server
	return http.ListenAndServe(addr, nil)
}

// handleIndex serves the main web UI page
func (c *WebUIServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	// Build command structure using reflection
	commands := c.buildCommandStructure()

	// Create template data
	data := struct {
		Commands string
	}{
		Commands: c.commandsToJSON(commands),
	}

	// Parse and execute template
	tmpl, err := template.New("webui").Parse(webUITemplate)
	if err != nil {
		http.Error(w, "Template error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	if err := tmpl.Execute(w, data); err != nil {
		http.Error(w, "Template execution error: "+err.Error(), http.StatusInternalServerError)
		return
	}
}

// handleExecute handles API execution requests
func (c *WebUIServer) handleExecute(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var request struct {
		Command    string                 `json:"command"`
		Parameters map[string]interface{} `json:"parameters"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Create a client for API calls
	client, err := NewClient()
	if err != nil {
		response := APIResponse{
			Success: false,
			Error:   fmt.Sprintf("Failed to create API client: %v", err),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
		return
	}

	// Execute the command based on the command path
	var result interface{}
	var success bool
	var errorMsg string

	switch request.Command {
	case "cloud-provider list":
		var cloudProviders interface{}
		err = client.Get("/cloud-providers", &cloudProviders)
		result = cloudProviders
		success = err == nil
		if err != nil {
			errorMsg = err.Error()
		}
	case "cloud-provider get-specs":
		cloudProvider, _ := request.Parameters["cloud-provider"].(string)
		instanceType, _ := request.Parameters["instance-type"].(string)
		var specs interface{}
		err = client.Get(fmt.Sprintf("/cloud-providers/%s/instance-types/%s", cloudProvider, instanceType), &specs)
		result = specs
		success = err == nil
		if err != nil {
			errorMsg = err.Error()
		}
	case "organization get":
		var org interface{}
		err = client.Get("/organization", &org)
		result = org
		success = err == nil
		if err != nil {
			errorMsg = err.Error()
		}
	case "api-keys list":
		var apiKeys interface{}
		err = client.Get("/api-keys", &apiKeys)
		result = apiKeys
		success = err == nil
		if err != nil {
			errorMsg = err.Error()
		}
	case "api-keys create":
		name, _ := request.Parameters["name"].(string)
		body := map[string]string{"name": name}
		var apiKey interface{}
		err = client.Post("/api-keys", body, &apiKey)
		result = apiKey
		success = err == nil
		if err != nil {
			errorMsg = err.Error()
		}
	case "api-keys delete":
		clientID, _ := request.Parameters["client-id"].(string)
		err = client.Delete(fmt.Sprintf("/api-keys/%s", clientID))
		success = err == nil
		if err != nil {
			errorMsg = err.Error()
		} else {
			result = map[string]string{"message": "API key deleted successfully"}
		}
	case "secrets list":
		var secrets interface{}
		err = client.Get("/secrets", &secrets)
		result = secrets
		success = err == nil
		if err != nil {
			errorMsg = err.Error()
		}
	case "secrets create":
		name, _ := request.Parameters["name"].(string)
		description, _ := request.Parameters["description"].(string)
		value, _ := request.Parameters["value"].(string)
		body := map[string]string{
			"name":        name,
			"description": description,
			"value":       value,
		}
		var secret interface{}
		err = client.Post("/secrets", body, &secret)
		result = secret
		success = err == nil
		if err != nil {
			errorMsg = err.Error()
		}
	case "secrets delete":
		secretID, _ := request.Parameters["secret-id"].(string)
		err = client.Delete(fmt.Sprintf("/secrets/%s", secretID))
		success = err == nil
		if err != nil {
			errorMsg = err.Error()
		} else {
			result = map[string]string{"message": "Secret deleted successfully"}
		}
	case "databases list":
		var databases interface{}
		path := "/databases"
		if statusNe, ok := request.Parameters["status-ne"].(string); ok && statusNe != "" {
			path += "?status-ne=" + statusNe
		}
		err = client.Get(path, &databases)
		result = databases
		success = err == nil
		if err != nil {
			errorMsg = err.Error()
		}
	case "databases create":
		// This is a complex command - for now, return a message about required parameters
		result = map[string]interface{}{
			"message":    "Database creation requires complex parameters. Please use the CLI directly for now.",
			"command":    request.Command,
			"parameters": request.Parameters,
		}
		success = true
	case "databases get":
		dbID, _ := request.Parameters["database-id"].(string)
		var database interface{}
		err = client.Get(fmt.Sprintf("/databases/%s", dbID), &database)
		result = database
		success = err == nil
		if err != nil {
			errorMsg = err.Error()
		}
	case "databases delete":
		dbID, _ := request.Parameters["database-id"].(string)
		err = client.Delete(fmt.Sprintf("/databases/%s", dbID))
		success = err == nil
		if err != nil {
			errorMsg = err.Error()
		} else {
			result = map[string]string{"message": "Database deleted successfully"}
		}
	case "databases metrics":
		dbID, _ := request.Parameters["database-id"].(string)
		var metrics interface{}
		err = client.Get(fmt.Sprintf("/databases/%s/metrics", dbID), &metrics)
		result = metrics
		success = err == nil
		if err != nil {
			errorMsg = err.Error()
		}
	case "databases vpc-peering list":
		dbID, _ := request.Parameters["database-id"].(string)
		var vpcPeerings interface{}
		err = client.Get(fmt.Sprintf("/databases/%s/vpc-peerings", dbID), &vpcPeerings)
		result = vpcPeerings
		success = err == nil
		if err != nil {
			errorMsg = err.Error()
		}
	case "databases vpc-peering create":
		dbID, _ := request.Parameters["database-id"].(string)
		vpcID, _ := request.Parameters["vpc-id"].(string)
		cidrBlock, _ := request.Parameters["cidr-block"].(string)
		accountID, _ := request.Parameters["account-id"].(string)
		region, _ := request.Parameters["region"].(string)
		isSecureConnection, _ := request.Parameters["is-secure-connection"].(string)

		body := map[string]interface{}{
			"vpcId":              vpcID,
			"cidrBlock":          cidrBlock,
			"accountId":          accountID,
			"region":             region,
			"isSecureConnection": isSecureConnection == "true",
		}
		var vpcPeering interface{}
		err = client.Post(fmt.Sprintf("/databases/%s/vpc-peerings", dbID), body, &vpcPeering)
		result = vpcPeering
		success = err == nil
		if err != nil {
			errorMsg = err.Error()
		}
	case "databases vpc-peering delete":
		dbID, _ := request.Parameters["database-id"].(string)
		vpcID, _ := request.Parameters["vpc-id"].(string)
		err = client.Delete(fmt.Sprintf("/databases/%s/vpc-peerings/%s", dbID, vpcID))
		success = err == nil
		if err != nil {
			errorMsg = err.Error()
		} else {
			result = map[string]string{"message": "VPC peering deleted successfully"}
		}
	case "databases credentials list":
		dbID, _ := request.Parameters["database-id"].(string)
		var credentials interface{}
		err = client.Get(fmt.Sprintf("/databases/%s/credentials", dbID), &credentials)
		result = credentials
		success = err == nil
		if err != nil {
			errorMsg = err.Error()
		}
	case "databases credentials create":
		dbID, _ := request.Parameters["database-id"].(string)
		username, _ := request.Parameters["username"].(string)
		password, _ := request.Parameters["password"].(string)
		privileges, _ := request.Parameters["privileges"].(string)

		body := map[string]string{
			"username":   username,
			"password":   password,
			"privileges": privileges,
		}
		var credential interface{}
		err = client.Post(fmt.Sprintf("/databases/%s/credentials", dbID), body, &credential)
		result = credential
		success = err == nil
		if err != nil {
			errorMsg = err.Error()
		}
	case "databases credentials delete":
		dbID, _ := request.Parameters["database-id"].(string)
		username, _ := request.Parameters["username"].(string)
		err = client.Delete(fmt.Sprintf("/databases/%s/credentials/%s", dbID, username))
		success = err == nil
		if err != nil {
			errorMsg = err.Error()
		} else {
			result = map[string]string{"message": "Database credentials deleted successfully"}
		}
	case "topologies list":
		var topologies interface{}
		err = client.Get("/topologies", &topologies)
		result = topologies
		success = err == nil
		if err != nil {
			errorMsg = err.Error()
		}
	case "topologies create":
		topologyID, _ := request.Parameters["topology-id"].(string)
		body := map[string]string{"topology-id": topologyID}
		var topology interface{}
		err = client.Post("/topologies", body, &topology)
		result = topology
		success = err == nil
		if err != nil {
			errorMsg = err.Error()
		}
	case "topologies get":
		topologyID, _ := request.Parameters["topology-id"].(string)
		var topology interface{}
		err = client.Get(fmt.Sprintf("/topologies/%s", topologyID), &topology)
		result = topology
		success = err == nil
		if err != nil {
			errorMsg = err.Error()
		}
	case "topologies delete":
		topologyID, _ := request.Parameters["topology-id"].(string)
		err = client.Delete(fmt.Sprintf("/topologies/%s", topologyID))
		success = err == nil
		if err != nil {
			errorMsg = err.Error()
		} else {
			result = map[string]string{"message": "Topology deleted successfully"}
		}
	default:
		success = false
		errorMsg = fmt.Sprintf("Command not implemented: %s", request.Command)
	}

	response := APIResponse{
		Success: success,
		Data:    result,
		Error:   errorMsg,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// buildCommandStructure uses reflection to build the command structure
func (c *WebUIServer) buildCommandStructure() []CommandInfo {
	var commands []CommandInfo

	// Get the main Options struct
	opts := &Options{}
	optsValue := reflect.ValueOf(opts).Elem()
	optsType := optsValue.Type()

	// Iterate through the fields of Options
	for i := 0; i < optsType.NumField(); i++ {
		field := optsType.Field(i)
		fieldValue := optsValue.Field(i)

		// Skip non-command fields
		if field.Tag.Get("command") == "" {
			continue
		}

		// Skip hidden commands
		if field.Tag.Get("hidden") == "true" {
			continue
		}

		commandName := field.Tag.Get("command")
		description := field.Tag.Get("description")

		// Process subcommands if this is a command group
		if fieldValue.Kind() == reflect.Struct {
			subCommands := c.processCommandStruct(fieldValue, commandName)
			commands = append(commands, subCommands...)
		} else {
			// This is a simple command without subcommands
			cmdInfo := CommandInfo{
				Name:        commandName,
				Description: description,
				Path:        commandName,
				Parameters:  []ParameterInfo{},
				SubCommands: []CommandInfo{},
			}
			commands = append(commands, cmdInfo)
		}
	}

	return commands
}

// processCommandStruct processes a command struct and its subcommands
func (c *WebUIServer) processCommandStruct(structValue reflect.Value, basePath string) []CommandInfo {
	var commands []CommandInfo
	structType := structValue.Type()

	// Iterate through fields of the struct
	for i := 0; i < structType.NumField(); i++ {
		field := structType.Field(i)
		fieldValue := structValue.Field(i)

		// Check if this is a command field
		commandTag := field.Tag.Get("command")
		if commandTag != "" {
			// Skip hidden commands
			if field.Tag.Get("hidden") == "true" {
				continue
			}

			description := field.Tag.Get("description")
			commandPath := basePath + " " + commandTag

			cmdInfo := CommandInfo{
				Name:        commandTag,
				Description: description,
				Path:        commandPath,
				Parameters:  []ParameterInfo{},
				SubCommands: []CommandInfo{},
			}

			// If this is a struct, process its parameters and subcommands
			if fieldValue.Kind() == reflect.Struct {
				// Extract parameters from this command struct
				parameters := c.extractParameters(fieldValue)
				cmdInfo.Parameters = parameters

				// Check for subcommands within this struct
				subCommands := c.processCommandStruct(fieldValue, commandPath)
				cmdInfo.SubCommands = subCommands

				// Add subcommands to the main list
				commands = append(commands, subCommands...)
			}

			// Add this command (only if it has parameters or is a leaf command)
			if len(cmdInfo.Parameters) > 0 || len(cmdInfo.SubCommands) == 0 {
				commands = append(commands, cmdInfo)
			}
		}
	}

	return commands
}

// extractParameters extracts parameter information from a struct
func (c *WebUIServer) extractParameters(structValue reflect.Value) []ParameterInfo {
	var parameters []ParameterInfo
	structType := structValue.Type()

	for i := 0; i < structType.NumField(); i++ {
		field := structType.Field(i)
		fieldValue := structValue.Field(i)

		// Skip command fields
		if field.Tag.Get("command") != "" {
			continue
		}

		// Extract parameter info
		short := field.Tag.Get("short")
		long := field.Tag.Get("long")
		description := field.Tag.Get("description")
		required := field.Tag.Get("required") == "true"
		defaultValue := field.Tag.Get("default")

		// Determine parameter name
		name := long
		if name == "" {
			name = short
		}
		if name == "" {
			name = field.Name
		}

		// Determine type
		var paramType string
		switch fieldValue.Kind() {
		case reflect.Bool:
			paramType = "bool"
		case reflect.String:
			paramType = "string"
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			paramType = "int"
		case reflect.Slice:
			if fieldValue.Type().Elem().Kind() == reflect.String {
				paramType = "[]string"
			} else {
				paramType = "slice"
			}
		default:
			paramType = "unknown"
		}

		param := ParameterInfo{
			Name:        name,
			Short:       short,
			Long:        long,
			Description: description,
			Required:    required,
			Default:     defaultValue,
			Type:        paramType,
		}

		parameters = append(parameters, param)
	}

	return parameters
}

// commandsToJSON converts commands to JSON string for the template
func (c *WebUIServer) commandsToJSON(commands []CommandInfo) string {
	jsonData, err := json.Marshal(commands)
	if err != nil {
		return "[]"
	}
	return string(jsonData)
}
