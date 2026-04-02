package bdocker

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/lithammer/shortuuid"
)

func (s *b) DockerCreateNetwork(region string, name string, driver string, subnet string, mtu string) error {
	log := s.log.WithPrefix("DockerCreateNetwork: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	defer s.invalidateCacheFunc(backends.CacheInvalidateNetwork) //nolint:errcheck
	cli, err := s.getDockerClient(region)
	if err != nil {
		return err
	}
	var options map[string]string
	if mtu != "" {
		options = map[string]string{
			"com.docker.network.driver.mtu": mtu,
		}
	}
	var ipam *network.IPAM
	if subnet != "" {
		ipam = &network.IPAM{
			Config: []network.IPAMConfig{
				{Subnet: subnet},
			},
		}
	}
	_, err = cli.NetworkCreate(context.Background(), name, network.CreateOptions{
		Driver:     driver,
		IPAM:       ipam,
		Options:    options,
		Attachable: true,
	})
	if err != nil {
		return err
	}
	return nil
}

func (s *b) DockerDeleteNetwork(region string, name string) error {
	log := s.log.WithPrefix("DockerDeleteNetwork: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	defer s.invalidateCacheFunc(backends.CacheInvalidateNetwork) //nolint:errcheck
	cli, err := s.getDockerClient(region)
	if err != nil {
		return err
	}
	err = cli.NetworkRemove(context.Background(), name)
	if err != nil {
		return err
	}
	return nil
}

func (s *b) DockerPruneNetworks(region string) error {
	log := s.log.WithPrefix("DockerPruneNetworks: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	defer s.invalidateCacheFunc(backends.CacheInvalidateNetwork) //nolint:errcheck
	cli, err := s.getDockerClient(region)
	if err != nil {
		return err
	}
	_, err = cli.NetworksPrune(context.Background(), filters.NewArgs())
	if err != nil {
		return err
	}
	return nil
}

func (s *b) DockerLoadImage(region string, reader io.Reader, projectLabels map[string]string) error {
	log := s.log.WithPrefix("DockerLoadImage: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	defer s.invalidateCacheFunc(backends.CacheInvalidateImage) //nolint:errcheck
	cli, err := s.getDockerClient(region)
	if err != nil {
		return err
	}

	log.Detail("Loading image from reader")
	resp, err := cli.ImageLoad(context.Background(), reader)
	if err != nil {
		return fmt.Errorf("docker image load: %w", err)
	}
	defer resp.Body.Close()

	// Parse load response to find the loaded image reference.
	// Docker returns JSON objects like {"stream":"Loaded image: name:tag\n"}
	// Podman may return {"stream":"Loaded image(s): sha256:...\n"}
	var loadedRef string
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		var msg struct {
			Stream string `json:"stream"`
		}
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}
		msg.Stream = strings.TrimSpace(msg.Stream)
		if strings.HasPrefix(msg.Stream, "Loaded image") {
			// "Loaded image: name:tag" or "Loaded image(s): sha256:abc"
			parts := strings.SplitN(msg.Stream, ": ", 2)
			if len(parts) == 2 {
				loadedRef = strings.TrimSpace(parts[1])
			}
		}
	}
	if loadedRef == "" {
		return fmt.Errorf("could not determine loaded image reference from docker load response")
	}
	log.Detail("Loaded image: %s", loadedRef)

	// Handle Podman localhost/ prefix
	inspectRef := loadedRef
	if s.isPodman[region] && !strings.HasPrefix(inspectRef, "localhost/") && !strings.HasPrefix(inspectRef, "sha256:") {
		inspectRef = "localhost/" + inspectRef
	}

	// Inspect the loaded image to get its original labels
	inspectData, _, err := cli.ImageInspectWithRaw(context.Background(), inspectRef)
	if err != nil {
		return fmt.Errorf("inspect loaded image %s: %w", inspectRef, err)
	}

	// Merge original labels with project label overrides
	mergedLabels := make(map[string]string)
	for k, v := range inspectData.Config.Labels {
		mergedLabels[k] = v
	}
	for k, v := range projectLabels {
		mergedLabels[k] = v
	}

	// Derive the final image name from labels or the loaded ref
	finalName := mergedLabels[TAG_NAME]
	if finalName == "" {
		finalName = loadedRef
	}

	// Create a temporary container from the loaded image (no start needed)
	log.Detail("Creating temp container for re-label")
	tempContainer, err := cli.ContainerCreate(context.Background(), &container.Config{
		Image: inspectRef,
	}, nil, nil, nil, "")
	if err != nil {
		return fmt.Errorf("create temp container: %w", err)
	}
	defer func() {
		cli.ContainerRemove(context.Background(), tempContainer.ID, container.RemoveOptions{Force: true}) //nolint:errcheck
	}()

	// Commit the container with corrected labels
	commitRef := finalName
	if s.isPodman[region] {
		commitRef = "localhost/" + finalName
	}
	log.Detail("Committing with corrected labels as %s", commitRef)
	_, err = cli.ContainerCommit(context.Background(), tempContainer.ID, container.CommitOptions{
		Reference: commitRef,
		Comment:   "Template loaded from registry",
		Author:    "aerolab",
		Pause:     false,
		Config: &container.Config{
			Labels:     mergedLabels,
			Entrypoint: inspectData.Config.Entrypoint,
			Cmd:        inspectData.Config.Cmd,
			Env:        inspectData.Config.Env,
			WorkingDir: inspectData.Config.WorkingDir,
			User:       inspectData.Config.User,
		},
	})
	if err != nil {
		return fmt.Errorf("commit re-labeled image: %w", err)
	}

	// Remove the original loaded image (the committed one replaces it)
	if inspectRef != commitRef {
		log.Detail("Removing original loaded image %s", inspectRef)
		cli.ImageRemove(context.Background(), inspectData.ID, image.RemoveOptions{Force: true, PruneChildren: true}) //nolint:errcheck
	}

	return nil
}
