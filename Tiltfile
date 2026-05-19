
image_name = os.getenv('IMG', 'localhost:5000/rancher/cluster-api-provider-k3k:dev')

local_resource('Set up CAPI operator', cmd='clusterctl init')
docker_build(image_name, '.', build_args={'K3K_VERSION': '1.1.0'})

k8s_yaml(kustomize('config/default'))

