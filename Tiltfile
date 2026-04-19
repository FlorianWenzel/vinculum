system_ns = 'vinculum-system'

allow_k8s_contexts(['rancher-desktop', 'nova', 'k3d-vinculum'])
k8s_kind('Agent', image_json_path='{.spec.image}')
k8s_kind('Task')
k8s_kind('AgentSchedule')

local('kubectl get namespace %s || kubectl create namespace %s' % (system_ns, system_ns), quiet=True)
k8s_namespace(system_ns)

# Build images.
local_resource(
    'agent-image',
    cmd='docker build -t vinculum-agent:tilt-dev -f apps/vinculum-agent/Dockerfile .',
    deps=['apps/vinculum-agent'],
)

docker_build(
    'ghcr.io/florianwenzel/vinculum-operator',
    '.',
    dockerfile='apps/operator/Dockerfile',
    only=['apps/operator'],
)

# Vinculum chart (CRDs + operator deployment + RBAC).
k8s_yaml(helm('helm/vinculum', name='vinculum', namespace=system_ns, set=[
    'operator.defaultAgentImage.repository=vinculum-agent',
    'operator.defaultAgentImage.tag=tilt-dev',
]))
k8s_resource('vinculum-operator', port_forwards=['8084:8084'])

# Provider secrets from .tilt-secrets/ (gitignored).
opencode_key = str(read_file('.tilt-secrets/opencode-api-key', default='')).strip()
if opencode_key:
    k8s_yaml(blob("""apiVersion: v1
kind: Secret
metadata:
  name: zen-provider-keys
  namespace: %s
type: Opaque
stringData:
  OPENCODE_API_KEY: %s
""" % (system_ns, opencode_key)))

azure_key = str(read_file('.tilt-secrets/azure-openai-api-key', default='')).strip()
azure_endpoint = str(read_file('.tilt-secrets/azure-openai-endpoint', default='')).strip()
if azure_key and azure_endpoint:
    k8s_yaml(blob("""apiVersion: v1
kind: Secret
metadata:
  name: azure-provider-keys
  namespace: %s
type: Opaque
stringData:
  AZURE_OPENAI_API_KEY: %s
  AZURE_OPENAI_API_ENDPOINT: %s
  AZURE_OPENAI_API_VERSION: "2024-10-21"
""" % (system_ns, azure_key, azure_endpoint)))

# E2E Agent + Task applied via kubectl once the operator is up (needs CRDs installed first).
local_resource(
    'e2e-manifests',
    cmd='kubectl apply -f .local/e2e.yaml',
    deps=['.local/e2e.yaml'],
    resource_deps=['vinculum-operator', 'agent-image'],
)
