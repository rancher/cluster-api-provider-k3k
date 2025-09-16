
image_name = os.getenv('TEST_IMAGE', 'localhost:5005/controller:latest')

local_resource('Set up CAPI operator', cmd='clusterctl init')
docker_build(image_name, '.', build_args={'K3K_VERSION': '0.3.4'})

k8s_yaml(kustomize('config/default', images={'controller': image_name}))
