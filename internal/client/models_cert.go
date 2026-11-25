package client

// HTTPS certificates (ADR 0051).

type CertificateIn struct {
	Name        *string           `json:"name,omitempty"`
	Description string            `json:"description"`
	Tags        map[string]string `json:"tags"`
	// Managed.
	DomainNames []string `json:"domain_names,omitempty"`
	Validation  string   `json:"validation,omitempty"`
	// Imported: the key is sealed to the platform's public key before it is sent.
	Certificate      string `json:"certificate,omitempty"`
	Chain            string `json:"chain,omitempty"`
	SealedPrivateKey string `json:"sealed_private_key,omitempty"`
}

type CertificateUse struct {
	LoadBalancerID string `json:"load_balancer_id"`
	ListenerID     string `json:"listener_id"`
	Port           int64  `json:"port"`
}

type Certificate struct {
	Meta
	DomainNames     []string         `json:"domain_names"`
	Source          string           `json:"source"`
	Validation      string           `json:"validation"`
	Status          string           `json:"status"`
	StatusCode      string           `json:"status_code"`
	StatusReason    string           `json:"status_reason"`
	NotBefore       *string          `json:"not_before"`
	NotAfter        *string          `json:"not_after"`
	RenewAfter      *string          `json:"renew_after"`
	Issuer          string           `json:"issuer"`
	Fingerprint     string           `json:"fingerprint"`
	Serial          string           `json:"serial"`
	TestCertificate bool             `json:"test_certificate"`
	UsedBy          []CertificateUse `json:"used_by"`
}

type ImportKey struct {
	Algorithm string `json:"algorithm"`
	PublicKey string `json:"public_key"`
}

// CertificateSettled: the certificate is done for now (issued, failed or revoked), whatever
// the lifecycle state says.
func CertificateSettled(c Certificate) bool {
	return c.Status == "issued" || c.Status == "failed" || c.Status == "revoked"
}
