// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package apidata

import (
	"context"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/siderolabs/talos/pkg/machinery/client"
)

// Source is a data source that gathers information about a Talos node using Talos API.
type Source struct {
	*client.Client

	Interval time.Duration

	ctx       context.Context //nolint:containedctx
	ctxCancel context.CancelFunc

	wg sync.WaitGroup
}

// Run the data poll on interval.
func (source *Source) Run(ctx context.Context) <-chan *Data {
	dataCh := make(chan *Data)

	source.ctx, source.ctxCancel = context.WithCancel(ctx)

	source.wg.Add(1)

	go source.run(dataCh)

	return dataCh
}

// Stop the data collection process.
func (source *Source) Stop() {
	source.ctxCancel()

	source.wg.Wait()
}

func (source *Source) run(dataCh chan<- *Data) {
	defer source.wg.Done()
	defer close(dataCh)

	ticker := time.NewTicker(source.Interval)
	defer ticker.Stop()

	var oldData, currentData *Data

	for {
		currentData = source.gather()

		if oldData == nil {
			currentData.CalculateDiff(currentData)
		} else {
			currentData.CalculateDiff(oldData)
		}

		select {
		case dataCh <- currentData:
		case <-source.ctx.Done():
			return
		}

		select {
		case <-source.ctx.Done():
			return
		case <-ticker.C:
		}

		oldData = currentData
	}
}

//nolint:gocyclo,cyclop
func (source *Source) gather() *Data {
	result := &Data{
		Timestamp: time.Now(),
		Nodes:     map[string]*Node{},
	}

	var resultLock sync.Mutex

	gatherFuncs := []func() error{
		func() error {
			resp, err := source.MachineClient.LoadAvg(source.ctx, &emptypb.Empty{})
			if err != nil {
				return err
			}

			resultLock.Lock()
			defer resultLock.Unlock()

			for _, msg := range resp.GetMessages() {
				node := msg.GetMetadata().GetHostname()

				if _, ok := result.Nodes[node]; !ok {
					result.Nodes[node] = &Node{}
				}

				result.Nodes[node].LoadAvg = msg
			}

			return nil
		},
		func() error {
			resp, err := source.MachineClient.Version(source.ctx, &emptypb.Empty{})
			if err != nil {
				return err
			}

			resultLock.Lock()
			defer resultLock.Unlock()

			for _, msg := range resp.GetMessages() {
				node := msg.GetMetadata().GetHostname()

				if _, ok := result.Nodes[node]; !ok {
					result.Nodes[node] = &Node{}
				}

				result.Nodes[node].Version = msg
			}

			return nil
		},
		func() error {
			resp, err := source.MachineClient.Memory(source.ctx, &emptypb.Empty{})
			if err != nil {
				return err
			}

			resultLock.Lock()
			defer resultLock.Unlock()

			for _, msg := range resp.GetMessages() {
				node := msg.GetMetadata().GetHostname()

				if _, ok := result.Nodes[node]; !ok {
					result.Nodes[node] = &Node{}
				}

				result.Nodes[node].Memory = msg
			}

			return nil
		},
		func() error {
			resp, err := source.MachineClient.SystemStat(source.ctx, &emptypb.Empty{})
			if err != nil {
				return err
			}

			resultLock.Lock()
			defer resultLock.Unlock()

			for _, msg := range resp.GetMessages() {
				node := msg.GetMetadata().GetHostname()

				if _, ok := result.Nodes[node]; !ok {
					result.Nodes[node] = &Node{}
				}

				result.Nodes[node].SystemStat = msg
			}

			return nil
		},
		func() error {
			resp, err := source.MachineClient.CPUInfo(source.ctx, &emptypb.Empty{})
			if err != nil {
				return err
			}

			resultLock.Lock()
			defer resultLock.Unlock()

			for _, msg := range resp.GetMessages() {
				node := msg.GetMetadata().GetHostname()

				if _, ok := result.Nodes[node]; !ok {
					result.Nodes[node] = &Node{}
				}

				result.Nodes[node].CPUsInfo = msg
			}

			return nil
		},
		func() error {
			resp, err := source.MachineClient.NetworkDeviceStats(source.ctx, &emptypb.Empty{})
			if err != nil {
				return err
			}

			resultLock.Lock()
			defer resultLock.Unlock()

			for _, msg := range resp.GetMessages() {
				node := msg.GetMetadata().GetHostname()

				if _, ok := result.Nodes[node]; !ok {
					result.Nodes[node] = &Node{}
				}

				result.Nodes[node].NetDevStats = msg
			}

			return nil
		},
		func() error {
			resp, err := source.MachineClient.DiskStats(source.ctx, &emptypb.Empty{})
			if err != nil {
				return err
			}

			resultLock.Lock()
			defer resultLock.Unlock()

			for _, msg := range resp.GetMessages() {
				node := msg.GetMetadata().GetHostname()

				if _, ok := result.Nodes[node]; !ok {
					result.Nodes[node] = &Node{}
				}

				result.Nodes[node].DiskStats = msg
			}

			return nil
		},
		func() error {
			resp, err := source.MachineClient.Processes(source.ctx, &emptypb.Empty{})
			if err != nil {
				return err
			}

			resultLock.Lock()
			defer resultLock.Unlock()

			for _, msg := range resp.GetMessages() {
				node := msg.GetMetadata().GetHostname()

				if _, ok := result.Nodes[node]; !ok {
					result.Nodes[node] = &Node{}
				}

				result.Nodes[node].Processes = msg
			}

			return nil
		},
		func() error {
			resp, err := source.MachineClient.ServiceList(source.ctx, &emptypb.Empty{})
			if err != nil {
				return err
			}

			resultLock.Lock()
			defer resultLock.Unlock()

			for _, msg := range resp.GetMessages() {
				node := msg.GetMetadata().GetHostname()

				if _, ok := result.Nodes[node]; !ok {
					result.Nodes[node] = &Node{}
				}

				result.Nodes[node].ServiceList = msg
			}

			return nil
		},
	}

	var eg errgroup.Group

	for _, f := range gatherFuncs {
		eg.Go(f)
	}

	if err := eg.Wait(); err != nil {
		// TODO: handle error
		_ = err
	}

	return result
}
