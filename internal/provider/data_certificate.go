package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/tebibcloud/terraform-provider-tebibcloud/internal/client"
)

type certificateDataSource struct{ base }

type certificateDataModel struct {
	ProjectID       types.String `tfsdk:"project_id"`
	ID              types.String `tfsdk:"id"`
	Name            types.String `tfsdk:"name"`
	TRN             types.String `tfsdk:"trn"`
	Description     types.String `tfsdk:"description"`
	Tags            types.Map    `tfsdk:"tags"`
	State           types.String `tfsdk:"state"`
	DomainNames     types.List   `tfsdk:"domain_names"`
	Source          types.String `tfsdk:"source"`
	Validation      types.String `tfsdk:"validation"`
	Status          types.String `tfsdk:"status"`
	StatusCode      types.String `tfsdk:"status_code"`
	StatusReason    types.String `tfsdk:"status_reason"`
	NotBefore       types.String `tfsdk:"not_before"`
	NotAfter        types.String `tfsdk:"not_after"`
	Issuer          types.String `tfsdk:"issuer"`
	Fingerprint     types.String `tfsdk:"fingerprint"`
	TestCertificate types.Bool   `tfsdk:"test_certificate"`
}

func newCertificateDataSource() datasource.DataSource { return &certificateDataSource{} }

func (d *certificateDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_certificate"
}

func (d *certificateDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up a certificate by ID or by name. A private key is never returned.",
		Attributes: map[string]schema.Attribute{
			"project_id":  dsProjectID(),
			"id":          schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Certificate ID."},
			"name":        schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Name to look up."},
			"trn":         dsString("TRN."),
			"description": dsString("Description."),
			"tags":        dsTags(),
			"state":       dsString("Lifecycle state."),
			"domain_names": schema.ListAttribute{
				ElementType: types.StringType, Computed: true, MarkdownDescription: "The names it covers.",
			},
			"source":           dsString("`managed` or `imported`."),
			"validation":       dsString("Managed: `dns` or `http`."),
			"status":           dsString("`pending`, `issued`, `failed` or `revoked`."),
			"status_code":      dsString("Why it isn't issued, as a code."),
			"status_reason":    dsString("The reason in words."),
			"not_before":       dsString("Start of validity."),
			"not_after":        dsString("End of validity."),
			"issuer":           dsString("Issuing CA."),
			"fingerprint":      dsString("SHA-256 fingerprint of the leaf."),
			"test_certificate": schema.BoolAttribute{Computed: true, MarkdownDescription: "Issued by a CA browsers don't trust."},
		},
	}
}

func (d *certificateDataSource) ConfigValidators(context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("name")),
	}
}

func (d *certificateDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.configure(req.ProviderData, &resp.Diagnostics)
}

func (d *certificateDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var m certificateDataModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	project := d.projectID(m.ProjectID, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	c, err := lookup(ctx, d.client(), project, "certificates", nil, m.ID.ValueString(), m.Name.ValueString(),
		false, func(client.Certificate) bool { return false }, "certificate")
	if err != nil {
		apiError(&resp.Diagnostics, "Looking up the certificate", err)
		return
	}
	m.ProjectID = types.StringValue(c.ProjectID)
	m.ID = types.StringValue(c.ID)
	m.Name = types.StringValue(c.Name)
	m.TRN = types.StringValue(c.TRN)
	m.Description = types.StringValue(c.Description)
	m.Tags = tagsValue(c.Tags)
	m.State = types.StringValue(c.State)
	m.DomainNames = stringListValue(c.DomainNames)
	m.Source = types.StringValue(c.Source)
	m.Validation = types.StringValue(c.Validation)
	m.Status = types.StringValue(c.Status)
	m.StatusCode = types.StringValue(c.StatusCode)
	m.StatusReason = types.StringValue(c.StatusReason)
	m.NotBefore = optString(c.NotBefore)
	m.NotAfter = optString(c.NotAfter)
	m.Issuer = types.StringValue(c.Issuer)
	m.Fingerprint = types.StringValue(c.Fingerprint)
	m.TestCertificate = types.BoolValue(c.TestCertificate)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}
