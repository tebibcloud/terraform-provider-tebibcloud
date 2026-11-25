package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/tebibcloud/terraform-provider-tebibcloud/internal/client"
)

// -- tebibcloud_database (data source) -----------------------------------------------------------

type databaseDataSource struct{ base }

type databaseDataModel struct {
	ProjectID         types.String `tfsdk:"project_id"`
	ID                types.String `tfsdk:"id"`
	Name              types.String `tfsdk:"name"`
	TRN               types.String `tfsdk:"trn"`
	Description       types.String `tfsdk:"description"`
	Tags              types.Map    `tfsdk:"tags"`
	State             types.String `tfsdk:"state"`
	Region            types.String `tfsdk:"region"`
	Zone              types.String `tfsdk:"zone"`
	Version           types.Int64  `tfsdk:"version"`
	PostgresVersion   types.String `tfsdk:"postgres_version"`
	Plan              types.String `tfsdk:"plan"`
	Mode              types.String `tfsdk:"mode"`
	VCPUs             types.Int64  `tfsdk:"vcpus"`
	CPUMillicores     types.Int64  `tfsdk:"cpu_millicores"`
	MemoryMiB         types.Int64  `tfsdk:"memory_mib"`
	DiskGiB           types.Int64  `tfsdk:"disk_gib"`
	DiskUsedBytes     types.Int64  `tfsdk:"disk_used_bytes"`
	PowerState        types.String `tfsdk:"power_state"`
	VPCID             types.String `tfsdk:"vpc_id"`
	SubnetID          types.String `tfsdk:"subnet_id"`
	Address           types.String `tfsdk:"address"`
	Host              types.String `tfsdk:"host"`
	Port              types.Int64  `tfsdk:"port"`
	CACertificate     types.String `tfsdk:"ca_certificate"`
	BackupEnabled     types.Bool   `tfsdk:"backup_enabled"`
	BackupRetention   types.Int64  `tfsdk:"backup_retention_days"`
	MaintenanceWindow types.String `tfsdk:"maintenance_window"`
	PublicEnabled     types.Bool   `tfsdk:"public_enabled"`
	PublicHost        types.String `tfsdk:"public_host"`
	AllowList         types.Set    `tfsdk:"allow_list"`
	AllowAll          types.Bool   `tfsdk:"allow_all"`
	FallbackEnabled   types.Bool   `tfsdk:"fallback_enabled"`
	FallbackPort      types.Int64  `tfsdk:"fallback_port"`
}

func newDatabaseDataSource() datasource.DataSource { return &databaseDataSource{} }

func (d *databaseDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_database"
}

func (d *databaseDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	attrs := lookupAttrs("database")
	delete(attrs, "default") // a project has no default database
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up a managed Postgres database by ID or by name. The owner's password " +
			"is only known to the `tebibcloud_database` resource that created it, so it is not exposed here.",
		Attributes: mergeDS(attrs, map[string]schema.Attribute{
			"region":                dsString("Region code, e.g. `neo`."),
			"zone":                  dsString("Zone code in the region, empty if the platform didn't pick one."),
			"version":               schema.Int64Attribute{Computed: true, MarkdownDescription: "Postgres major version."},
			"postgres_version":      dsString("The Postgres version running, e.g. `18.6`."),
			"plan":                  dsString("Plan code, e.g. `b1.micro`."),
			"mode":                  dsString("`single` or `ha`."),
			"vcpus":                 schema.Int64Attribute{Computed: true, MarkdownDescription: "Whole vCPUs of an instance."},
			"cpu_millicores":        schema.Int64Attribute{Computed: true, MarkdownDescription: "Exact CPU of an instance: 500 is half a vCPU."},
			"memory_mib":            schema.Int64Attribute{Computed: true, MarkdownDescription: "Memory of an instance in MiB."},
			"disk_gib":              schema.Int64Attribute{Computed: true, MarkdownDescription: "Disk per instance in GiB."},
			"disk_used_bytes":       schema.Int64Attribute{Computed: true, MarkdownDescription: "Bytes used on the disk (null until measured)."},
			"power_state":           dsString("`running` or `stopped`."),
			"vpc_id":                dsString("The VPC whose VMs connect."),
			"subnet_id":             dsString("The subnet of the private address's proxy pods."),
			"address":               dsString("The private address inside the VPC (port 5432)."),
			"host":                  dsString("The platform host name: use it with `sslmode=verify-full`."),
			"port":                  schema.Int64Attribute{Computed: true, MarkdownDescription: "The port, 5432."},
			"ca_certificate":        dsString("The database's CA certificate (PEM), for `sslrootcert`."),
			"backup_enabled":        schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether backups (and point-in-time restore) are on."},
			"backup_retention_days": schema.Int64Attribute{Computed: true, MarkdownDescription: "Days of backups kept."},
			"maintenance_window":    dsString("Weekly four-hour window for minor updates (UTC), e.g. `sun-02`."),
			"public_enabled":        schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the database is open to the internet."},
			"public_host":           dsString("The public host name (empty while the endpoint is off)."),
			"allow_list":            schema.SetAttribute{ElementType: types.StringType, Computed: true, MarkdownDescription: "Addresses or CIDRs that may connect over the internet."},
			"allow_all":             schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether every address may connect."},
			"fallback_enabled":      schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether a fallback port is open for clients without direct TLS."},
			"fallback_port":         schema.Int64Attribute{Computed: true, MarkdownDescription: "The fallback port (null while off)."},
		}),
	}
}

func (d *databaseDataSource) ConfigValidators(context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("name")),
	}
}

func (d *databaseDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.configure(req.ProviderData, &resp.Diagnostics)
}

func (d *databaseDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var m databaseDataModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	project := d.projectID(m.ProjectID, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	db, err := lookup(ctx, d.client(), project, "databases", nil, m.ID.ValueString(), m.Name.ValueString(),
		false, func(client.Database) bool { return false }, "database")
	if err != nil {
		apiError(&resp.Diagnostics, "Looking up the database", err)
		return
	}
	if db.State == "deleted" || db.KeptBackupUntil != nil {
		resp.Diagnostics.AddError("Looking up the database", "The database was deleted (only its final backup is kept).")
		return
	}
	m.ProjectID = types.StringValue(db.ProjectID)
	m.ID = types.StringValue(db.ID)
	m.Name = types.StringValue(db.Name)
	m.TRN = types.StringValue(db.TRN)
	m.Description = types.StringValue(db.Description)
	m.Tags = tagsValue(db.Tags)
	m.State = types.StringValue(db.State)
	m.Region = types.StringValue(db.Region)
	m.Zone = zoneValue(db.Zone)
	m.Version = types.Int64Value(db.Version)
	m.PostgresVersion = optString(db.PostgresVersion)
	m.Plan = types.StringValue(db.Plan)
	m.Mode = types.StringValue(db.Mode)
	m.VCPUs = types.Int64Value(db.VCPUs)
	m.CPUMillicores = types.Int64Value(db.CPUMillicores)
	m.MemoryMiB = types.Int64Value(db.MemoryMiB)
	m.DiskGiB = types.Int64Value(db.DiskGiB)
	m.DiskUsedBytes = optInt(db.DiskUsedBytes)
	m.PowerState = types.StringValue(db.PowerState)
	m.VPCID = types.StringValue(db.VPCID)
	m.SubnetID = types.StringValue(db.SubnetID)
	m.Address = types.StringValue(db.Address)
	m.Host = types.StringValue(db.Connection.Host)
	m.Port = types.Int64Value(db.Connection.Port)
	m.CACertificate = types.StringValue(db.CACertificate)
	m.BackupEnabled = types.BoolValue(db.Backup.Enabled)
	m.BackupRetention = types.Int64Value(db.Backup.RetentionDays)
	m.MaintenanceWindow = types.StringValue(db.MaintenanceWindow)
	m.PublicEnabled = types.BoolValue(db.Public.Enabled)
	m.PublicHost = optString(db.Public.Host)
	m.AllowList = stringSetValue(db.Public.AllowList)
	m.AllowAll = types.BoolValue(db.Public.AllowAll)
	m.FallbackEnabled = types.BoolValue(db.Public.FallbackEnable)
	m.FallbackPort = optInt(db.Public.FallbackPort)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

// -- tebibcloud_database_plans -------------------------------------------------------------------

type databasePlansDataSource struct{ base }

type databasePlansDataModel struct {
	ProjectID                types.String `tfsdk:"project_id"`
	Codes                    types.List   `tfsdk:"codes"`
	Plans                    types.List   `tfsdk:"plans"`
	Versions                 types.List   `tfsdk:"versions"`
	DefaultVersion           types.Int64  `tfsdk:"default_version"`
	MaintenanceWindows       types.List   `tfsdk:"maintenance_windows"`
	DefaultMaintenanceWindow types.String `tfsdk:"default_maintenance_window"`
	HAEnabled                types.Bool   `tfsdk:"ha_enabled"`
}

var databasePlanAttrTypes = map[string]attr.Type{
	"code": types.StringType, "name": types.StringType, "vcpus": types.Int64Type,
	"cpu_millicores": types.Int64Type, "memory_mib": types.Int64Type,
	"default_disk_gib": types.Int64Type, "max_disk_gib": types.Int64Type,
	"max_connections": types.Int64Type, "schedulable": types.BoolType,
}

func newDatabasePlansDataSource() datasource.DataSource { return &databasePlansDataSource{} }

func (d *databasePlansDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_database_plans"
}

func (d *databasePlansDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The managed Postgres plans on offer, and the versions, maintenance windows and " +
			"whether high availability is on. Use a plan `code` for `tebibcloud_database.plan`.",
		Attributes: map[string]schema.Attribute{
			"project_id": dsProjectID(),
			"codes":      schema.ListAttribute{ElementType: types.StringType, Computed: true, MarkdownDescription: "Plan codes, for `tebibcloud_database.plan`."},
			"plans": schema.ListNestedAttribute{
				Computed: true, MarkdownDescription: "The plans.",
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"code":             dsString("Plan code, e.g. `b1.micro`."),
					"name":             dsString("Meter name, e.g. `db.b1.micro` (see `tebibcloud_prices`)."),
					"vcpus":            schema.Int64Attribute{Computed: true, MarkdownDescription: "Whole vCPUs, rounded up."},
					"cpu_millicores":   schema.Int64Attribute{Computed: true, MarkdownDescription: "Exact CPU: 500 is half a vCPU."},
					"memory_mib":       schema.Int64Attribute{Computed: true, MarkdownDescription: "Memory in MiB."},
					"default_disk_gib": schema.Int64Attribute{Computed: true, MarkdownDescription: "Disk a new database gets."},
					"max_disk_gib":     schema.Int64Attribute{Computed: true, MarkdownDescription: "Largest disk."},
					"max_connections":  schema.Int64Attribute{Computed: true, MarkdownDescription: "Connections the database accepts."},
					"schedulable":      schema.BoolAttribute{Computed: true, MarkdownDescription: "A database of this plan can be created right now (advisory: creating decides)."},
				}},
			},
			"versions":                   schema.ListAttribute{ElementType: types.Int64Type, Computed: true, MarkdownDescription: "Postgres major versions on offer."},
			"default_version":            schema.Int64Attribute{Computed: true, MarkdownDescription: "The default Postgres major version."},
			"maintenance_windows":        schema.ListAttribute{ElementType: types.StringType, Computed: true, MarkdownDescription: "Maintenance windows to choose from, e.g. `sun-02`."},
			"default_maintenance_window": dsString("The default maintenance window."),
			"ha_enabled":                 schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether `mode = \"ha\"` can be used for new databases."},
		},
	}
}

func (d *databasePlansDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.configure(req.ProviderData, &resp.Diagnostics)
}

func (d *databasePlansDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var m databasePlansDataModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	project := d.projectID(m.ProjectID, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	plans, err := client.GetList[client.DatabasePlan](ctx, d.client(), client.ProjectPath(project, "database-plans"), nil)
	if err != nil {
		apiError(&resp.Diagnostics, "Listing database plans", err)
		return
	}
	var vers client.DatabaseVersions
	if err := d.client().Get(ctx, client.ProjectPath(project, "database-versions"), &vers); err != nil {
		apiError(&resp.Diagnostics, "Reading the database versions", err)
		return
	}
	codes := make([]string, 0, len(plans))
	objs := make([]attr.Value, 0, len(plans))
	for _, p := range plans {
		codes = append(codes, p.Code)
		objs = append(objs, types.ObjectValueMust(databasePlanAttrTypes, map[string]attr.Value{
			"code": types.StringValue(p.Code), "name": types.StringValue(p.Name),
			"vcpus": types.Int64Value(p.VCPUs), "cpu_millicores": types.Int64Value(p.CPUMillicores),
			"memory_mib": types.Int64Value(p.MemoryMiB), "default_disk_gib": types.Int64Value(p.DefaultDiskGiB),
			"max_disk_gib": types.Int64Value(p.MaxDiskGiB), "max_connections": types.Int64Value(p.MaxConnections),
			"schedulable": types.BoolValue(p.Schedulable),
		}))
	}
	versions := make([]attr.Value, 0, len(vers.Versions))
	for _, v := range vers.Versions {
		versions = append(versions, types.Int64Value(v))
	}
	m.ProjectID = types.StringValue(project)
	m.Codes = stringListValue(codes)
	m.Plans = types.ListValueMust(types.ObjectType{AttrTypes: databasePlanAttrTypes}, objs)
	m.Versions = types.ListValueMust(types.Int64Type, versions)
	m.DefaultVersion = types.Int64Value(vers.Default)
	m.MaintenanceWindows = stringListValue(vers.MaintenanceWindows)
	m.DefaultMaintenanceWindow = types.StringValue(vers.DefaultMaintenanceWindow)
	m.HAEnabled = types.BoolValue(vers.HAEnabled)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}
