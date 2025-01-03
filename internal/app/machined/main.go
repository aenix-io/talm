// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package machined provides machined implementation.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/hashicorp/go-cleanhttp"
	"github.com/siderolabs/go-cmd/pkg/cmd/proc"
	"github.com/siderolabs/go-cmd/pkg/cmd/proc/reaper"
	debug "github.com/siderolabs/go-debug"
	"github.com/siderolabs/go-procfs/procfs"
	"go.uber.org/zap"
	"golang.org/x/sys/unix"

	"github.com/aenix-io/talm/internal/app/apid"
	"github.com/aenix-io/talm/internal/app/dashboard"
	"github.com/aenix-io/talm/internal/app/machined/pkg/runtime"
	"github.com/aenix-io/talm/internal/app/machined/pkg/runtime/emergency"
	v1alpha1runtime "github.com/aenix-io/talm/internal/app/machined/pkg/runtime/v1alpha1"
	startuptasks "github.com/aenix-io/talm/internal/app/machined/pkg/startup"
	"github.com/aenix-io/talm/internal/app/machined/pkg/system"
	"github.com/aenix-io/talm/internal/app/machined/pkg/system/services"
	"github.com/aenix-io/talm/internal/app/maintenance"
	"github.com/aenix-io/talm/internal/app/poweroff"
	"github.com/aenix-io/talm/internal/app/trustd"
	"github.com/aenix-io/talm/internal/pkg/mount/v2"
	"github.com/siderolabs/talos/pkg/httpdefaults"
	"github.com/siderolabs/talos/pkg/machinery/api/common"
	"github.com/siderolabs/talos/pkg/machinery/api/machine"
	"github.com/siderolabs/talos/pkg/machinery/constants"
	"github.com/siderolabs/talos/pkg/startup"
)

func init() {
	// Patch a default HTTP client with updated transport to handle cases when default client is being used.
	http.DefaultClient.Transport = httpdefaults.PatchTransport(cleanhttp.DefaultPooledTransport())
}

func recovery(ctx context.Context) {
	if r := recover(); r != nil {
		var (
			err error
			ok  bool
		)

		err, ok = r.(error)
		if ok {
			handle(ctx, err)
		}
	}
}

// syncNonVolatileStorageBuffers invokes unix.Sync and waits up to 30 seconds
// for it to finish.
//
// See http://man7.org/linux/man-pages/man2/reboot.2.html.
func syncNonVolatileStorageBuffers() {
	syncdone := make(chan struct{})

	go func() {
		defer close(syncdone)

		unix.Sync()
	}()

	log.Printf("waiting for sync...")

	for i := 29; i >= 0; i-- {
		select {
		case <-syncdone:
			log.Printf("sync done")

			return
		case <-time.After(time.Second):
		}

		if i != 0 {
			log.Printf("waiting %d more seconds for sync to finish", i)
		}
	}

	log.Printf("sync hasn't completed in time, aborting...")
}

//nolint:gocyclo
func handle(ctx context.Context, err error) {
	rebootCmd := int(emergency.RebootCmd.Load())

	var rebootErr runtime.RebootError

	if errors.As(err, &rebootErr) {
		// not a failure, but wrapped reboot command
		rebootCmd = rebootErr.Cmd

		err = nil
	}

	if err != nil {
		log.Print(err)
		revertBootloader(ctx)

		if p := procfs.ProcCmdline().Get(constants.KernelParamPanic).First(); p != nil {
			if *p == "0" {
				log.Printf("panic=0 kernel flag found, sleeping forever")

				rebootCmd = 0
			}
		}

		if rebootCmd == unix.LINUX_REBOOT_CMD_RESTART {
			for i := 10; i >= 0; i-- {
				log.Printf("rebooting in %d seconds\n", i)
				time.Sleep(1 * time.Second)
			}
		}
	}

	if err = proc.KillAll(); err != nil {
		log.Printf("error killing all procs: %s", err)
	}

	if err = mount.UnmountAll(); err != nil {
		log.Printf("error unmounting: %s", err)
	}

	syncNonVolatileStorageBuffers()

	if rebootCmd == 0 {
		exitSignal := make(chan os.Signal, 1)

		signal.Notify(exitSignal, syscall.SIGINT, syscall.SIGTERM)

		<-exitSignal
	} else if unix.Reboot(rebootCmd) == nil {
		// Wait forever.
		select {}
	}
}

func runDebugServer(ctx context.Context) {
	const debugAddr = ":9982"

	debugLogFunc := func(msg string) {
		log.Print(msg)
	}

	if err := debug.ListenAndServe(ctx, debugAddr, debugLogFunc); err != nil {
		log.Fatalf("failed to start debug server: %s", err)
	}
}

func run() error {
	// Limit GOMAXPROCS.
	startup.LimitMaxProcs(constants.MachinedMaxProcs)

	// Initialize the controller without a config.
	c, err := v1alpha1runtime.NewController()
	if err != nil {
		return err
	}

	revertSetState(c.Runtime().State().V1Alpha2().Resources())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	logger, err := c.V1Alpha2().MakeLogger("early-startup")
	if err != nil {
		return err
	}

	start := time.Now()

	// Run startup tasks, and then run the entrypoint.
	return startuptasks.RunTasks(ctx, logger, c.Runtime(), append(
		startuptasks.DefaultTasks(),
		func(ctx context.Context, log *zap.Logger, _ runtime.Runtime, _ startuptasks.NextTaskFunc) error {
			logger.Info("early startup done", zap.Duration("duration", time.Since(start)))

			return runEntrypoint(ctx, c)
		},
	)...)
}

//nolint:gocyclo
func runEntrypoint(ctx context.Context, c *v1alpha1runtime.Controller) error {
	errCh := make(chan error)

	var controllerWaitGroup sync.WaitGroup
	defer controllerWaitGroup.Wait() // wait for controller-runtime to finish before rebooting

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	drainer := runtime.NewDrainer()
	defer func() {
		drainCtx, drainCtxCancel := context.WithTimeout(context.Background(), time.Second*10)
		defer drainCtxCancel()

		if e := drainer.Drain(drainCtx); e != nil {
			log.Printf("WARNING: failed to drain controllers: %s", e)
		}
	}()

	go runDebugServer(ctx)

	// Schedule service shutdown on any return.
	defer system.Services(c.Runtime()).Shutdown(ctx)

	// Start signal and ACPI listeners.
	go func() {
		if e := c.ListenForEvents(ctx); e != nil {
			log.Printf("WARNING: signals and ACPI events will be ignored: %s", e)
		}
	}()

	controllerWaitGroup.Add(1)

	// Start v2 controller runtime.
	go func() {
		defer controllerWaitGroup.Done()

		if e := c.V1Alpha2().Run(ctx, drainer); e != nil {
			ctrlErr := fmt.Errorf("fatal controller runtime error: %s", e)

			log.Printf("controller runtime goroutine error: %s", ctrlErr)

			errCh <- ctrlErr
		}

		log.Printf("controller runtime finished")
	}()

	// Inject controller into maintenance service.
	maintenance.InjectController(c)

	// Load machined service.
	system.Services(c.Runtime()).Load(
		&services.Machined{Controller: c},
	)

	initializeCanceled := false

	// Initialize the machine.
	if err := c.Run(ctx, runtime.SequenceInitialize, nil); err != nil {
		if errors.Is(err, context.Canceled) {
			initializeCanceled = true
		} else {
			return err
		}
	}

	// If Initialize sequence was canceled, don't run any other sequence.
	if !initializeCanceled {
		// Perform an installation if required.
		if err := c.Run(ctx, runtime.SequenceInstall, nil); err != nil {
			return err
		}

		// Start the machine API.
		system.Services(c.Runtime()).LoadAndStart(
			&services.APID{},
		)

		// Boot the machine.
		if err := c.Run(ctx, runtime.SequenceBoot, nil); err != nil && !errors.Is(err, context.Canceled) {
			return err
		}
	}

	// Watch and handle runtime events.
	//nolint:errcheck
	_ = c.Runtime().Events().Watch(
		func(events <-chan runtime.EventInfo) {
			for {
				for event := range events {
					switch msg := event.Payload.(type) {
					case *machine.SequenceEvent:
						if msg.Error != nil {
							if msg.Error.GetCode() == common.Code_LOCKED ||
								msg.Error.GetCode() == common.Code_CANCELED {
								// ignore sequence lock and canceled errors, they're not fatal
								continue
							}

							errCh <- fmt.Errorf(
								"fatal sequencer error in %q sequence: %v",
								msg.GetSequence(),
								msg.GetError().String(),
							)
						}
					case *machine.RestartEvent:
						errCh <- runtime.RebootError{Cmd: int(msg.Cmd)}
					}
				}
			}
		},
	)

	return <-errCh
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	switch filepath.Base(os.Args[0]) {
	case "apid":
		apid.Main()

		return
	case "trustd":
		trustd.Main()

		return
	// Azure uses the hv_utils kernel module to shutdown the node in hyper-v by calling perform_shutdown which will call orderly_poweroff which will call /sbin/poweroff.
	case "poweroff", "shutdown":
		poweroff.Main(os.Args)

		return
	case "dashboard":
		dashboard.Main()

		return
	default:
	}

	// Setup panic handler.
	defer recovery(ctx)

	// Initialize the process reaper.
	reaper.Run()
	defer reaper.Shutdown()

	handle(ctx, run())
}
