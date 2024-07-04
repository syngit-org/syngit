# syngit roadmap

All the points are listed in the priority order (top is the most important).

## 2024 Q3

- [x] Automatic RemoteUserBinding creation on RemoteUser creation
- [ ] Centralize default excluded fields in a ConfigMap (like the git server configuration)
- [ ] Choose to force automatic RemoteUserBinding creation on RemoteUser creation in the helm values
- [ ] Centralized config to configure the repoPath with a fine-grained object scope
- [ ] Remove authorizedUsers field from the RemoteSyncer : search for all RemoteUserBinding in the same namespace

## 2024 Q4

- [ ] Choose the git push error default behavior
- [ ] Path finder : if an object already exists in any form on the remote repo, then replace it
- [ ] Specify the git server credentials directly inside the RemoteUser

## 2025 Q1

- [ ] Init the repo for the syngit website to quickly build the RemoteSyncer object