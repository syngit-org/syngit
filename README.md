# syngit

syngit is a Kubernetes operator that allows you to push resources on a git repository. It leverage the gitops by unifying the source of truth between your cluster and your git repository.

## Description

Sounds cool, isn't it? **But what is the difference with the other Gitops CD tools such as Flux or ArgoCD?**

The main approach of these tools is to pull changes from the remote git repository to apply them on the cluster. syngit does the opposite : it pushes the changes that you made on the cluster to the remote git repository.

**Why do I need syngit?**

There is plenty of reasons to use this operator. It can be borring to make every modification only through the git repository. Applying manifests will return an instant result of the cluster state.

Basically, if you like to use Kubernetes with cli or through an UI BUT you want to work in GitOps, then syngit is the operator that you need.

**I use an automatic reconciliation with my CD tool, do I really need to use syngit?**

By concept, both are not compatible. In fact, the automatic reconciliation will not have any effect because the changes made on the cluster are pushed on the remote git repository.

## Getting Started

### Prerequisites
- docker version 17.03+.
- kubectl version v1.11.3+.
- Access to a Kubernetes v1.11.3+ cluster.

### Installation

For now, you can only install syngit using Helm.

1. Add this github repository to your helm repos.
```sh
helm repo add syngit https://github.com/damsien/syngit.git
```

2. Install the operator
You can customize the values before installing the Helm chart. The template can be found under `chart/x.x.x/values.yaml` and add the `-f values.yaml` flag.
```sh
helm install syngit syngit/syngit --version 0.0.1
```

syngit is now installed on your cluster!

## Use syngit

There is 3 custom objects that are necessary to create in order to use syngit.

### RemoteUser

The RemoteUser object make the connexion to the remote git server using an user account. In order to use this object, you first need to create a secret that reference a Personal Access Token (and not an Access Token).

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: git-server-my_git_username-auth
  namespace: test
type: kubernetes.io/basic-auth
stringData:
  username: <MY_GIT_USERNAME>
  password: <PERSONAL_ACCESS_TOKEN>
```

```yaml
apiVersion: syngit.damsien.fr/v1alpha1
kind: RemoteUser
metadata:
  name: remoteuser-sample
  namespace: test
spec:
  gitBaseDomainFQDN: "github.com"
  testAuthentication: true
  email: your@email.com
  secretRef:
    name: git-server-my_git_username-auth
```

Now, if you look at the status of the object, the user should be connected to the git server.

```sh
kubectl get -n test remoteuser remoteuser-sample -o=jsonpath='{.status.connexionStatus}'
```

### RemoteUserBinding

The RemoteUserBinding bind the Kubernetes user with the remote git user. This is used by syngit when the user apply changes on the cluster. Syngit will push on the git server with the associated git user.

To retrieve your own username, you can run the following command :
```sh
kubectl auth whoami -o=jsonpath='{.status.userInfo.username}'
```

The name of the user is the id of the user. It can be different depending of your RBAC manager. It should be unique.

```yaml
apiVersion: syngit.damsien.fr/v1alpha1
kind: RemoteUserBinding
metadata:
  name: remoteuserbinding-sample
  namespace: test
spec:
  subject:
    kind: User
    name: kubernetes-user
  remoteRefs:
    - name: remoteuser-sample
```

### RemoteSyncer

The RemoteSyncer object contains the whole logic part of the operator.

In this example, the RemoteSyncer will intercept all the *configmaps*. It will push them to *https://github.com/my_repo_path.git* in the branch *main* under the path `my_configmaps/`. Because the `commitProcess` is set to `CommitApply`, the changes will be pushed and then applied to the cluster.

```yaml
apiVersion: syngit.damsien.fr/v1alpha1
kind: RemoteSyncer
metadata:
  name: remotesyncer-sample
  namespace: test
spec:
  remoteRepository: https://github.com/my_repo_path.git
  branch: main
  commitMode: Commit
  commitProcess: CommitApply
  operations:
    - CREATE
    - UPDATE
    - DELETE
  authorizedUsers:
    - name: remoteuserbinding-sample
  defaultUnauthorizedUserMode: Block
  excludedFields:
    - metadata.managedFields
    - metadata.creationTimestamp
    - metadata.annotations.[kubectl.kubernetes.io/last-applied-configuration]
    - metadata.uid
    - metadata.resourceVersion
  includedResources:
    - apiGroups: [""]
      apiVersions: ["v1"]
      resources: ["configmaps"]
      repotPath: "my_configmaps"
```

### Catch the resource

Now, let's apply this configmap :

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: test-configmap
  namespace: test
data:
  somedata: here
```

The configmap has been applied on the cluster and it has been pushed on the remote git repository as well!

## Contributing

TODO

## License

This operator has been built using the [kubebuilder]("https://book.kubebuilder.io/") framework.

### damsien license

Copyright 2024-present Damien Dassieu

Permission is hereby granted, free of charge, to any person obtaining a copy of this software and associated documentation files (the “Software”), to deal in the Software without restriction, including without limitation the rights to use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of the Software, and to permit persons to whom the Software is furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED “AS IS”, WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

### Kubebuilder license

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

