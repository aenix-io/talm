// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package grub provides the interface to the GRUB bootloader: config management, installation, etc.
package grub

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/siderolabs/gen/xerrors"

	"github.com/aenix-io/talm/internal/app/machined/pkg/runtime/v1alpha1/bootloader/mount"
	mountv2 "github.com/aenix-io/talm/internal/pkg/mount/v2"
	"github.com/aenix-io/talm/internal/pkg/partition"
	"github.com/siderolabs/talos/pkg/machinery/constants"
)

// Revert reverts the bootloader to the previous version.
func (c *Config) Revert(disk string) error {
	if c == nil {
		return fmt.Errorf("cannot revert bootloader: %w", bootloaderNotInstalledError{})
	}

	err := mount.PartitionOp(
		disk,
		[]mount.Spec{
			{
				PartitionLabel: constants.BootPartitionLabel,
				FilesystemType: partition.FilesystemTypeXFS,
				MountTarget:    constants.BootMountPoint,
			},
		},
		c.revert,
		nil,
		nil,
		[]mountv2.OperationOption{
			mountv2.WithSkipIfMounted(),
		},
		nil,
	)
	if err != nil && !xerrors.TagIs[mount.NotFoundTag](err) {
		return err
	}

	return nil
}

func (c *Config) revert() error {
	if err := c.flip(); err != nil {
		return err
	}

	if _, err := os.Stat(filepath.Join(constants.BootMountPoint, string(c.Default))); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("cannot rollback to %q, label does not exist", "")
	}

	if err := c.Write(ConfigPath, log.Printf); err != nil {
		return fmt.Errorf("failed to revert bootloader: %v", err)
	}

	return nil
}
