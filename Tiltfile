local_resource('Configure local', cmd='make configure-local TEST_IMAGE=localhost:5005/controller:latest')
local_resource('Set up CAPI operator', cmd='clusterctl init')
docker_build('localhost:5005/controller:latest', '.', build_args={'K3K_VERSION': '0.1.4-r1'})
k8s_yaml(kustomize('config/default'))
