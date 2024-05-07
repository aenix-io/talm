// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package meta provides access to META partition: key-value partition persisted across reboots.
package meta

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"sync"

	"github.com/cosi-project/runtime/pkg/resource"
	"github.com/cosi-project/runtime/pkg/safe"
	"github.com/cosi-project/runtime/pkg/state"
	"github.com/siderolabs/go-blockdevice/blockdevice/probe"

	"github.com/aenix-io/talm/internal/pkg/meta/internal/adv"
	"github.com/aenix-io/talm/internal/pkg/meta/internal/adv/syslinux"
	"github.com/aenix-io/talm/internal/pkg/meta/internal/adv/talos"
	"github.com/siderolabs/talos/pkg/machinery/constants"
	"github.com/siderolabs/talos/pkg/machinery/resources/runtime"
)

// Meta represents the META reader/writer.
//
// Meta abstracts away all details about loading/storing the metadata providing an easy to use interface.
type Meta struct {
	mu sync.Mutex

	legacy adv.ADV
	talos  adv.ADV
	state  state.State
	opts   Options
}

// Options configures the META.
type Options struct {
	fixedPath string
	printer   func(string, ...any)
}

// Option is a functional option.
type Option func(*Options)

// WithFixedPath sets the fixed path to META partition.
func WithFixedPath(path string) Option {
	return func(o *Options) {
		o.fixedPath = path
	}
}

// WithPrinter sets the function to print the logs, default is log.Printf.
func WithPrinter(printer func(string, ...any)) Option {
	return func(o *Options) {
		o.printer = printer
	}
}

// New initializes empty META, trying to probe the existing META first.
func New(ctx context.Context, st state.State, opts ...Option) (*Meta, error) {
	meta := &Meta{
		state: st,
		opts: Options{
			printer: log.Printf,
		},
	}

	for _, opt := range opts {
		opt(&meta.opts)
	}

	var err error

	meta.legacy, err = syslinux.NewADV(nil)
	if err != nil {
		return nil, err
	}

	meta.talos, err = talos.NewADV(nil)
	if err != nil {
		return nil, err
	}

	err = meta.Reload(ctx)

	return meta, err
}

func (meta *Meta) getPath() (string, error) {
	if meta.opts.fixedPath != "" {
		return meta.opts.fixedPath, nil
	}

	dev, err := probe.GetDevWithPartitionName(constants.MetaPartitionLabel)
	if err != nil {
		return "", err
	}

	defer dev.Close() //nolint:errcheck

	return dev.PartPath(constants.MetaPartitionLabel)
}

// Reload refreshes the META from the disk.
func (meta *Meta) Reload(ctx context.Context) error {
	meta.mu.Lock()
	defer meta.mu.Unlock()

	path, err := meta.getPath()
	if err != nil {
		return err
	}

	f, err := os.Open(path)
	if err != nil {
		return err
	}

	defer f.Close() //nolint:errcheck

	adv, err := talos.NewADV(f)
	if adv == nil && err != nil {
		// if adv is not nil, but err is nil, it might be missing ADV, ignore it
		return err
	}

	legacyAdv, err := syslinux.NewADV(f)
	if err != nil {
		return err
	}

	// copy values from in-memory to on-disk version
	for _, t := range meta.talos.ListTags() {
		val, _ := meta.talos.ReadTagBytes(t)
		adv.SetTagBytes(t, val)
	}

	meta.opts.printer("META: loaded %d keys", len(adv.ListTags()))

	meta.talos = adv
	meta.legacy = legacyAdv

	return meta.syncState(ctx)
}

// syncState sync resources with adv contents.
func (meta *Meta) syncState(ctx context.Context) error {
	if meta.state == nil {
		return nil
	}

	existingTags := make(map[resource.ID]struct{})

	for _, t := range meta.talos.ListTags() {
		existingTags[runtime.MetaKeyTagToID(t)] = struct{}{}
		val, _ := meta.talos.ReadTag(t)

		if err := updateTagResource(ctx, meta.state, t, val); err != nil {
			return err
		}
	}

	items, err := meta.state.List(ctx, runtime.NewMetaKey(runtime.NamespaceName, "").Metadata())
	if err != nil {
		return err
	}

	for _, item := range items.Items {
		if _, exists := existingTags[item.Metadata().ID()]; exists {
			continue
		}

		if err = meta.state.Destroy(ctx, item.Metadata()); err != nil {
			return err
		}
	}

	return nil
}

// Flush writes the META to the disk.
func (meta *Meta) Flush() error {
	meta.mu.Lock()
	defer meta.mu.Unlock()

	path, err := meta.getPath()
	if err != nil {
		return err
	}

	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return err
	}

	defer f.Close() //nolint:errcheck

	serialized, err := meta.talos.Bytes()
	if err != nil {
		return err
	}

	n, err := f.WriteAt(serialized, 0)
	if err != nil {
		return err
	}

	if n != len(serialized) {
		return fmt.Errorf("expected to write %d bytes, wrote %d", len(serialized), n)
	}

	serialized, err = meta.legacy.Bytes()
	if err != nil {
		return err
	}

	offset, err := f.Seek(-int64(len(serialized)), io.SeekEnd)
	if err != nil {
		return err
	}

	n, err = f.WriteAt(serialized, offset)
	if err != nil {
		return err
	}

	if n != len(serialized) {
		return fmt.Errorf("expected to write %d bytes, wrote %d", len(serialized), n)
	}

	meta.opts.printer("META: saved %d keys", len(meta.talos.ListTags()))

	return f.Sync()
}

// ReadTag reads a tag from the META.
func (meta *Meta) ReadTag(t uint8) (val string, ok bool) {
	meta.mu.Lock()
	defer meta.mu.Unlock()

	val, ok = meta.talos.ReadTag(t)
	if !ok {
		val, ok = meta.legacy.ReadTag(t)
	}

	return val, ok
}

// ReadTagBytes reads a tag from the META.
func (meta *Meta) ReadTagBytes(t uint8) (val []byte, ok bool) {
	meta.mu.Lock()
	defer meta.mu.Unlock()

	val, ok = meta.talos.ReadTagBytes(t)
	if !ok {
		val, ok = meta.legacy.ReadTagBytes(t)
	}

	return val, ok
}

// SetTag writes a tag to the META.
func (meta *Meta) SetTag(ctx context.Context, t uint8, val string) (bool, error) {
	meta.mu.Lock()
	defer meta.mu.Unlock()

	ok := meta.talos.SetTag(t, val)

	if ok {
		err := updateTagResource(ctx, meta.state, t, val)
		if err != nil {
			return false, err
		}
	}

	return ok, nil
}

// SetTagBytes writes a tag to the META.
func (meta *Meta) SetTagBytes(ctx context.Context, t uint8, val []byte) (bool, error) {
	meta.mu.Lock()
	defer meta.mu.Unlock()

	ok := meta.talos.SetTagBytes(t, val)

	if ok {
		err := updateTagResource(ctx, meta.state, t, string(val))
		if err != nil {
			return false, err
		}
	}

	return ok, nil
}

// DeleteTag deletes a tag from the META.
func (meta *Meta) DeleteTag(ctx context.Context, t uint8) (bool, error) {
	meta.mu.Lock()
	defer meta.mu.Unlock()

	ok := meta.talos.DeleteTag(t)
	if !ok {
		ok = meta.legacy.DeleteTag(t)
	}

	if meta.state == nil {
		return ok, nil
	}

	err := meta.state.Destroy(ctx, runtime.NewMetaKey(runtime.NamespaceName, runtime.MetaKeyTagToID(t)).Metadata())
	if state.IsNotFoundError(err) {
		err = nil
	}

	return ok, err
}

func updateTagResource(ctx context.Context, st state.State, t uint8, val string) error {
	if st == nil {
		return nil
	}

	_, err := safe.StateUpdateWithConflicts(ctx, st, runtime.NewMetaKey(runtime.NamespaceName, runtime.MetaKeyTagToID(t)).Metadata(), func(r *runtime.MetaKey) error {
		r.TypedSpec().Value = val

		return nil
	})

	if err == nil {
		return nil
	}

	if state.IsNotFoundError(err) {
		r := runtime.NewMetaKey(runtime.NamespaceName, runtime.MetaKeyTagToID(t))
		r.TypedSpec().Value = val

		return st.Create(ctx, r)
	}

	return err
}
