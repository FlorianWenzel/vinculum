system_ns = 'vinculum-system'

allow_k8s_contexts('zora')
default_registry('ttl.sh')
k8s_kind('Drone', image_json_path='{.spec.image}')
k8s_kind('Task')
k8s_kind('Repository')
k8s_kind('DroneRepositoryAccess')

local_resource('namespaces', 'kubectl apply -f dev/namespaces.yaml')

docker_build(
    'ghcr.io/florianwenzel/vinculum-infra',
    '.',
    dockerfile='apps/vinculum-infra/Dockerfile',
    only=['apps/vinculum-infra', 'go.work'],
    live_update=[
        sync('apps/vinculum-infra', '/src/apps/vinculum-infra'),
        run('cd /src/apps/vinculum-infra && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /usr/local/bin/vinculum-infra ./cmd/vinculum-infra'),
    ],
)

local_resource(
    'agent-image',
    cmd='docker buildx build --platform linux/amd64 -t ttl.sh/vinculum-agent:12h -f apps/vinculum-agent/Dockerfile . --push',
    deps=['apps/vinculum-agent'],
)

docker_build(
    'ghcr.io/florianwenzel/vinculum-orchestrator',
    '.',
    dockerfile='apps/orchestrator/Dockerfile',
    only=['apps/orchestrator'],
)

k8s_yaml(helm('helm/infrastructure', name='vinculum-infra', namespace=system_ns, values=['helm/infrastructure/values-dev.yaml']))
k8s_resource('vinculum-infra-postgresql', resource_deps=['namespaces'])
k8s_resource('vinculum-infra-keycloak', resource_deps=['vinculum-infra-postgresql'], port_forwards=['8080:8080'])
k8s_resource('vinculum-infra-forgejo', resource_deps=['vinculum-infra-postgresql'], port_forwards=['3000:3000', '2222:22'])

k8s_yaml(helm('helm/vinculum', name='vinculum', namespace=system_ns, values=['helm/vinculum/values-dev.yaml'], set=['vinculumInfra.image.tag=dev']))
k8s_resource(
    'vinculum-vinculum-infra',
    resource_deps=['vinculum-infra-postgresql', 'vinculum-infra-keycloak', 'vinculum-infra-forgejo'],
)

k8s_yaml(helm('helm/orchestrator', name='orchestrator', namespace=system_ns, set=['orchestrator.image.tag=dev']))
k8s_resource('orchestrator-orchestrator', resource_deps=['vinculum-vinculum-infra'], port_forwards=['8084:8084'])

local_resource(
    'hive-ui-deps',
    cmd='cd apps/hive-ui && npm install',
    deps=['apps/hive-ui/package.json'],
)

local_resource(
    'hive-ui',
    serve_cmd='cd apps/hive-ui && npm run dev -- --host 0.0.0.0 --port 4173',
    deps=['apps/hive-ui/index.html', 'apps/hive-ui/vite.config.js', 'apps/hive-ui/src'],
    resource_deps=['hive-ui-deps', 'orchestrator-orchestrator'],
)
