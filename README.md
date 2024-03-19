# capi-k3k

A cluster-api provider for [k3k](https://github.com/rancher/k3k).

## Description

This project aims to allow the provisioning/management of a k3k cluster using CAPI.

## Getting Started

### Prerequisites
- go version v1.21.0+
- docker version 17.03+.
- kubectl version v1.11.3+.
- Access to a Kubernetes v1.11.3+ cluster.
- tilt version v0.33.11+. 
- helm version v3.12.0+.

### Install/Run Development branch

Follow the [upstream docs](https://cluster-api.sigs.k8s.io/developer/tilt) to setup the project to run locally. The basic steps are repeated here for clarity.

Install tilt. Deploy/configure a cluster. The upstream docs use KIND. If using k3d, do the following:

1. Create a local registry (necessary to avoid pushing to a remote repo frequently).
```bash
k3d registry create
```

2. Retrieve the image name and port using docker.
```bash
## Look for a "k3d-registry" container
docker ps 
```

3. Create a cluster using the registry. It's also recommended to use the `--image` parameter to deploy a recent k3s version.
```bash
k3d cluster create --registry-use $CONTAINER_NAME:$PORT --image rancher/k3s:v1.28.7-k3s1
```

4. Configure the project to use the local registry.
```bash
make configure-local TEST_IMAGE=$CONTAINER_NAME:PORT/controller:latest
```

5. Clone the upstream [capi](https://github.com/kubernetes-sigs/cluster-api) project. In your local copy of the project, paste the following into `tilt-settings.yaml`:
```yaml
provider_repos:
- ../cluster-api-provider-k3k
enable_providers:
- k3k
allowed_contexts:
- k3d-k3s-default 
```
Some notes:
- The above assumes that the upstream capi project is in the same folder as the cluster-api-provider-k3k project. If that isn't true, adjust the path in `provider_repos`.
- `allowed_contexts` controlls which contexts tilt is able to use to deploy resources. This needs to point to a context for an admin account on the cluster created in step 3. You may need to change this value depending on your current kubeconfig structure.

6. Install the upstream [k3k project](https://github.com/rancher/k3k?tab=readme-ov-file#usage) into the cluster created in step 3:

```
helm repo add k3k https://rancher.github.io/k3k && helm repo update
helm install my-k3k k3k/k3k --devel
```

7. In the local copy of the **upstream project** (meaning the project cloned in step 5), run tilt:
```
tilt up
```

Some notes on running the project:
- Most changes will cause tilt to automatically re-build/push the image. However, if you are changing a value included as part of the chart (such as RBAC, or CRD structure), this may require you to do the following:
  - Stop tilt.
  - In the project repo (for cluster-api-provider-k3k) regenerate the CRDs and charts (`make generate && make manifests`).
  - Start tilt.

## Project Distribution

TODO: This project is in an unreleased, alpha state. As of now, there are no releases, or a supported way to install the project other than the dev setup documented above.

## Contributing

All changes should have an issue attached to them, and should have tests (where appropriate).

## License

Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

