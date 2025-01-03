// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package extensions_test

import (
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aenix-io/talm/internal/pkg/extensions"
	"github.com/siderolabs/talos/pkg/machinery/imager/quirks"
)

func TestCompress(t *testing.T) {
	// Compress is going to change contents of the extension, copy to some temporary directory
	extDir := t.TempDir()

	require.NoError(t, exec.Command("cp", "-r", "testdata/good/extension1", extDir).Run())

	exts, err := extensions.List(extDir)
	require.NoError(t, err)

	require.Len(t, exts, 1)

	ext := exts[0]

	squashDest, initramfsDest := t.TempDir(), t.TempDir()
	squashFile, err := ext.Compress(squashDest, initramfsDest, quirks.New(""))
	assert.NoError(t, err)

	assert.FileExists(t, squashFile)
	assert.FileExists(t, filepath.Join(initramfsDest, "lib", "firmware", "amd", "cpu"))
}
