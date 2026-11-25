package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/tebibcloud/terraform-provider-tebibcloud/internal/client"
)

// -- tebibcloud_database (ADR 0046) --------------------------------------------------------------

type databaseResource struct{ base }

type databaseModel struct {
	projectMeta
	Region            types.String   `tfsdk:"region"`
	Zone              types.String   `tfsdk:"zone"`
	VCPUs             types.Int64    `tfsdk:"vcpus"`
	CPUMillicores     types.Int64    `tfsdk:"cpu_millicores"`
	DiskUsedBytes     types.Int64    `tfsdk:"disk_used_bytes"`
	MemoryMiB         types.Int64    `tfsdk:"memory_mib"`
	PowerState        types.String   `tfsdk:"power_state"`
	Version           types.Int64    `tfsdk:"version"`
	Plan              types.String   `tfsdk:"plan"`
	Mode              types.String   `tfsdk:"mode"`
	DiskGiB           types.Int64    `tfsdk:"disk_gib"`
	VPCID             types.String   `tfsdk:"vpc_id"`
	SubnetID          types.String   `tfsdk:"subnet_id"`
	BackupEnabled     types.Bool     `tfsdk:"backup_enabled"`
	BackupRetention   types.Int64    `tfsdk:"backup_retention_days"`
	MaintenanceWindow types.String   `tfsdk:"maintenance_window"`
	SkipFinalBackup   types.Bool     `tfsdk:"skip_final_backup"`
	PublicEnabled     types.Bool     `tfsdk:"public_enabled"`
	AllowList         types.Set      `tfsdk:"allow_list"`
	AllowAll          types.Bool     `tfsdk:"allow_all"`
	FallbackEnabled   types.Bool     `tfsdk:"fallback_enabled"`
	PublicHost        types.String   `tfsdk:"public_host"`
	FallbackPort      types.Int64    `tfsdk:"fallback_port"`
	PostgresVersion   types.String   `tfsdk:"postgres_version"`
	Address           types.String   `tfsdk:"address"`
	Host              types.String   `tfsdk:"host"`
	Port              types.Int64    `tfsdk:"port"`
	CACertificate     types.String   `tfsdk:"ca_certificate"`
	Password          types.String   `tfsdk:"password"`
	Timeouts          timeouts.Value `tfsdk:"timeouts"`
}

func newDatabaseResource() resource.Resource { return &databaseResource{} }

func (r *databaseResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_database"
}

func (r *databaseResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A managed Postgres database: its own cluster, reached from the VMs of its " +
			"VPC on a private address with TLS and a password. Backups are **off unless you turn them " +
			"on** (`backup_enabled`): without them there is no point-in-time restore. The owner's " +
			"password is only known when the database is created: it is stored in the Terraform state " +
			"(marked sensitive), so keep the state encrypted. An imported database has no password " +
			"(reset it with `tebib db reset-password`).",
		Attributes: mergeAttrs(metaAttrs("databases", "Database ID (`pgdb-...`)."), map[string]schema.Attribute{
			"version": schema.Int64Attribute{
				Optional: true, Computed: true, Default: int64default.StaticInt64(18),
				MarkdownDescription: "Postgres major version, `18` (default) or `17`. Changing it replaces the database.",
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.RequiresReplace()},
				Validators:          []validator.Int64{int64validator.OneOf(17, 18)},
			},
			"plan": schema.StringAttribute{
				Optional: true, Computed: true,
				MarkdownDescription: "Plan code from the `tebibcloud_database_plans` data source, e.g. " +
					"`b1.nano` or `b1.micro`. Default: the platform's default plan (`b1.micro`). The code is " +
					"checked against the plans the API offers at plan time. Changing it restarts the database.",
				Validators:    []validator.String{stringvalidator.LengthAtLeast(1)},
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"mode": schema.StringAttribute{
				Optional: true, Computed: true, Default: stringdefault.StaticString("single"),
				MarkdownDescription: "`single` (default) or `ha`: a standby on another node with automatic " +
					"failover, billed as a second instance.",
				Validators: []validator.String{stringvalidator.OneOf("single", "ha")},
			},
			"disk_gib": schema.Int64Attribute{
				Optional: true, Computed: true,
				MarkdownDescription: "Disk per instance in GiB, at least 5 (default: the plan's). It can grow, never shrink.",
				Validators:          []validator.Int64{int64validator.AtLeast(5)},
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"vpc_id": schema.StringAttribute{
				Optional: true, Computed: true,
				MarkdownDescription: "The VPC whose VMs connect (default: the project's default VPC). " +
					"Changing it replaces the database.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(), stringplanmodifier.UseStateForUnknown(),
				},
			},
			"subnet_id": schema.StringAttribute{
				Optional: true, Computed: true,
				MarkdownDescription: "The subnet where the private address's two proxy pods run (default: the " +
					"VPC's default subnet). Changing it replaces the database.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(), stringplanmodifier.UseStateForUnknown(),
				},
			},
			"backup_enabled": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(false),
				MarkdownDescription: "Turn backups (and point-in-time restore) on. Default `false`. Billed " +
					"for the backup storage while on.",
			},
			"backup_retention_days": schema.Int64Attribute{
				Optional: true, Computed: true, Default: int64default.StaticInt64(7),
				MarkdownDescription: "Days of backups to keep, 1-30 (default 7).",
				Validators:          []validator.Int64{int64validator.Between(1, 30)},
			},
			"maintenance_window": schema.StringAttribute{
				Optional: true, Computed: true,
				MarkdownDescription: "Weekly four-hour window for minor updates (UTC), e.g. `sun-02` (default).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"skip_final_backup": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(false),
				MarkdownDescription: "On destroy, don't take a final backup. Default `false`: a final " +
					"backup is taken and kept for 7 days (billed). With backups on it is always taken.",
			},
			"public_enabled": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(false),
				MarkdownDescription: "Open the database to the internet. Needs a non-empty `allow_list` " +
					"(enforced before TLS and password). Postgres 17+ clients connect to `public_host` " +
					"port 5432 with `sslnegotiation=direct`; others need `fallback_enabled`.",
			},
			"allow_list": schema.SetAttribute{
				Optional: true, ElementType: types.StringType,
				MarkdownDescription: "Addresses or CIDRs that may connect over the internet.",
			},
			"allow_all": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(false),
				MarkdownDescription: "Allow every address (`0.0.0.0/0`). Needs the `db:AllowAllSources` permission.",
			},
			"fallback_enabled": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(false),
				MarkdownDescription: "A fallback port for clients that can't use direct TLS (libpq before 17). " +
					"From a small fixed pool, limited per organization.",
			},
			"region":           regionAttr("the VPC's region"),
			"zone":             zoneAttr(),
			"vcpus":            volatileInt("Whole vCPUs of an instance (the plan's CPU rounded up)."),
			"cpu_millicores":   volatileInt("Exact CPU of an instance: 500 is half a vCPU."),
			"disk_used_bytes":  volatileInt("Bytes used on the database disk (null until measured)."),
			"memory_mib":       volatileInt("Memory of an instance in MiB."),
			"power_state":      volatileString("`running` or `stopped`."),
			"public_host":      volatileString("The public host name (empty while the endpoint is off)."),
			"fallback_port":    volatileInt("The fallback port (null while off)."),
			"postgres_version": volatileString("The Postgres version running, e.g. `18.6`."),
			"address":          computedString("The private address inside the VPC (port 5432)."),
			"host":             computedString("The platform host name: use it with `sslmode=verify-full`."),
			"port":             stableInt("The port, 5432."),
			"ca_certificate":   computedString("The database's CA certificate (PEM), for `sslrootcert`."),
			"password": schema.StringAttribute{
				Computed: true, Sensitive: true,
				MarkdownDescription: "The owner role's (`app`) password. Only set by the create; null after an import.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		}),
		Blocks: map[string]schema.Block{
			"timeouts": timeouts.Block(ctx, timeouts.Opts{Create: true, Update: true, Delete: true}),
		},
	}
}

func (r *databaseResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.configure(req.ProviderData, &resp.Diagnostics)
}

func (r *databaseResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importProjectScoped(ctx, req, resp)
}

// planCatalogueTimeout bounds the plan-time call for the plan catalogue (a variable for tests).
var planCatalogueTimeout = 10 * time.Second

// ModifyPlan checks a new or changed plan code against the plans the API offers, so a typo or a
// plan that is not on offer fails in `terraform plan` instead of at the apply. It stays quiet when
// the provider isn't configured yet or the list can't be read: the API checks again on create.
func (r *databaseResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() || r.data == nil {
		return // destroy, or the provider isn't configured
	}
	var plan databaseModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() || plan.Plan.IsNull() || plan.Plan.IsUnknown() || plan.Plan.ValueString() == "" {
		return
	}
	if !req.State.Raw.IsNull() {
		var state databaseModel
		resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
		if state.Plan.Equal(plan.Plan) {
			return // unchanged: a plan that is no longer offered keeps working
		}
	}
	project := r.data.project
	if !plan.ProjectID.IsNull() && !plan.ProjectID.IsUnknown() && plan.ProjectID.ValueString() != "" {
		project = plan.ProjectID.ValueString()
	}
	if project == "" {
		return
	}
	// A short, single try: the API checks the plan again on create, so a slow or unreachable
	// catalogue must not slow down or fail the plan.
	ctx, cancel := context.WithTimeout(ctx, planCatalogueTimeout)
	defer cancel()
	var plans []client.DatabasePlan
	err := r.client().Do(ctx, client.Request{Method: http.MethodGet,
		Path: client.ProjectPath(project, "database-plans"), NoRetry: true}, &plans)
	if err != nil || len(plans) == 0 {
		return
	}
	codes := make([]string, 0, len(plans))
	for _, p := range plans {
		if p.Code == plan.Plan.ValueString() {
			return
		}
		codes = append(codes, p.Code)
	}
	sort.Strings(codes)
	resp.Diagnostics.AddAttributeError(path.Root("plan"), "Unknown database plan",
		fmt.Sprintf("%q is not a database plan on offer. Choose one of: %s (see the tebibcloud_database_plans data source).",
			plan.Plan.ValueString(), strings.Join(codes, ", ")))
}

func databasePath(project, id string) string { return client.ProjectPath(project, "databases", id) }

func (m *databaseModel) fromAPI(d *client.Database) {
	m.projectMeta.fromAPI(d.Meta)
	m.Region = types.StringValue(d.Region)
	m.Zone = zoneValue(d.Zone)
	m.VCPUs = types.Int64Value(d.VCPUs)
	m.CPUMillicores = types.Int64Value(d.CPUMillicores)
	m.DiskUsedBytes = optInt(d.DiskUsedBytes)
	m.MemoryMiB = types.Int64Value(d.MemoryMiB)
	m.PowerState = types.StringValue(d.PowerState)
	m.Version = types.Int64Value(d.Version)
	m.Plan = types.StringValue(d.Plan)
	m.Mode = types.StringValue(d.Mode)
	m.DiskGiB = types.Int64Value(d.DiskGiB)
	m.VPCID = types.StringValue(d.VPCID)
	m.SubnetID = types.StringValue(d.SubnetID)
	m.BackupEnabled = types.BoolValue(d.Backup.Enabled)
	m.BackupRetention = types.Int64Value(d.Backup.RetentionDays)
	m.MaintenanceWindow = types.StringValue(d.MaintenanceWindow)
	m.PostgresVersion = optString(d.PostgresVersion)
	m.PublicEnabled = types.BoolValue(d.Public.Enabled)
	m.AllowAll = types.BoolValue(d.Public.AllowAll)
	m.FallbackEnabled = types.BoolValue(d.Public.FallbackEnable)
	m.PublicHost = optString(d.Public.Host)
	m.FallbackPort = optInt(d.Public.FallbackPort)
	if d.Public.Enabled || !m.AllowList.IsNull() {
		m.AllowList = stringSetValue(d.Public.AllowList)
	}
	m.Address = types.StringValue(d.Address)
	m.Host = types.StringValue(d.Connection.Host)
	m.Port = types.Int64Value(d.Connection.Port)
	m.CACertificate = types.StringValue(d.CACertificate)
	if m.Password.IsUnknown() {
		m.Password = types.StringNull()
	}
	if m.SkipFinalBackup.IsNull() || m.SkipFinalBackup.IsUnknown() {
		m.SkipFinalBackup = types.BoolValue(false) // after an import
	}
}

// settled waits for the database to leave its transitional state.
func (r *databaseResource) settled(ctx context.Context, project, id string) (*client.Database, error) {
	return client.Wait[client.Database](ctx, r.client(), databasePath(project, id), nil)
}

func (r *databaseResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan databaseModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	project := r.projectID(plan.ProjectID, &resp.Diagnostics)
	d, ds := plan.Timeouts.Create(ctx, 20*time.Minute)
	ctx, cancel := withTimeout(ctx, d, ds, &resp.Diagnostics)
	defer cancel()
	body := client.DatabaseIn{
		Name: plan.Name.ValueString(), Description: plan.Description.ValueString(),
		Region: strPtr(plan.Region), Zone: strPtr(plan.Zone),
		Tags:    tagsFrom(ctx, plan.Tags, &resp.Diagnostics),
		Version: plan.Version.ValueInt64(), Plan: strPtr(plan.Plan), Mode: plan.Mode.ValueString(),
		DiskGiB: int64Ptr(plan.DiskGiB), VPCID: strPtr(plan.VPCID), SubnetID: strPtr(plan.SubnetID),
		Backup:            client.DatabaseBackupIn{Enabled: plan.BackupEnabled.ValueBool(), RetentionDays: int64Ptr(plan.BackupRetention)},
		MaintenanceWindow: strPtr(plan.MaintenanceWindow),
	}
	if resp.Diagnostics.HasError() {
		return
	}
	exposure := r.exposure(ctx, plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	var created client.Database
	// Not retried: a replayed create answers without the password.
	if err := r.client().Post(ctx, client.ProjectPath(project, "databases"), body, &created); err != nil {
		apiError(&resp.Diagnostics, "Creating the database", err)
		return
	}
	plan.fromAPI(&created)
	plan.Password = optString(created.Password)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	settled, err := r.settled(ctx, project, created.ID)
	if settled != nil {
		plan.fromAPI(settled)
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	}
	if err != nil {
		waitError(&resp.Diagnostics, "the database", err)
		return
	}
	if exposure != nil {
		// The public endpoint is a separate call: its allow-list and fallback port are checked there.
		var updated client.Database
		if err := r.client().Put(ctx, databasePath(project, created.ID)+"/exposure", *exposure, &updated); err != nil {
			apiError(&resp.Diagnostics, "Opening the database to the internet", err)
			return
		}
		if settled, err := r.settled(ctx, project, created.ID); err == nil && settled != nil {
			plan.fromAPI(settled)
			resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
		}
	}
}

// exposure is the request for the public endpoint, or nil when the plan leaves it off.
func (r *databaseResource) exposure(ctx context.Context, m databaseModel, diags *diag.Diagnostics) *client.DatabaseExposureIn {
	if !m.PublicEnabled.ValueBool() {
		return nil
	}
	return &client.DatabaseExposureIn{
		Public: true, AllowList: stringsFromSet(ctx, m.AllowList, diags),
		AllowAll: m.AllowAll.ValueBool(), Fallback: m.FallbackEnabled.ValueBool(),
	}
}

func (r *databaseResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state databaseModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	project := r.projectID(state.ProjectID, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	var got client.Database
	err := r.client().Get(ctx, databasePath(project, state.ID.ValueString()), &got)
	// A deleted database whose final backup is kept is gone as far as Terraform is concerned.
	if client.IsNotFound(err) || (err == nil && (got.State == "deleted" || got.KeptBackupUntil != nil)) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		apiError(&resp.Diagnostics, "Reading the database", err)
		return
	}
	state.fromAPI(&got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *databaseResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state databaseModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	d, ds := plan.Timeouts.Update(ctx, 30*time.Minute)
	ctx, cancel := withTimeout(ctx, d, ds, &resp.Diagnostics)
	defer cancel()
	project := state.ProjectID.ValueString()
	id := state.ID.ValueString()
	var patch client.DatabasePatch
	changed := false
	if tp := tagsPatch(ctx, plan.projectMeta, state.projectMeta, &resp.Diagnostics); tp != nil {
		patch.Description, patch.Tags = tp.Description, tp.Tags
		changed = true
	}
	if !plan.Plan.Equal(state.Plan) {
		patch.Plan = ptr(plan.Plan.ValueString())
		changed = true
	}
	if !plan.Mode.Equal(state.Mode) {
		patch.Mode = ptr(plan.Mode.ValueString())
		changed = true
	}
	if !plan.DiskGiB.Equal(state.DiskGiB) && !plan.DiskGiB.IsUnknown() {
		patch.DiskGiB = ptr(plan.DiskGiB.ValueInt64())
		changed = true
	}
	if !plan.MaintenanceWindow.Equal(state.MaintenanceWindow) && !plan.MaintenanceWindow.IsUnknown() {
		patch.MaintenanceWindow = ptr(plan.MaintenanceWindow.ValueString())
		changed = true
	}
	if !plan.BackupEnabled.Equal(state.BackupEnabled) || !plan.BackupRetention.Equal(state.BackupRetention) {
		patch.Backup = &client.DatabaseBackupPatch{
			Enabled: ptr(plan.BackupEnabled.ValueBool()), RetentionDays: ptr(plan.BackupRetention.ValueInt64()),
		}
		changed = true
	}
	if resp.Diagnostics.HasError() {
		return
	}
	if changed {
		if err := r.client().Patch(ctx, databasePath(project, id), patch, nil); err != nil {
			apiError(&resp.Diagnostics, "Updating the database", err)
			return
		}
	}
	if !plan.PublicEnabled.Equal(state.PublicEnabled) || !plan.AllowList.Equal(state.AllowList) ||
		!plan.AllowAll.Equal(state.AllowAll) || !plan.FallbackEnabled.Equal(state.FallbackEnabled) {
		body := client.DatabaseExposureIn{
			Public: plan.PublicEnabled.ValueBool(), AllowList: stringsFromSet(ctx, plan.AllowList, &resp.Diagnostics),
			AllowAll: plan.AllowAll.ValueBool(), Fallback: plan.FallbackEnabled.ValueBool(),
		}
		if resp.Diagnostics.HasError() {
			return
		}
		if err := r.client().Put(ctx, databasePath(project, id)+"/exposure", body, nil); err != nil {
			apiError(&resp.Diagnostics, "Changing the database's public endpoint", err)
			return
		}
	}
	settled, err := r.settled(ctx, project, id)
	if err != nil {
		waitError(&resp.Diagnostics, "the database", err)
		return
	}
	skip, password := plan.SkipFinalBackup, state.Password
	plan.fromAPI(settled)
	plan.SkipFinalBackup, plan.Password = skip, password
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *databaseResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state databaseModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	d, ds := state.Timeouts.Delete(ctx, 60*time.Minute)
	ctx, cancel := withTimeout(ctx, d, ds, &resp.Diagnostics)
	defer cancel()
	c := r.client()
	p := databasePath(state.ProjectID.ValueString(), state.ID.ValueString())
	q := url.Values{"final_backup": {"true"}}
	if state.SkipFinalBackup.ValueBool() {
		q = url.Values{"final_backup": {"false"}}
	}
	err := c.Do(ctx, client.Request{Method: http.MethodDelete, Path: p, Query: q}, nil)
	if client.IsNotFound(err) {
		return
	}
	if err != nil {
		apiError(&resp.Diagnostics, "Deleting the database", err)
		return
	}
	// Done when the row is gone, or only a kept final backup is left (it lives 7 days).
	_, err = client.WaitFor[client.Database](ctx, c, p, func(db client.Database) bool {
		return db.KeptBackupUntil != nil
	})
	if err != nil && !client.IsNotFound(err) {
		apiError(&resp.Diagnostics, "Deleting the database", err)
	}
}
