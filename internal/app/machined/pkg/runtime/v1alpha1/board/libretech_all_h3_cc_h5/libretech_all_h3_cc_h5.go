// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package libretechallh3cch5 provides the LibretechAllH3CCH5 board implementation.
package libretechallh3cch5

import (
	"os"
	"path/filepath"

	"github.com/siderolabs/go-copy/copy"
	"github.com/siderolabs/go-procfs/procfs"
	"golang.org/x/sys/unix"

	"github.com/aenix-io/talm/internal/app/machined/pkg/runtime"
	"github.com/siderolabs/talos/pkg/machinery/constants"
)

var (
	bin       = constants.BoardLibretechAllH3CCH5 + "/u-boot-sunxi-with-spl.bin"
	off int64 = 1024 * 8
	dtb       = "allwinner/sun50i-h5-libretech-all-h3-cc.dtb"
)

// LibretechAllH3CCH5 represents the Libre Computer ALL-H3-CC (Tritium).
//
// Reference: https://libre.computer/products/boards/all-h3-cc/
type LibretechAllH3CCH5 struct{}

// Name implements the runtime.Board.
func (l *LibretechAllH3CCH5) Name() string {
	return constants.BoardLibretechAllH3CCH5
}

// Install implements the runtime.Board.
func (l *LibretechAllH3CCH5) Install(options runtime.BoardInstallOptions) (err error) {
	var f *os.File

	if f, err = os.OpenFile(options.InstallDisk, os.O_RDWR|unix.O_CLOEXEC, 0o666); err != nil {
		return err
	}
	//nolint:errcheck
	defer f.Close()

	var uboot []byte

	uboot, err = os.ReadFile(filepath.Join(options.UBootPath, bin))
	if err != nil {
		return err
	}

	options.Printf("writing %s at offset %d", bin, off)

	var n int

	n, err = f.WriteAt(uboot, off)
	if err != nil {
		return err
	}

	options.Printf("wrote %d bytes", n)

	// NB: In the case that the block device is a loopback device, we sync here
	// to esure that the file is written before the loopback device is
	// unmounted.
	err = f.Sync()
	if err != nil {
		return err
	}

	src := filepath.Join(options.DTBPath, dtb)
	dst := filepath.Join(options.MountPrefix, "/boot/EFI/dtb", dtb)

	err = os.MkdirAll(filepath.Dir(dst), 0o600)
	if err != nil {
		return err
	}

	err = copy.File(src, dst)
	if err != nil {
		return err
	}

	return nil
}

// KernelArgs implements the runtime.Board.
func (l *LibretechAllH3CCH5) KernelArgs() procfs.Parameters {
	return []*procfs.Parameter{
		procfs.NewParameter("console").Append("tty0").Append("ttyS0,115200"),
		procfs.NewParameter(constants.KernelParamDashboardDisabled).Append("1"),
	}
}

// PartitionOptions implements the runtime.Board.
func (l *LibretechAllH3CCH5) PartitionOptions() *runtime.PartitionOptions {
	return &runtime.PartitionOptions{PartitionsOffset: 2048}
}
