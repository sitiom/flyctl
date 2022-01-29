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
		long = `Manage Stolon and Postgres configuration.  Configure postgres cluster
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
			Name:        "log-statement",
			Description: "Sets the type of statements logged. (none, ddl, mod, all)",
		},
		flag.String{
			Name:        "log-min-duration-statement",
			Description: "Sets the minimum execution time above which all statements will be logged. (ms)",
		},
		flag.String{
			Name:        "automatic-pg-restart",
			Description: "Restart postgres automatically after changing any pgParameters that requires a restart. (true, false)",
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
	logStatement := flag.GetString(ctx, "log-statement")
	logMinDurationStatement := flag.GetString(ctx, "log-min-duration-statement")
	autoRestart := flag.GetString(ctx, "automatic-pg-restart")

	pgCmd := newPostgresCmd(ctx, app, dialer)

	// Original stolon configuration
	oCfg, err := pgCmd.viewStolonConfig()
	if err != nil {
		return err
	}

	// New stolon configuration
	var nCfg *stolonSpec

	// Duplicate original configuration.
	oCfgJSON, err := json.Marshal(oCfg)
	if err != nil {
		return err
	}
	json.Unmarshal(oCfgJSON, &nCfg)

	if maxConnections != "" {
		nCfg.PGParameters.MaxConnections = maxConnections
	}
	if walLevel != "" {
		nCfg.PGParameters.WalLevel = walLevel
	}
	if logStatement != "" {
		nCfg.PGParameters.LogStatement = logStatement
	}
	if logMinDurationStatement != "" {
		nCfg.PGParameters.LogMinDurationStatement = logMinDurationStatement
	}
	if autoRestart != "" {
		b, err := strconv.ParseBool(autoRestart)
		if err != nil {
			return err
		}
		nCfg.AutomaticPgRestart = &b
	}

	// Verify that we actually have changes to apply
	changelog, _ := diff.Diff(oCfg, &nCfg)
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
	if !confirm {
		return nil
	}

	fmt.Fprintln(out, "Performing update...")
	err = pgCmd.updateStolonConfig(nCfg)
	if err != nil {
		return err
	}

	fmt.Fprintln(out, "Confirming changes...")
	cfg, err := pgCmd.viewStolonConfig()
	if err != nil {
		return err
	}

	// Diff newly pulled configuration with what was expected.
	changelog, _ = diff.Diff(cfg, &nCfg)
	if len(changelog) != 0 {
		return fmt.Errorf(("Update failed to apply changes..."))
	}

	fmt.Fprintln(out, "Updates were applied successfully!")

	return nil
}
