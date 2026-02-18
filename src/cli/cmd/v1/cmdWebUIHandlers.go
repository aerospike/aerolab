package cmd

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"path"
	"strings"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/sshexec"
)

// handleFileDownload handles streaming file downloads via tar.gz
// This streams files directly from SFTP to the HTTP response without temporary storage
func (c *WebUICmd) handleFileDownload(w http.ResponseWriter, r *http.Request, cmdPath string, cmdInfo *CommandInfo) {
	// Parse query parameters for file download
	clusterName := r.URL.Query().Get("cluster")
	if clusterName == "" {
		clusterName = r.URL.Query().Get("name")
	}
	if clusterName == "" {
		http.Error(w, "Missing required parameter: cluster (or name)", http.StatusBadRequest)
		return
	}

	nodes := r.URL.Query().Get("nodes")
	sourcePath := r.URL.Query().Get("source")
	if sourcePath == "" {
		sourcePath = r.URL.Query().Get("path")
	}
	if sourcePath == "" {
		http.Error(w, "Missing required parameter: source (or path)", http.StatusBadRequest)
		return
	}

	// Get inventory
	inventory := c.getInventory()
	if inventory == nil {
		http.Error(w, "Backend not initialized", http.StatusInternalServerError)
		return
	}

	// Get cluster instances
	instances := inventory.Instances.WithClusterName(clusterName)
	if instances.Count() == 0 {
		http.Error(w, fmt.Sprintf("Cluster not found: %s", clusterName), http.StatusNotFound)
		return
	}

	// Filter to running instances
	instances = instances.WithState(backends.LifeCycleStateRunning)
	if instances.Count() == 0 {
		http.Error(w, fmt.Sprintf("No running instances in cluster: %s", clusterName), http.StatusNotFound)
		return
	}

	// Filter by node numbers if specified
	if nodes != "" {
		nodeNos, err := expandNodeNumbers(nodes)
		if err != nil {
			http.Error(w, fmt.Sprintf("Invalid nodes parameter: %s", err), http.StatusBadRequest)
			return
		}
		instances = instances.WithNodeNo(nodeNos...)
		if instances.Count() == 0 {
			http.Error(w, fmt.Sprintf("No matching nodes found for: %s", nodes), http.StatusNotFound)
			return
		}
	}

	instanceList := instances.Describe()

	// Set response headers for tar.gz download
	filename := fmt.Sprintf("%s-%s.tar.gz", clusterName, path.Base(sourcePath))
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))

	// Create streaming pipeline: SFTP -> tar -> gzip -> HTTP response
	gzWriter := gzip.NewWriter(w)
	defer gzWriter.Close()
	tarWriter := tar.NewWriter(gzWriter)
	defer tarWriter.Close()

	// Download from each instance
	for _, instance := range instanceList {
		conf, err := instance.GetSftpConfig("root")
		if err != nil {
			c.logError("Failed to get SFTP config for node %d: %s", instance.NodeNo, err)
			continue
		}

		sftp, err := sshexec.NewSftp(conf)
		if err != nil {
			c.logError("Failed to create SFTP client for node %d: %s", instance.NodeNo, err)
			continue
		}

		// Walk the remote path and stream files to tar
		prefix := fmt.Sprintf("node-%d", instance.NodeNo)
		err = c.streamRemoteToTar(sftp, sourcePath, prefix, tarWriter)
		sftp.Close()

		if err != nil {
			c.logError("Failed to stream files from node %d: %s", instance.NodeNo, err)
			continue
		}
	}
}

// logError logs an error message safely, handling nil system/logger
func (c *WebUICmd) logError(format string, args ...any) {
	if c.system != nil && c.system.Logger != nil {
		c.system.Logger.Error(format, args...)
	} else {
		log.Printf("[ERROR] "+format, args...)
	}
}

// logDebug logs a debug message safely, handling nil system/logger
func (c *WebUICmd) logDebug(format string, args ...any) {
	if c.system != nil && c.system.Logger != nil {
		c.system.Logger.Debug(format, args...)
	} else {
		log.Printf("[DEBUG] "+format, args...)
	}
}

// streamRemoteToTar walks a remote path and streams all files to the tar writer
func (c *WebUICmd) streamRemoteToTar(sftp *sshexec.Sftp, remotePath string, prefix string, tarWriter *tar.Writer) error {
	client := sftp.RawClient()

	// Check if path exists and get info
	info, err := client.Stat(remotePath)
	if err != nil {
		return fmt.Errorf("failed to stat remote path: %w", err)
	}

	if info.IsDir() {
		// Walk directory recursively
		return c.walkRemoteDir(sftp, remotePath, prefix, tarWriter)
	}

	// Single file - write it to tar
	return c.writeFileToTar(sftp, remotePath, path.Join(prefix, path.Base(remotePath)), tarWriter)
}

// walkRemoteDir recursively walks a remote directory and writes files to tar
func (c *WebUICmd) walkRemoteDir(sftp *sshexec.Sftp, remotePath string, prefix string, tarWriter *tar.Writer) error {
	client := sftp.RawClient()

	entries, err := client.ReadDir(remotePath)
	if err != nil {
		return fmt.Errorf("failed to read directory: %w", err)
	}

	for _, entry := range entries {
		entryPath := path.Join(remotePath, entry.Name())
		tarPath := path.Join(prefix, entry.Name())

		if entry.IsDir() {
			// Write directory entry to tar
			header := &tar.Header{
				Name:     tarPath + "/",
				Mode:     int64(entry.Mode()),
				ModTime:  entry.ModTime(),
				Typeflag: tar.TypeDir,
			}
			if err := tarWriter.WriteHeader(header); err != nil {
				return err
			}

			// Recurse into directory
			if err := c.walkRemoteDir(sftp, entryPath, tarPath, tarWriter); err != nil {
				return err
			}
		} else if entry.Mode().IsRegular() {
			// Write file
			if err := c.writeFileToTar(sftp, entryPath, tarPath, tarWriter); err != nil {
				return err
			}
		}
		// Skip symlinks and other special files for now
	}

	return nil
}

// writeFileToTar writes a single file to the tar archive
func (c *WebUICmd) writeFileToTar(sftp *sshexec.Sftp, remotePath string, tarPath string, tarWriter *tar.Writer) error {
	client := sftp.RawClient()

	// Get file info
	fileInfo, err := client.Stat(remotePath)
	if err != nil {
		return err
	}

	// Write tar header
	header := &tar.Header{
		Name:    tarPath,
		Size:    fileInfo.Size(),
		Mode:    int64(fileInfo.Mode()),
		ModTime: fileInfo.ModTime(),
	}
	if err := tarWriter.WriteHeader(header); err != nil {
		return err
	}

	// Stream file content directly to tar
	err = sftp.ReadFile(&sshexec.FileReader{
		SourcePath:  remotePath,
		Destination: tarWriter,
	})
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	return nil
}

// handleFileUpload handles streaming file uploads from multipart form to SFTP
// This streams files directly from the HTTP request to SFTP without temporary storage
func (c *WebUICmd) handleFileUpload(w http.ResponseWriter, r *http.Request, cmdPath string, cmdInfo *CommandInfo) {
	// Parse multipart form (32MB max memory, rest goes to disk)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, fmt.Sprintf("Failed to parse multipart form: %s", err), http.StatusBadRequest)
		return
	}

	// Get cluster name
	clusterName := r.FormValue("cluster")
	if clusterName == "" {
		clusterName = r.FormValue("name")
	}
	if clusterName == "" {
		http.Error(w, "Missing required parameter: cluster (or name)", http.StatusBadRequest)
		return
	}

	nodes := r.FormValue("nodes")
	destPath := r.FormValue("destination")
	if destPath == "" {
		destPath = r.FormValue("path")
	}
	if destPath == "" {
		http.Error(w, "Missing required parameter: destination (or path)", http.StatusBadRequest)
		return
	}

	// Get the uploaded file
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get uploaded file: %s", err), http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Get inventory
	inventory := c.getInventory()
	if inventory == nil {
		http.Error(w, "Backend not initialized", http.StatusInternalServerError)
		return
	}

	// Get cluster instances
	instances := inventory.Instances.WithClusterName(clusterName)
	if instances.Count() == 0 {
		http.Error(w, fmt.Sprintf("Cluster not found: %s", clusterName), http.StatusNotFound)
		return
	}

	// Filter to running instances
	instances = instances.WithState(backends.LifeCycleStateRunning)
	if instances.Count() == 0 {
		http.Error(w, fmt.Sprintf("No running instances in cluster: %s", clusterName), http.StatusNotFound)
		return
	}

	// Filter by node numbers if specified
	if nodes != "" {
		nodeNos, err := expandNodeNumbers(nodes)
		if err != nil {
			http.Error(w, fmt.Sprintf("Invalid nodes parameter: %s", err), http.StatusBadRequest)
			return
		}
		instances = instances.WithNodeNo(nodeNos...)
		if instances.Count() == 0 {
			http.Error(w, fmt.Sprintf("No matching nodes found for: %s", nodes), http.StatusNotFound)
			return
		}
	}

	instanceList := instances.Describe()

	// Check if it's a tar.gz upload
	isTarGz := strings.HasSuffix(header.Filename, ".tar.gz") || strings.HasSuffix(header.Filename, ".tgz")

	results := make(map[int]string)
	var errors []string

	for _, instance := range instanceList {
		conf, err := instance.GetSftpConfig("root")
		if err != nil {
			errors = append(errors, fmt.Sprintf("node %d: failed to get SFTP config: %s", instance.NodeNo, err))
			continue
		}

		sftp, err := sshexec.NewSftp(conf)
		if err != nil {
			errors = append(errors, fmt.Sprintf("node %d: failed to create SFTP client: %s", instance.NodeNo, err))
			continue
		}

		if isTarGz {
			// Extract tar.gz directly to SFTP
			err = c.streamTarToRemote(sftp, file, destPath)
		} else {
			// Single file upload
			err = sftp.WriteFile(true, &sshexec.FileWriter{
				DestPath:    destPath,
				Source:      file,
				Permissions: 0644,
			})
		}
		sftp.Close()

		if err != nil {
			errors = append(errors, fmt.Sprintf("node %d: %s", instance.NodeNo, err))
		} else {
			results[instance.NodeNo] = "success"
		}

		// Seek back to start for next node
		if _, err := file.Seek(0, 0); err != nil {
			errors = append(errors, fmt.Sprintf("failed to seek file for next node: %s", err))
			break
		}
	}

	// Build response
	response := map[string]any{
		"success": len(errors) == 0,
		"results": results,
	}
	if len(errors) > 0 {
		response["errors"] = errors
	}

	w.Header().Set("Content-Type", "application/json")
	if len(errors) > 0 && len(results) == 0 {
		w.WriteHeader(http.StatusInternalServerError)
	}
	json.NewEncoder(w).Encode(response)
}

// streamTarToRemote extracts a tar.gz stream directly to remote SFTP
func (c *WebUICmd) streamTarToRemote(sftp *sshexec.Sftp, reader io.Reader, destPath string) error {
	gzReader, err := gzip.NewReader(reader)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar entry: %w", err)
		}

		targetPath := path.Join(destPath, header.Name)

		switch header.Typeflag {
		case tar.TypeDir:
			// Create directory
			if err := sftp.Mkdir(targetPath, 0755); err != nil {
				// Ignore errors for existing directories
				c.logDebug("mkdir %s: %s", targetPath, err)
			}
		case tar.TypeReg:
			// Stream file content directly to SFTP
			err = sftp.WriteFile(true, &sshexec.FileWriter{
				DestPath:    targetPath,
				Source:      tarReader,
				Permissions: header.FileInfo().Mode(),
			})
			if err != nil {
				return fmt.Errorf("failed to write file %s: %w", targetPath, err)
			}
		}
	}

	return nil
}
