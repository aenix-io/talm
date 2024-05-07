// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package runtime

import (
	"context"

	containerdapi "github.com/containerd/containerd"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/namespaces"
	criconstants "github.com/containerd/containerd/pkg/cri/constants"
	"github.com/containerd/containerd/platforms"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/aenix-io/talm/internal/pkg/containers/image"
	"github.com/siderolabs/talos/pkg/machinery/api/common"
	"github.com/siderolabs/talos/pkg/machinery/api/machine"
	"github.com/siderolabs/talos/pkg/machinery/constants"
)

func containerdNamespaceHelper(ctx context.Context, ns common.ContainerdNamespace) (context.Context, error) {
	var namespaceName string

	switch ns {
	case common.ContainerdNamespace_NS_CRI:
		namespaceName = criconstants.K8sContainerdNamespace
	case common.ContainerdNamespace_NS_SYSTEM:
		namespaceName = constants.SystemContainerdNamespace
	case common.ContainerdNamespace_NS_UNKNOWN:
		fallthrough
	default:
		return nil, status.Errorf(codes.InvalidArgument, "invalid namespace %s", ns)
	}

	return namespaces.WithNamespace(ctx, namespaceName), nil
}

// ImageList lists the images in the CRI.
func (s *Server) ImageList(req *machine.ImageListRequest, srv machine.MachineService_ImageListServer) error {
	client, err := containerdapi.New(constants.CRIContainerdAddress)
	if err != nil {
		return status.Errorf(codes.Unavailable, "error connecting to containerd: %s", err)
	}
	//nolint:errcheck
	defer client.Close()

	ctx, err := containerdNamespaceHelper(srv.Context(), req.Namespace)
	if err != nil {
		return err
	}

	images, err := client.ImageService().List(ctx)
	if err != nil {
		return err
	}

	for _, image := range images {
		item := &machine.ImageListResponse{
			Name:      image.Name,
			Digest:    image.Target.Digest.String(),
			CreatedAt: timestamppb.New(image.CreatedAt),
		}

		size, err := image.Size(ctx, client.ContentStore(), platforms.Default())
		if err == nil {
			item.Size = size
		}

		if err = srv.Send(item); err != nil {
			return err
		}
	}

	return nil
}

// ImagePull pulls an image to the CRI.
func (s *Server) ImagePull(ctx context.Context, req *machine.ImagePullRequest) (*machine.ImagePullResponse, error) {
	client, err := containerdapi.New(constants.CRIContainerdAddress)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "error connecting to containerd: %s", err)
	}
	//nolint:errcheck
	defer client.Close()

	ctx, err = containerdNamespaceHelper(ctx, req.Namespace)
	if err != nil {
		return nil, err
	}

	_, err = image.Pull(ctx, s.Controller.Runtime().Config().Machine().Registries(), client, req.Reference, image.WithSkipIfAlreadyPulled())
	if err != nil {
		if errdefs.IsNotFound(err) {
			return nil, status.Errorf(codes.NotFound, "error pulling image: %s", err)
		}

		return nil, err
	}

	return &machine.ImagePullResponse{
		Messages: []*machine.ImagePull{
			{},
		},
	}, nil
}
