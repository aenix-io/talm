// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package block

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"time"

	"github.com/cosi-project/runtime/pkg/controller"
	"github.com/cosi-project/runtime/pkg/safe"
	"github.com/cosi-project/runtime/pkg/state"
	"github.com/siderolabs/gen/maps"
	"github.com/siderolabs/go-blockdevice/v2/blkid"
	"go.uber.org/zap"

	"github.com/siderolabs/talos/pkg/machinery/resources/block"
)

// DiscoveryController provides a filesystem/partition discovery for blockdevices.
type DiscoveryController struct{}

// Name implements controller.Controller interface.
func (ctrl *DiscoveryController) Name() string {
	return "block.DiscoveryController"
}

// Inputs implements controller.Controller interface.
func (ctrl *DiscoveryController) Inputs() []controller.Input {
	return []controller.Input{
		{
			Namespace: block.NamespaceName,
			Type:      block.DeviceType,
			Kind:      controller.InputWeak,
		},
	}
}

// Outputs implements controller.Controller interface.
func (ctrl *DiscoveryController) Outputs() []controller.Output {
	return []controller.Output{
		{
			Type: block.DiscoveredVolumeType,
			Kind: controller.OutputExclusive,
		},
	}
}

// Run implements controller.Controller interface.
//
//nolint:gocyclo
func (ctrl *DiscoveryController) Run(ctx context.Context, r controller.Runtime, logger *zap.Logger) error {
	// lastObservedGenerations holds the last observed generation of each device.
	//
	// when the generation of a device changes, the device might have changed and might need to be re-probed.
	lastObservedGenerations := map[string]int{}

	// nextRescan holds the pool of devices to be rescanned in the next batch.
	nextRescan := map[string]int{}

	rescanTicker := time.NewTicker(100 * time.Millisecond)
	defer rescanTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-rescanTicker.C:
			if len(nextRescan) == 0 {
				continue
			}

			if err := ctrl.rescan(ctx, r, logger, maps.Keys(nextRescan)); err != nil {
				return fmt.Errorf("failed to rescan devices: %w", err)
			}

			nextRescan = map[string]int{}
		case <-r.EventCh():
			devices, err := safe.ReaderListAll[*block.Device](ctx, r)
			if err != nil {
				return fmt.Errorf("failed to list devices: %w", err)
			}

			parents := map[string]string{}
			allDevices := map[string]struct{}{}

			for iterator := devices.Iterator(); iterator.Next(); {
				device := iterator.Value()

				allDevices[device.Metadata().ID()] = struct{}{}

				if device.TypedSpec().Parent != "" {
					parents[device.Metadata().ID()] = device.TypedSpec().Parent
				}

				if device.TypedSpec().Generation == lastObservedGenerations[device.Metadata().ID()] {
					continue
				}

				nextRescan[device.Metadata().ID()] = device.TypedSpec().Generation
				lastObservedGenerations[device.Metadata().ID()] = device.TypedSpec().Generation
			}

			// remove child devices if the parent is marked for rescan
			for id := range nextRescan {
				if parent, ok := parents[id]; ok {
					if _, ok := nextRescan[parent]; ok {
						delete(nextRescan, id)
					}
				}
			}

			// if the device is removed, add it to the nextRescan, and remove from lastObservedGenerations
			for id := range lastObservedGenerations {
				if _, ok := allDevices[id]; !ok {
					nextRescan[id] = lastObservedGenerations[id]
					delete(lastObservedGenerations, id)
				}
			}
		}
	}
}

//nolint:gocyclo
func (ctrl *DiscoveryController) rescan(ctx context.Context, r controller.Runtime, logger *zap.Logger, ids []string) error {
	failedIDs := map[string]struct{}{}
	touchedIDs := map[string]struct{}{}

	for _, id := range ids {
		device, err := safe.ReaderGetByID[*block.Device](ctx, r, id)
		if err != nil {
			if state.IsNotFoundError(err) {
				failedIDs[id] = struct{}{}

				continue
			}

			return fmt.Errorf("failed to get device: %w", err)
		}

		info, err := blkid.ProbePath(filepath.Join("/dev", id))
		if err != nil {
			logger.Debug("failed to probe device", zap.String("id", id), zap.Error(err))

			failedIDs[id] = struct{}{}

			continue
		}

		if err = safe.WriterModify(ctx, r, block.NewDiscoveredVolume(block.NamespaceName, id), func(dv *block.DiscoveredVolume) error {
			dv.TypedSpec().Type = device.TypedSpec().Type
			dv.TypedSpec().DevicePath = device.TypedSpec().DevicePath
			dv.TypedSpec().Parent = device.TypedSpec().Parent

			dv.TypedSpec().Size = info.Size
			dv.TypedSpec().SectorSize = info.SectorSize
			dv.TypedSpec().IOSize = info.IOSize

			ctrl.fillDiscoveredVolumeFromInfo(dv, info.ProbeResult)

			return nil
		}); err != nil {
			return fmt.Errorf("failed to write discovered volume: %w", err)
		}

		touchedIDs[id] = struct{}{}

		for _, nested := range info.Parts {
			partID := partitionID(id, nested.PartitionIndex)

			if err = safe.WriterModify(ctx, r, block.NewDiscoveredVolume(block.NamespaceName, partID), func(dv *block.DiscoveredVolume) error {
				dv.TypedSpec().Type = "partition"
				dv.TypedSpec().DevicePath = filepath.Join(device.TypedSpec().DevicePath, partID)
				dv.TypedSpec().Parent = id

				dv.TypedSpec().Size = nested.ProbedSize
				dv.TypedSpec().SectorSize = info.SectorSize
				dv.TypedSpec().IOSize = info.IOSize

				ctrl.fillDiscoveredVolumeFromInfo(dv, nested.ProbeResult)

				if nested.PartitionUUID != nil {
					dv.TypedSpec().PartitionUUID = nested.PartitionUUID.String()
				} else {
					dv.TypedSpec().PartitionUUID = ""
				}

				if nested.PartitionType != nil {
					dv.TypedSpec().PartitionType = nested.PartitionType.String()
				} else {
					dv.TypedSpec().PartitionType = ""
				}

				if nested.PartitionLabel != nil {
					dv.TypedSpec().PartitionLabel = *nested.PartitionLabel
				} else {
					dv.TypedSpec().PartitionLabel = ""
				}

				dv.TypedSpec().PartitionIndex = nested.PartitionIndex

				return nil
			}); err != nil {
				return fmt.Errorf("failed to write discovered volume: %w", err)
			}

			touchedIDs[partID] = struct{}{}
		}
	}

	// clean up discovered volumes
	discoveredVolumes, err := safe.ReaderListAll[*block.DiscoveredVolume](ctx, r)
	if err != nil {
		return fmt.Errorf("failed to list discovered volumes: %w", err)
	}

	for iterator := discoveredVolumes.Iterator(); iterator.Next(); {
		dv := iterator.Value()

		if _, ok := touchedIDs[dv.Metadata().ID()]; ok {
			continue
		}

		_, isFailed := failedIDs[dv.Metadata().ID()]

		parentTouched := false

		if dv.TypedSpec().Parent != "" {
			if _, ok := touchedIDs[dv.TypedSpec().Parent]; ok {
				parentTouched = true
			}
		}

		if isFailed || parentTouched {
			// if the probe failed, or if the parent was touched, while this device was not, remove it
			if err = r.Destroy(ctx, dv.Metadata()); err != nil {
				return fmt.Errorf("failed to destroy discovered volume: %w", err)
			}
		}
	}

	return nil
}

func (ctrl *DiscoveryController) fillDiscoveredVolumeFromInfo(dv *block.DiscoveredVolume, info blkid.ProbeResult) {
	dv.TypedSpec().Name = info.Name

	if info.UUID != nil {
		dv.TypedSpec().UUID = info.UUID.String()
	} else {
		dv.TypedSpec().UUID = ""
	}

	if info.Label != nil {
		dv.TypedSpec().Label = *info.Label
	} else {
		dv.TypedSpec().Label = ""
	}

	dv.TypedSpec().BlockSize = info.BlockSize
	dv.TypedSpec().FilesystemBlockSize = info.FilesystemBlockSize
	dv.TypedSpec().ProbedSize = info.ProbedSize
}

func partitionID(devname string, part uint) string {
	result := devname

	if len(result) > 0 && result[len(result)-1] >= '0' && result[len(result)-1] <= '9' {
		result += "p"
	}

	return result + strconv.FormatUint(uint64(part), 10)
}
