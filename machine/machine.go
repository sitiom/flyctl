package machine

import (
	"context"
	"fmt"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/iostreams"
)

type MachineApp struct {
	Machines []*api.Machine
	Config   api.MachineConfig
	Client   *flaps.Client
	ApiInput *api.LaunchMachineInput
}

func NewMachineApp(ctx context.Context, app *api.AppCompact) (machineApp *MachineApp, err error) {
	client, err := flaps.New(ctx, app)
	if err != nil {
		return nil, err
	}
	return &MachineApp{
		Client: client,
		Config: api.MachineConfig{},
		ApiInput: &api.LaunchMachineInput{
			AppID:   app.Name,
			OrgSlug: app.Organization.ID,
		},
	}, nil
}

func (m *MachineApp) GetMachines(ctx context.Context) (err error) {
	m.Machines, err = m.Client.List(ctx, "")
	return
}

func (m *MachineApp) LeaseAll(ctx context.Context) (err error) {
	out := iostreams.FromContext(ctx).Out

	for _, machine := range m.Machines {

		fmt.Fprintf(out, "Taking lease out on VM %s\n", machine.ID)
		leaseTTL := api.IntPointer(30)
		lease, err := m.Client.GetLease(ctx, machine.ID, leaseTTL)

		if err != nil {
			return err
		}

		machine.LeaseNonce = lease.Data.Nonce

	}

	return
}

func (m *MachineApp) UpdateAll(ctx context.Context) (err error) {
	out := iostreams.FromContext(ctx).Out

	for _, machine := range m.Machines {

		fmt.Fprintf(out, "Updating VM %s\n", machine.ID)
		updateInput := *m.ApiInput
		updateInput.ID = machine.ID
		updateResult, err := m.Client.Update(ctx, updateInput, machine.LeaseNonce)

		if err != nil {
			return err
		}

		fmt.Fprintf(out, "Waiting for update to finish on %s\n", machine.ID)
		err = m.Client.Wait(ctx, updateResult)

		if err != nil {
			return err
		}

	}
	return
}

func (m *MachineApp) ReleaseAll(ctx context.Context) (err error) {
	out := iostreams.FromContext(ctx).Out

	for _, machine := range m.Machines {
		fmt.Fprintf(out, "Releasing lease on %s\n", machine.ID)
		err = m.Client.ReleaseLease(ctx, machine.ID, machine.LeaseNonce)

		if err != nil {
			return err
		}
	}

	return
}

func (m *MachineApp) LaunchMachine(ctx context.Context) (err error) {
	out := iostreams.FromContext(ctx).Out
	fmt.Fprintf(out, "Launching VM with image %s\n", m.Config.Image)
	_, err = m.Client.Launch(ctx, *m.ApiInput)

	return
}
