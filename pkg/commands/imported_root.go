package commands

import (
	"context"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"

	criconstants "github.com/containerd/containerd/pkg/cri/constants"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"

	"github.com/siderolabs/gen/maps"
	_ "github.com/siderolabs/talos/pkg/grpc/codec" // register codec
	"github.com/siderolabs/talos/pkg/machinery/api/common"
	machineapi "github.com/siderolabs/talos/pkg/machinery/api/machine"
	"github.com/siderolabs/talos/pkg/machinery/client"
	"github.com/siderolabs/talos/pkg/machinery/constants"
	"github.com/siderolabs/talos/pkg/machinery/formatters"
)

// completeResource represents tab complete options for `ls` and `ls *` commands.
func completePathFromNode(inputPath string) []string {
	pathToSearch := inputPath

	// If the pathToSearch is empty, use root '/'
	if pathToSearch == "" {
		pathToSearch = "/"
	}

	var paths map[string]struct{}

	// search up one level to find possible completions
	if pathToSearch != "/" && !strings.HasSuffix(pathToSearch, "/") {
		index := strings.LastIndex(pathToSearch, "/")
		// we need a trailing slash to search for items in a directory
		pathToSearch = pathToSearch[:index] + "/"
	}

	paths = getPathFromNode(pathToSearch, inputPath)

	return maps.Keys(paths)
}

//nolint:gocyclo
func getPathFromNode(path, filter string) map[string]struct{} {
	paths := make(map[string]struct{})

	//nolint:errcheck
	GlobalArgs.WithClient(
		func(ctx context.Context, c *client.Client) error {
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()

			stream, err := c.LS(
				ctx, &machineapi.ListRequest{
					Root: path,
				},
			)
			if err != nil {
				return err
			}

			for {
				resp, err := stream.Recv()
				if err != nil {
					if err == io.EOF || client.StatusCode(err) == codes.Canceled {
						return nil
					}

					return fmt.Errorf("error streaming results: %s", err)
				}

				if resp.Metadata != nil && resp.Metadata.Error != "" {
					continue
				}

				if resp.Error != "" {
					continue
				}

				// skip reference to the same directory
				if resp.RelativeName == "." {
					continue
				}

				// limit the results to a reasonable amount
				if len(paths) > pathAutoCompleteLimit {
					return nil
				}

				// directories have a trailing slash
				if resp.IsDir {
					fullPath := path + resp.RelativeName + "/"

					if relativeTo(fullPath, filter) {
						paths[fullPath] = struct{}{}
					}
				} else {
					fullPath := path + resp.RelativeName

					if relativeTo(fullPath, filter) {
						paths[fullPath] = struct{}{}
					}
				}
			}
		},
	)

	return paths
}

func getServiceFromNode() []string {
	var svcIds []string

	//nolint:errcheck
	GlobalArgs.WithClient(
		func(ctx context.Context, c *client.Client) error {
			var remotePeer peer.Peer

			resp, err := c.ServiceList(ctx, grpc.Peer(&remotePeer))
			if err != nil {
				return err
			}

			for _, msg := range resp.Messages {
				for _, s := range msg.Services {
					svc := formatters.ServiceInfoWrapper{ServiceInfo: s}
					svcIds = append(svcIds, svc.Id)
				}
			}

			return nil
		},
	)

	return svcIds
}

func getContainersFromNode(kubernetes bool) []string {
	var containerIds []string

	//nolint:errcheck
	GlobalArgs.WithClient(
		func(ctx context.Context, c *client.Client) error {
			var (
				namespace string
				driver    common.ContainerDriver
			)

			if kubernetes {
				namespace = criconstants.K8sContainerdNamespace
				driver = common.ContainerDriver_CRI
			} else {
				namespace = constants.SystemContainerdNamespace
				driver = common.ContainerDriver_CONTAINERD
			}

			resp, err := c.Containers(ctx, namespace, driver)
			if err != nil {
				return err
			}

			for _, msg := range resp.Messages {
				for _, p := range msg.Containers {
					if p.Pid == 0 {
						continue
					}

					if kubernetes && p.Id == p.PodId {
						continue
					}

					containerIds = append(containerIds, p.Id)
				}
			}

			return nil
		},
	)

	return containerIds
}

func mergeSuggestions(a, b, c []string) []string {
	merged := append(slices.Clone(a), b...)

	sort.Strings(merged)

	n := 1

	for i := 1; i < len(merged); i++ {
		if merged[i] != merged[i-1] {
			merged[n] = merged[i]
			n++
		}
	}

	merged = merged[:n]

	return merged
}

func relativeTo(fullPath string, filter string) bool {
	return strings.HasPrefix(fullPath, filter)
}
