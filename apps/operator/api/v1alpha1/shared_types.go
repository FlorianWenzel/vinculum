package v1alpha1

// ArtifactSink describes where Task outputs are persisted after the run.
type ArtifactSink struct {
	Type      string       `json:"type,omitempty"`
	SourceDir string       `json:"sourceDir,omitempty"`
	S3        *S3Sink      `json:"s3,omitempty"`
	PVC       *PVCSink     `json:"pvc,omitempty"`
	Webhook   *WebhookSink `json:"webhook,omitempty"`
}

type S3Sink struct {
	Bucket    string     `json:"bucket"`
	Prefix    string     `json:"prefix,omitempty"`
	Endpoint  string     `json:"endpoint,omitempty"`
	Region    string     `json:"region,omitempty"`
	SecretRef *SecretRef `json:"secretRef,omitempty"`
}

type PVCSink struct {
	ClaimName string `json:"claimName"`
	SubPath   string `json:"subPath,omitempty"`
}

type WebhookSink struct {
	URL       string     `json:"url"`
	SecretRef *SecretRef `json:"secretRef,omitempty"`
}

func (in *ArtifactSink) DeepCopyInto(out *ArtifactSink) {
	*out = *in
	if in.S3 != nil {
		cp := *in.S3
		if in.S3.SecretRef != nil {
			cp.SecretRef = &SecretRef{Name: in.S3.SecretRef.Name}
		}
		out.S3 = &cp
	}
	if in.PVC != nil {
		cp := *in.PVC
		out.PVC = &cp
	}
	if in.Webhook != nil {
		cp := *in.Webhook
		if in.Webhook.SecretRef != nil {
			cp.SecretRef = &SecretRef{Name: in.Webhook.SecretRef.Name}
		}
		out.Webhook = &cp
	}
}
