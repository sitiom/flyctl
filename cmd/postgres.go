package cmd

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/briandowns/spinner"
	"github.com/logrusorgru/aurora"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/cmd/presenters"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/docstrings"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/client"
)

type PostgresClusterOption struct {
	Name         string
	ImageRef     string
	Password     string
	SnapshotID   string
	Organization string
	Region       string
	Count        int
}
type PostgresConfiguration struct {
	Name             string
	Description      string
	VmSize           string
	MemoryMb         int
	DiskGb           int
	ClusteringOption PostgresClusterOption
}

func postgresConfigurations() []PostgresConfiguration {
	return []PostgresConfiguration{
		{
			Description:      "Development - Single node, 1x shared CPU, 256MB RAM, 10GB disk",
			VmSize:           "shared-cpu-1x",
			MemoryMb:         256,
			DiskGb:           10,
			ClusteringOption: standalonePostgres(),
		},
		{
			Description:      "Production - Highly available, 1x shared CPU, 256MB RAM, 10GB disk",
			VmSize:           "shared-cpu-1x",
			MemoryMb:         256,
			DiskGb:           10,
			ClusteringOption: highlyAvailablePostgres(),
		},
		{
			Description:      "Production - Highly available, 1x Dedicated CPU, 2GB RAM, 50GB disk",
			VmSize:           "dedicated-cpu-1x",
			MemoryMb:         2048,
			DiskGb:           50,
			ClusteringOption: highlyAvailablePostgres(),
		},
		{
			Description:      "Production - Highly available, 2x Dedicated CPU's, 4GB RAM, 100GB disk",
			VmSize:           "dedicated-cpu-2x",
			MemoryMb:         4096,
			DiskGb:           100,
			ClusteringOption: highlyAvailablePostgres(),
		},
		{
			Description:      "Production - Highly available, 4x Dedicated CPU's, 8GB RAM, 200GB disk",
			VmSize:           "dedicated-cpu-4x",
			MemoryMb:         8192,
			DiskGb:           200,
			ClusteringOption: highlyAvailablePostgres(),
		},
		{
			Description: "Specify custom configuration",
			VmSize:      "",
			MemoryMb:    0,
			DiskGb:      0,
		},
	}
}

func standalonePostgres() PostgresClusterOption {
	return PostgresClusterOption{
		Name:     "Development (Single node)",
		ImageRef: "flyio/postgres-standalone",
		Count:    1,
	}
}

func highlyAvailablePostgres() PostgresClusterOption {
	return PostgresClusterOption{
		Name:     "Production (Highly available)",
		ImageRef: "flyio/postgres",
		Count:    2,
	}
}

func postgresClusteringOptions() []PostgresClusterOption {
	return []PostgresClusterOption{
		standalonePostgres(),
		highlyAvailablePostgres(),
	}
}

func newPostgresCommand(client *client.Client) *Command {
	domainsStrings := docstrings.Get("postgres")
	cmd := BuildCommandKS(nil, nil, domainsStrings, client, requireSession)
	cmd.Aliases = []string{"pg"}

	listStrings := docstrings.Get("postgres.list")
	listCmd := BuildCommandKS(cmd, runPostgresList, listStrings, client, requireSession)
	listCmd.Args = cobra.MaximumNArgs(1)

	createStrings := docstrings.Get("postgres.create")
	createCmd := BuildCommandKS(cmd, CreatePostgresClusterFromCommand, createStrings, client, requireSession)
	createCmd.AddStringFlag(StringFlagOpts{Name: "organization", Description: "the organization that will own the app"})
	createCmd.AddStringFlag(StringFlagOpts{Name: "name", Description: "the name of the new app"})
	createCmd.AddStringFlag(StringFlagOpts{Name: "region", Description: "the region to launch the new app in"})
	createCmd.AddStringFlag(StringFlagOpts{Name: "password", Description: "the superuser password. one will be generated for you if you leave this blank"})
	createCmd.AddStringFlag(StringFlagOpts{Name: "volume-size", Description: "the size in GB for volumes"})
	createCmd.AddStringFlag(StringFlagOpts{Name: "vm-size", Description: "the size of the VM"})

	createCmd.AddStringFlag(StringFlagOpts{Name: "image-ref", Hidden: true})
	createCmd.AddStringFlag(StringFlagOpts{Name: "snapshot-id", Description: "Creates the volume with the contents of the snapshot"})

	attachStrngs := docstrings.Get("postgres.attach")
	attachCmd := BuildCommandKS(cmd, runAttachPostgresCluster, attachStrngs, client, requireSession, requireAppName)
	attachCmd.AddStringFlag(StringFlagOpts{Name: "postgres-app", Description: "the postgres cluster to attach to the app"})
	attachCmd.AddStringFlag(StringFlagOpts{Name: "database-name", Description: "database to use, defaults to a new database with the same name as the app"})
	attachCmd.AddStringFlag(StringFlagOpts{Name: "variable-name", Description: "the env variable name that will be added to the app. Defaults to DATABASE_URL"})

	detachStrngs := docstrings.Get("postgres.detach")
	detachCmd := BuildCommandKS(cmd, runDetachPostgresCluster, detachStrngs, client, requireSession, requireAppName)
	detachCmd.AddStringFlag(StringFlagOpts{Name: "postgres-app", Description: "the postgres cluster to detach from the app"})

	dbStrings := docstrings.Get("postgres.db")
	dbCmd := BuildCommandKS(cmd, nil, dbStrings, client, requireSession)

	listDBStrings := docstrings.Get("postgres.db.list")
	listDBCmd := BuildCommandKS(dbCmd, runListPostgresDatabases, listDBStrings, client, requireSession, requireAppNameAsArg)
	listDBCmd.Args = cobra.ExactArgs(1)

	usersStrings := docstrings.Get("postgres.users")
	usersCmd := BuildCommandKS(cmd, nil, usersStrings, client, requireSession)

	usersListStrings := docstrings.Get("postgres.users.list")
	usersListCmd := BuildCommandKS(usersCmd, runListPostgresUsers, usersListStrings, client, requireSession, requireAppNameAsArg)
	usersListCmd.Args = cobra.ExactArgs(1)

	return cmd
}

func runPostgresList(ctx *cmdctx.CmdContext) error {
	apps, err := ctx.Client.API().GetApps(context.Background(), api.StringPointer("postgres_cluster"))
	if err != nil {
		return err
	}

	if ctx.OutputJSON() {
		ctx.WriteJSON(apps)
		return nil
	}

	return ctx.Render(&presenters.Apps{Apps: apps})
}

func CreatePostgresClusterFromCommand(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()

	clusterOption := PostgresClusterOption{
		Organization: cmdCtx.Config.GetString("organization"),
		Region:       cmdCtx.Config.GetString("region"),
		ImageRef:     cmdCtx.Config.GetString("image-ref"),
		Password:     cmdCtx.Config.GetString("password"),
		SnapshotID:   cmdCtx.Config.GetString("snapshot-id"),
	}

	postgresConfig := PostgresConfiguration{
		Name:             cmdCtx.Config.GetString("name"),
		VmSize:           cmdCtx.Config.GetString("vm-size"),
		DiskGb:           cmdCtx.Config.GetInt("volume-size"),
		ClusteringOption: clusterOption,
	}

	err := createPostgresCluster(ctx, cmdCtx.Client.API(), postgresConfig)
	return err
}

func createPostgresCluster(ctx context.Context, client *api.Client, postgresConfig PostgresConfiguration) error {
	// Ask for an app name if it's not specified
	if postgresConfig.Name == "" {
		n, err := inputAppName("", false)
		if err != nil {
			return err
		}
		postgresConfig.Name = n
	}

	// Ask for an org name if not specified
	org, err := selectOrganization(ctx, client, postgresConfig.ClusteringOption.Organization, nil)
	if err != nil {
		return err
	}

	// Ask for a region if not specified
	var region *api.Region
	region, err = selectRegion(ctx, client, postgresConfig.ClusteringOption.Region)
	if err != nil {
		return err
	}

	// create the initial cluster creation mutation input
	input := &api.CreatePostgresClusterInput{
		OrganizationID: org.ID,
		Name:           postgresConfig.Name,
		Region:         api.StringPointer(region.Code),
	}

	if postgresConfig.DiskGb != 0 || postgresConfig.VmSize != "" {
		// createCustomCluster
	} else {
		input, err = createStockCluster(ctx, client, postgresConfig, input)
		if err != nil {
			return err
		}
	}

	fmt.Sprintf("Creating postgres cluster %s in organization %s\n", postgresConfig.Name, org.Slug)

	s := spinner.New(spinner.CharSets[11], 100*time.Millisecond)
	s.Writer = os.Stderr
	s.Prefix = "Launching..."
	s.Start()

	payload, err := client.CreatePostgresCluster(ctx, *input)
	if err != nil {
		return err
	}

	s.FinalMSG = fmt.Sprintf("Postgres cluster %s created\n", payload.App.Name)
	s.Stop()

	fmt.Printf("  Username:    %s\n", payload.Username)
	fmt.Printf("  Password:    %s\n", payload.Password)
	fmt.Printf("  Hostname:    %s.internal\n", payload.App.Name)
	fmt.Printf("  Proxy Port:  5432\n")
	fmt.Printf("  PG Port: 5433\n")

	fmt.Println(aurora.Italic("Save your credentials in a secure place, you won't be able to see them again!"))
	fmt.Println()

	cancelCtx := ctx.Command.Context()
	cmdCtx.AppName = payload.App.Name
	err = watchDeployment(cancelCtx, cmdCtx)

	if isCancelledError(err) {
		err = nil
	}

	if err == nil {
		fmt.Println()
		fmt.Println(aurora.Bold("Connect to postgres"))
		fmt.Printf("Any app within the %s organization can connect to postgres using the above credentials and the hostname \"%s.internal.\"\n", org.Slug, payload.App.Name)
		fmt.Printf("For example: postgres://%s:%s@%s.internal:%d\n", payload.Username, payload.Password, payload.App.Name, 5432)

		fmt.Println()
		fmt.Println("See the postgres docs for more information on next steps, managing postgres, connecting from outside fly:  https://fly.io/docs/reference/postgres/")
	}

	return err
}

// If no custom configuration settings have been passed on the command line, prompt user to
// select from a list of pre-defined configurations or opt into specifying a custom
// configuration.
func createStockCluster(ctx context.Context, client *api.Client, config PostgresConfiguration, input *api.CreatePostgresClusterInput) (*api.CreatePostgresClusterInput, error) {

	selectedCfg := 0
	options := []string{}
	for _, cfg := range postgresConfigurations() {
		options = append(options, cfg.Description)
	}

	prompt := &survey.Select{
		Message:  "Select configuration:",
		Options:  options,
		PageSize: len(postgresConfigurations()),
	}

	if err := survey.AskOne(prompt, &selectedCfg); err != nil {
		return nil, err
	}

	var pgConfig *PostgresConfiguration

	pgConfig = &postgresConfigurations()[selectedCfg]

	// Resolve configuration from pre-defined configuration.
	vmSize, err := selectVMSize(ctx, client, pgConfig.VmSize)
	if err != nil {
		return nil, err
	}

	input.VMSize = api.StringPointer(vmSize.Name)
	input.VolumeSizeGB = api.IntPointer(pgConfig.DiskGb)
	input.Count = api.IntPointer(pgConfig.ClusteringOption.Count)

	input.ImageRef = &pgConfig.ClusteringOption.ImageRef

	if config.ClusteringOption.ImageRef != "" {
		input.ImageRef = api.StringPointer(config.ClusteringOption.ImageRef)
	}

	// If someone chose to make a custom config from the list...
	if pgConfig.VmSize == "" {
		// createCustomCluster
	}

	return &input, nil

}

func createCustomCluster(ctx context.Context, client *api.Client, config PostgresConfiguration, input api.CreatePostgresClusterInput) (*api.CreatePostgresClusterInput, error) {

	selected := 0

	options := []string{}
	for _, opt := range postgresClusteringOptions() {
		options = append(options, opt.Name)
	}
	prompt := &survey.Select{
		Message:  "Select configuration:",
		Options:  options,
		PageSize: 2,
	}
	if err := survey.AskOne(prompt, &selected); err != nil {
		return nil, err
	}
	option := postgresClusteringOptions()[selected]

	input.Count = &option.Count
	input.ImageRef = &option.ImageRef

	// Resolve VM size
	vmSize, err := selectVMSize(ctx, client, config.VmSize)
	if err != nil {
		return nil, err
	}
	input.VMSize = api.StringPointer(vmSize.Name)

	// Resolve volume size
	if config.DiskGb == 0 {
		config.DiskGb, err = volumeSizeInput(10)
		if err != nil {
			return nil, err
		}
	}
	input.VolumeSizeGB = api.IntPointer(config.DiskGb)

	return &input, nil
}

func runAttachPostgresCluster(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()

	postgresAppName := cmdCtx.Config.GetString("postgres-app")
	appName := cmdCtx.AppName

	input := api.AttachPostgresClusterInput{
		AppID:                appName,
		PostgresClusterAppID: postgresAppName,
	}

	if dbName := cmdCtx.Config.GetString("database-name"); dbName != "" {
		input.DatabaseName = api.StringPointer(dbName)
	}
	if varName := cmdCtx.Config.GetString("variable-name"); varName != "" {
		input.VariableName = api.StringPointer(varName)
	}

	s := spinner.New(spinner.CharSets[11], 100*time.Millisecond)
	s.Writer = os.Stderr
	s.Prefix = "Attaching..."
	s.Start()

	payload, err := cmdCtx.Client.API().AttachPostgresCluster(ctx, input)

	if err != nil {
		return err
	}
	s.Stop()

	fmt.Printf("Postgres cluster %s is now attached to %s\n", payload.PostgresClusterApp.Name, payload.App.Name)
	fmt.Printf("The following secret was added to %s:\n  %s=%s\n", payload.App.Name, payload.EnvironmentVariableName, payload.ConnectionString)

	return nil
}

func runDetachPostgresCluster(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()

	postgresAppName := cmdCtx.Config.GetString("postgres-app")
	appName := cmdCtx.AppName

	s := spinner.New(spinner.CharSets[11], 100*time.Millisecond)
	s.Writer = os.Stderr
	s.Prefix = "Detaching..."
	s.Start()

	err := cmdCtx.Client.API().DetachPostgresCluster(ctx, postgresAppName, appName)

	if err != nil {
		return err
	}

	s.FinalMSG = fmt.Sprintf("Postgres cluster %s is now detached from %s\n", postgresAppName, appName)
	s.Stop()

	return nil
}

func runListPostgresDatabases(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()

	databases, err := cmdCtx.Client.API().ListPostgresDatabases(ctx, cmdCtx.AppName)
	if err != nil {
		return err
	}

	if cmdCtx.OutputJSON() {
		cmdCtx.WriteJSON(databases)
		return nil
	}

	table := helpers.MakeSimpleTable(cmdCtx.Out, []string{"Name", "Users"})

	for _, database := range databases {
		table.Append([]string{database.Name, strings.Join(database.Users, ",")})
	}

	table.Render()

	return nil
}

func runListPostgresUsers(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()

	users, err := cmdCtx.Client.API().ListPostgresUsers(ctx, cmdCtx.AppName)
	if err != nil {
		return err
	}

	if cmdCtx.OutputJSON() {
		cmdCtx.WriteJSON(users)
		return nil
	}

	table := helpers.MakeSimpleTable(cmdCtx.Out, []string{"Username", "Superuser", "Databases"})

	for _, user := range users {
		table.Append([]string{user.Username, strconv.FormatBool(user.IsSuperuser), strings.Join(user.Databases, ",")})
	}

	table.Render()

	return nil
}
