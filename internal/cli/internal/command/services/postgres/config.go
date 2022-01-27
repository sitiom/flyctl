package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/r3labs/diff"
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/cli/internal/app"
	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/flag"
	"github.com/superfly/flyctl/internal/cli/internal/prompt"
	"github.com/superfly/flyctl/internal/cli/internal/render"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/pkg/agent"
	"github.com/superfly/flyctl/pkg/iostreams"
)

func newConfig() (cmd *cobra.Command) {
	// TODO - Add better top level docs.
	const (
		long = `
`
		short = ""
	)

	cmd = command.New("config", short, long, nil)

	cmd.AddCommand(
		newConfigView(),
		newConfigUpdate(),
	)

	return
}

func newConfigView() (cmd *cobra.Command) {
	const (
		long = `Configure postgres cluster
`
		short = "Configure postgres cluster"
		usage = "view"
	)

	cmd = command.New(usage, short, long, runConfigView,
		command.RequireSession,
		command.RequireAppName,
	)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
	)

	return
}

func runConfigView(ctx context.Context) error {
	client := client.FromContext(ctx).API()
	appName := app.NameFromContext(ctx)

	app, err := client.GetApp(ctx, appName)
	if err != nil {
		return fmt.Errorf("get app: %w", err)
	}

	agentclient, err := agent.Establish(ctx, client)
	if err != nil {
		return fmt.Errorf("error establishing agent: %w", err)
	}

	dialer, err := agentclient.Dialer(ctx, app.Organization.Slug)
	if err != nil {
		return fmt.Errorf("ssh: can't build tunnel for %s: %s", app.Organization.Slug, err)
	}

	pgCmd := newPostgresCmd(ctx, app, dialer)

	resp, err := pgCmd.viewStolonConfig()
	if err != nil {
		return err
	}

	str, err := json.MarshalIndent(resp, "", "\t")
	if err != nil {
		return err
	}
	fmt.Println(string(str))

	return nil
}

func newConfigUpdate() (cmd *cobra.Command) {
	const (
		long = `Configure postgres cluster
`
		short = "Configure postgres cluster"
		usage = "update"
	)

	cmd = command.New(usage, short, long, runConfigUpdate,
		command.RequireSession,
		command.RequireAppName,
	)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.String{
			Name:        "max-connections",
			Description: "Sets the maximum number of concurrent connections.",
		},
		flag.String{
			Name:        "wal-level",
			Description: "Sets the level of information written to the WAL. (minimal, replica, logical)",
		},
		flag.String{
			Name:        "log-duration",
			Description: "Logs the duration of each completed SQL statement. (on, off)",
		},
		flag.String{
			Name:        "automatic-pg-restart",
			Description: "Restart postgres automatically after changing pgParameters that requires a restart. (true, false)",
		},
	)

	return
}

func runConfigUpdate(ctx context.Context) error {
	client := client.FromContext(ctx).API()
	appName := app.NameFromContext(ctx)

	app, err := client.GetApp(ctx, appName)
	if err != nil {
		return fmt.Errorf("get app: %w", err)
	}

	agentclient, err := agent.Establish(ctx, client)
	if err != nil {
		return fmt.Errorf("error establishing agent: %w", err)
	}

	dialer, err := agentclient.Dialer(ctx, app.Organization.Slug)
	if err != nil {
		return fmt.Errorf("ssh: can't build tunnel for %s: %s", app.Organization.Slug, err)
	}

	maxConnections := flag.GetString(ctx, "max-connections")
	walLevel := flag.GetString(ctx, "wal-level")
	logDuration := flag.GetString(ctx, "log-duration")

	pgCmd := newPostgresCmd(ctx, app, dialer)

	// Original stolon configuration
	oCfg, err := pgCmd.viewStolonConfig()
	if err != nil {
		return err
	}

	// Target stolon configuration
	nCfg := &stolonSpec{
		AutomaticPgRestart: oCfg.AutomaticPgRestart, // Default
		PGParameters: &pgParameters{
			LogDuration:    logDuration,
			MaxConnections: maxConnections,
			WalLevel:       walLevel,
		},
	}

	// TODO - See if there's a cleaner way to accommodate this.
	// Ideally we wouldn't send this option if the value doesn't actually change,
	// however, omitempty doesn't play nicely with booleans.
	autoRestart := flag.GetString(ctx, "automatic-pg-restart")
	if autoRestart != "" {
		val, err := strconv.ParseBool(autoRestart)
		if err != nil {
			return err
		}
		nCfg.AutomaticPgRestart = val
	}

	changelog, _ := diff.Diff(oCfg, nCfg)
	if len(changelog) == 0 {
		return fmt.Errorf("no changes to apply")
	}

	out := iostreams.FromContext(ctx).Out

	rows := make([][]string, 0, len(changelog))
	for _, change := range changelog {
		rows = append(rows, []string{
			change.Path[len(change.Path)-1],
			fmt.Sprint(change.From),
			fmt.Sprint(change.To),
		})
	}
	_ = render.Table(out, "", rows, "Configuration option", "Original", "Target")

	confirm, err := prompt.Confirm(ctx, fmt.Sprintf("Are you sure you want to apply these changes?"))
	if err != nil {
		return err
	}

	if confirm {
		err = pgCmd.updateStolonConfig(nCfg)
		if err != nil {
			return err
		}
		fmt.Fprintln(out, "Changes have been successfully applied!")
	}

	return nil
}
