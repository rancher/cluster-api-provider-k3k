
image_name = os.getenv('IMG', 'localhost:5005/controller:latest')

local_resource('Set up CAPI operator', cmd='clusterctl init')
docker_build(image_name, '.', build_args={'K3K_VERSION': '1.0.2'})

k8s_yaml(kustomize('config/default'))

