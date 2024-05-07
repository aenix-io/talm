// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package files

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"

	"github.com/cosi-project/runtime/pkg/controller"
	"github.com/cosi-project/runtime/pkg/resource"
	"go.uber.org/zap"

	"github.com/aenix-io/talm/internal/pkg/toml"
	"github.com/siderolabs/talos/pkg/machinery/constants"
	"github.com/siderolabs/talos/pkg/machinery/resources/files"
)

// CRIConfigPartsController merges parts of the CRI config from /etc/cri/conf.d/*.part into final /etc/cri/conf.d/cri.toml.
type CRIConfigPartsController struct {
	// Path to /etc/cri/conf.d directory.
	CRIConfdPath string
}

// Name implements controller.Controller interface.
func (ctrl *CRIConfigPartsController) Name() string {
	return "files.CRIConfigPartsController"
}

// Inputs implements controller.Controller interface.
func (ctrl *CRIConfigPartsController) Inputs() []controller.Input {
	return []controller.Input{
		{
			Namespace: files.NamespaceName,
			Type:      files.EtcFileStatusType,
			Kind:      controller.InputWeak,
		},
	}
}

// Outputs implements controller.Controller interface.
func (ctrl *CRIConfigPartsController) Outputs() []controller.Output {
	return []controller.Output{
		{
			Type: files.EtcFileSpecType,
			Kind: controller.OutputShared,
		},
	}
}

// Run implements controller.Controller interface.
func (ctrl *CRIConfigPartsController) Run(ctx context.Context, r controller.Runtime, logger *zap.Logger) error {
	if ctrl.CRIConfdPath == "" {
		ctrl.CRIConfdPath = constants.CRIConfdPath
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-r.EventCh():
		}

		// scan conf.d directory for config parts and merge them together into final configuration
		parts, err := filepath.Glob(filepath.Join(ctrl.CRIConfdPath, "*.part"))
		if err != nil {
			return err
		}

		sort.Strings(parts)

		out, err := toml.Merge(parts)
		if err != nil {
			return err
		}

		if err := r.Modify(ctx, files.NewEtcFileSpec(files.NamespaceName, constants.CRIConfig),
			func(r resource.Resource) error {
				spec := r.(*files.EtcFileSpec).TypedSpec()

				spec.Contents = out
				spec.Mode = 0o600

				return nil
			}); err != nil {
			return fmt.Errorf("error modifying resource: %w", err)
		}

		r.ResetRestartBackoff()
	}
}
