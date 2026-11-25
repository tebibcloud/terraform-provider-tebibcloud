package provider

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/tebibcloud/terraform-provider-tebibcloud/internal/client"
)

// -- tebibcloud_certificate ----------------------------------------------------------------------

type certificateResource struct{ base }

type certificateModel struct {
	projectMeta
	DomainNames      types.List     `tfsdk:"domain_names"`
	Validation       types.String   `tfsdk:"validation"`
	Certificate      types.String   `tfsdk:"certificate"`
	CertificateChain types.String   `tfsdk:"certificate_chain"`
	PrivateKey       types.String   `tfsdk:"private_key"`
	Source           types.String   `tfsdk:"source"`
	Status           types.String   `tfsdk:"status"`
	StatusCode       types.String   `tfsdk:"status_code"`
	StatusReason     types.String   `tfsdk:"status_reason"`
	NotBefore        types.String   `tfsdk:"not_before"`
	NotAfter         types.String   `tfsdk:"not_after"`
	RenewAfter       types.String   `tfsdk:"renew_after"`
	Issuer           types.String   `tfsdk:"issuer"`
	Fingerprint      types.String   `tfsdk:"fingerprint"`
	TestCertificate  types.Bool     `tfsdk:"test_certificate"`
	Timeouts         timeouts.Value `tfsdk:"timeouts"`
}

func newCertificateResource() resource.Resource { return &certificateResource{} }

func (r *certificateResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_certificate"
}

func (r *certificateResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := metaAttrs("certificates", "Certificate ID (`cert-...`).")
	attrs["name"] = optionalComputedReplace("Name, unique among the project's certificates. Default: made " +
		"from the first domain. Changing it replaces the certificate.")
	resp.Schema = schema.Schema{
		MarkdownDescription: "A TLS certificate for HTTPS listeners on load balancers. **Managed**: give " +
			"`domain_names` and `validation`; TebibCloud orders a free certificate from Let's Encrypt and " +
			"renews it. **Imported**: give `certificate`, `certificate_chain` and `private_key`; the " +
			"key is sealed on this machine to the platform's public key before it is sent, so the API " +
			"can't read it. Caveat: `private_key` is stored in the Terraform state (marked sensitive) " +
			"and never read back; protect the state. Attach the certificate to an `https` " +
			"`tebibcloud_load_balancer_listener`. A certificate a listener uses can't be deleted.",
		Attributes: mergeAttrs(attrs, map[string]schema.Attribute{
			"domain_names": schema.ListAttribute{
				ElementType: types.StringType, Optional: true, Computed: true,
				MarkdownDescription: "Managed: up to 5 names (`*.example.com` needs `validation = \"dns\"`). " +
					"Imported: filled from the certificate. Changing it replaces the certificate.",
				Validators: []validator.List{listvalidator.SizeBetween(1, 5)},
				PlanModifiers: []planmodifier.List{
					listplanmodifier.RequiresReplaceIfConfigured(),
					listplanmodifier.UseStateForUnknown(),
				},
			},
			"validation": schema.StringAttribute{
				Optional: true, Computed: true,
				MarkdownDescription: "Managed: `dns` (names in a zone hosted with TebibCloud) or `http` " +
					"(names that point at one of the project's public load balancers, which needs an http " +
					"listener on port 80).",
				Validators: []validator.String{stringvalidator.OneOf("dns", "http")},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplaceIfConfigured(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"certificate": schema.StringAttribute{
				Optional: true, MarkdownDescription: "Imported: the leaf certificate (PEM).",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"certificate_chain": schema.StringAttribute{
				Optional: true, MarkdownDescription: "Imported: the intermediate certificates (PEM).",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"private_key": schema.StringAttribute{
				Optional: true, Sensitive: true,
				MarkdownDescription: "Imported: the private key (PEM, unencrypted, RSA 2048-4096 or ECDSA " +
					"P-256/P-384). Sealed before it is sent; kept in state, never read back.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"source":        computedString("`managed` or `imported`."),
			"status":        volatileString("`pending`, `issued`, `failed` or `revoked`."),
			"status_code":   volatileString("Why it isn't issued (for example `dns_not_pointing`, `caa_blocks`, `key_mismatch`)."),
			"status_reason": volatileString("The reason in words."),
			"not_before":    volatileString("Start of validity (RFC 3339)."),
			"not_after":     volatileString("End of validity (RFC 3339)."),
			"renew_after":   volatileString("Managed: when renewal starts (a third of the lifetime before the end)."),
			"issuer":        volatileString("Issuing CA."),
			"fingerprint":   volatileString("SHA-256 fingerprint of the leaf certificate."),
			"test_certificate": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "True when issued by a test CA that browsers don't trust (staging).",
			},
		}),
		Blocks: map[string]schema.Block{
			"timeouts": timeouts.Block(ctx, timeouts.Opts{Create: true, Update: true, Delete: true}),
		},
	}
}

func (r *certificateResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.configure(req.ProviderData, &resp.Diagnostics)
}

func (r *certificateResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importProjectScoped(ctx, req, resp)
}

func certPath(project, id string) string { return client.ProjectPath(project, "certificates", id) }

func (m *certificateModel) fromAPI(c *client.Certificate) {
	m.projectMeta.fromAPI(c.Meta)
	m.DomainNames = stringListValue(c.DomainNames)
	m.Validation = types.StringValue(c.Validation)
	m.Source = types.StringValue(c.Source)
	m.Status = types.StringValue(c.Status)
	m.StatusCode = types.StringValue(c.StatusCode)
	m.StatusReason = types.StringValue(c.StatusReason)
	m.NotBefore = optString(c.NotBefore)
	m.NotAfter = optString(c.NotAfter)
	m.RenewAfter = optString(c.RenewAfter)
	m.Issuer = types.StringValue(c.Issuer)
	m.Fingerprint = types.StringValue(c.Fingerprint)
	m.TestCertificate = types.BoolValue(c.TestCertificate)
}

func (r *certificateResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan certificateModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	project := r.projectID(plan.ProjectID, &resp.Diagnostics)
	d, ds := plan.Timeouts.Create(ctx, 15*time.Minute)
	ctx, cancel := withTimeout(ctx, d, ds, &resp.Diagnostics)
	defer cancel()
	body := client.CertificateIn{
		Name: strPtr(plan.Name), Description: plan.Description.ValueString(),
		Tags: tagsFrom(ctx, plan.Tags, &resp.Diagnostics),
	}
	if resp.Diagnostics.HasError() {
		return
	}
	c := r.client()
	imported := !plan.Certificate.IsNull()
	if imported {
		if plan.PrivateKey.IsNull() {
			resp.Diagnostics.AddAttributeError(path.Root("private_key"), "Missing private key",
				"An imported certificate needs `private_key` next to `certificate`.")
			return
		}
		var key client.ImportKey
		if err := c.Get(ctx, client.ProjectPath(project, "certificates", "import-key"), &key); err != nil {
			apiError(&resp.Diagnostics, "Reading the platform's import key", err)
			return
		}
		sealed, err := client.SealPrivateKey(key.PublicKey, []byte(plan.PrivateKey.ValueString()), project)
		if err != nil {
			resp.Diagnostics.AddError("Sealing the private key", err.Error())
			return
		}
		body.Certificate = plan.Certificate.ValueString()
		body.Chain = plan.CertificateChain.ValueString()
		body.SealedPrivateKey = sealed
	} else {
		if plan.DomainNames.IsNull() || plan.DomainNames.IsUnknown() || plan.Validation.IsNull() || plan.Validation.IsUnknown() {
			resp.Diagnostics.AddError("Missing arguments",
				"A managed certificate needs `domain_names` and `validation`; an imported one needs "+
					"`certificate`, `certificate_chain` and `private_key`.")
			return
		}
		resp.Diagnostics.Append(plan.DomainNames.ElementsAs(ctx, &body.DomainNames, false)...)
		body.Validation = plan.Validation.ValueString()
	}
	var cert client.Certificate
	if err := c.Create(ctx, client.ProjectPath(project, "certificates"), body, &cert); err != nil {
		apiError(&resp.Diagnostics, "Creating the certificate", err)
		return
	}
	plan.fromAPI(&cert)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	settled, err := client.WaitFor(ctx, c, certPath(project, cert.ID), client.CertificateSettled)
	if settled != nil {
		plan.fromAPI(settled)
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	}
	if err != nil {
		waitError(&resp.Diagnostics, "the certificate", err)
		return
	}
	if settled != nil && settled.Status != "issued" {
		resp.Diagnostics.AddError("The certificate wasn't issued",
			settled.StatusCode+": "+settled.StatusReason+" (fix the cause and run `tebib lb cert retry "+settled.ID+"`, then refresh)")
	}
}

func (r *certificateResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state certificateModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	project := r.projectID(state.ProjectID, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	var cert client.Certificate
	err := r.client().Get(ctx, certPath(project, state.ID.ValueString()), &cert)
	if client.IsNotFound(err) || (err == nil && cert.State == "deleted") {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		apiError(&resp.Diagnostics, "Reading the certificate", err)
		return
	}
	state.fromAPI(&cert)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *certificateResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state certificateModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	d, ds := plan.Timeouts.Update(ctx, 5*time.Minute)
	ctx, cancel := withTimeout(ctx, d, ds, &resp.Diagnostics)
	defer cancel()
	p := certPath(state.ProjectID.ValueString(), state.ID.ValueString())
	if patch := tagsPatch(ctx, plan.projectMeta, state.projectMeta, &resp.Diagnostics); patch != nil {
		if err := r.client().Patch(ctx, p, patch, nil); err != nil {
			apiError(&resp.Diagnostics, "Updating the certificate", err)
			return
		}
	}
	var cert client.Certificate
	if err := r.client().Get(ctx, p, &cert); err != nil {
		apiError(&resp.Diagnostics, "Reading the certificate", err)
		return
	}
	plan.fromAPI(&cert)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *certificateResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state certificateModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	d, ds := state.Timeouts.Delete(ctx, 10*time.Minute)
	ctx, cancel := withTimeout(ctx, d, ds, &resp.Diagnostics)
	defer cancel()
	if err := deleteAndWait[client.Certificate](ctx, r.client(), certPath(state.ProjectID.ValueString(), state.ID.ValueString())); err != nil {
		apiError(&resp.Diagnostics, "Deleting the certificate", err)
	}
}
