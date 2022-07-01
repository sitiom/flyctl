package deploy

import (
	"context"
	"fmt"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/build/imgsrc"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/machine"
)

// Deploy ta machines app directly from flyctl, applying the desired config to running machines,
// or launching new ones
func createMachinesRelease(ctx context.Context, config *app.Config, img *imgsrc.DeploymentImage) (err error) {
	io := iostreams.FromContext(ctx)

	client := client.FromContext(ctx).API()

	app, err := client.GetAppCompact(ctx, config.AppName)

	if err != nil {
		return
	}

	mApp, err := machine.NewMachineApp(ctx, app)

	if err != nil {
		return
	}

	mApp.ApiInput.Region = config.GetPrimaryRegion()

	// Run validations against struct types and their JSON tags
	err = config.Validate()

	if err != nil {
		return fmt.Errorf("Invalid fly.toml: %w", err)
	}

	mApp.Config.Image = img.Tag

	// Convert the new, slimmer http service config to standard services
	if config.HttpService != nil {

		httpService := api.MachineService{
			Protocol:     "tcp",
			InternalPort: config.HttpService.InternalPort,
			Ports: []api.MachinePort{
				{
					Port:       80,
					Handlers:   []string{"http"},
					ForceHttps: true,
				},
			},
		}

		httpsService := api.MachineService{
			Protocol:     "tcp",
			InternalPort: config.HttpService.InternalPort,
			Ports: []api.MachinePort{
				{
					Port:     443,
					Handlers: []string{"http", "tls"},
				},
			},
		}

		mApp.Config.Services = append(mApp.Config.Services, httpService, httpsService)
	}

	// Copy standard services to the machine vonfig
	if config.Services != nil {
		mApp.Config.Services = append(mApp.Config.Services, config.Services...)
	}

	if config.Env != nil {
		mApp.Config.Env = config.Env
	}

	if config.Metrics != nil {
		mApp.Config.Metrics = config.Metrics
	}

	err = mApp.GetMachines(ctx)

	if err != nil {
		return
	}

	if len(mApp.Machines) > 0 {

		err = mApp.LeaseAll(ctx)

		if err != nil {
			return err
		}

		err = mApp.UpdateAll(ctx)

		if err != nil {
			return err
		}

		err = mApp.ReleaseAll(ctx)

		if err != nil {
			return err
		}

	} else {

		mApp.LaunchMachine(ctx)
		if err != nil {
			return err
		}

		if err != nil {
			return err
		}
	}

	fmt.Fprintln(io.Out, "Deploy complete. Check the result with 'fly status'.")

	return
}
