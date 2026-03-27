{{- define "vinculum.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "vinculum.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name (include "vinculum.name" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}

{{- define "vinculum.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" -}}
{{- end -}}

{{- define "vinculum.labels" -}}
helm.sh/chart: {{ include "vinculum.chart" . }}
app.kubernetes.io/name: {{ include "vinculum.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{- define "vinculum.selectorLabels" -}}
app.kubernetes.io/name: {{ include "vinculum.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "vinculum.infra.name" -}}
{{- printf "%s-vinculum-infra" .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "vinculum.ui.name" -}}
{{- printf "%s-hive-ui" .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "vinculum.keycloakBaseURL" -}}
{{- default (printf "http://%s-keycloak.%s.svc.cluster.local" .Release.Name .Release.Namespace) .Values.vinculumInfra.env.keycloakBaseURL -}}
{{- end -}}

{{- define "vinculum.keycloakIssuerURL" -}}
{{- default (printf "http://%s-keycloak.%s.svc.cluster.local/realms/%s" .Release.Name .Release.Namespace .Values.vinculumInfra.env.keycloakRealm) .Values.vinculumInfra.env.keycloakIssuerURL -}}
{{- end -}}

{{- define "vinculum.forgejoBaseURL" -}}
{{- default (printf "http://%s-forgejo-http.%s.svc.cluster.local:3000" .Release.Name .Release.Namespace) .Values.vinculumInfra.env.forgejoBaseURL -}}
{{- end -}}

{{- define "vinculum.forgejoPublicURL" -}}
{{- default (printf "http://%s-forgejo-http.%s.svc.cluster.local:3000" .Release.Name .Release.Namespace) .Values.vinculumInfra.env.forgejoPublicURL -}}
{{- end -}}

{{- define "vinculum.keycloakForgejoRedirectURIs" -}}
{{- default (printf "%s/user/oauth2/*" (include "vinculum.forgejoPublicURL" .)) .Values.vinculumInfra.env.keycloakForgejoRedirectURIs -}}
{{- end -}}

{{- define "vinculum.keycloakForgejoWebOrigins" -}}
{{- default (include "vinculum.forgejoPublicURL" .) .Values.vinculumInfra.env.keycloakForgejoWebOrigins -}}
{{- end -}}

{{- define "vinculum.forgejoPodNamespace" -}}
{{- default .Release.Namespace .Values.vinculumInfra.env.forgejoPodNamespace -}}
{{- end -}}

{{- define "vinculum.forgejoPodLabelSelector" -}}
{{- default (printf "app.kubernetes.io/instance=%s,app.kubernetes.io/name=forgejo" .Release.Name) .Values.vinculumInfra.env.forgejoPodLabelSelector -}}
{{- end -}}
