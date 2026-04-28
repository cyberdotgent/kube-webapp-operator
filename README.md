# kube-webapp-operator

A lightweight Kubernetes operator that introduces a `WebApp` Custom Resource to simplify deploying web-based containers with minimal boilerplate.

## Description

`kube-webapp-operator` allows you to define a web application using a simple custom resource and automatically provisions all required Kubernetes resources:

- `Deployment`
- `Service`
- `PersistentVolumeClaims` (optional)
- Traefik `IngressRoute` (optional)

Instead of writing repetitive YAML for each app, you define a single `WebApp` resource, and the operator handles the rest.

This is especially useful for homelabs, internal platforms, or lightweight PaaS-style workflows on clusters such as k3s with Traefik.

---

## Example

```yaml
apiVersion: webapp.cyber.gent/v1
kind: WebApp
metadata:
  name: example-webapp
  namespace: apps
spec:
  image: ghcr.io/cyberdotgent/helloworld:latest
  port: 3000
  proto: http
  ingress: helloworld.example.com

  env:
    - name: ENV_VAR_NAME
      value: env_var_data
    - name: DB_PASS
      fromSecret: example-webapp-db-pass

  volumes:
    - mountPath: /app/data
    - mountPath: /app/logs
      size: 25Gi
```

This will automatically create:

- A `Deployment` running your container
- A `Service` exposing it internally
- `PersistentVolumeClaims` for each volume
- A Traefik `IngressRoute` for external access

---

## Features

- Minimal YAML for app deployment
- Automatic Service + Deployment wiring
- Optional environment variables:
  - Static values
  - Secrets (key = env var name)
- Automatic PVC creation:
  - Default size: `10Gi`
  - Auto-generated names if omitted
- Traefik integration via `IngressRoute`
- Supports `http` and `https` backends

---

## Getting Started

### Prerequisites

- Go 1.24+
- Docker
- kubectl
- Kubernetes cluster (k3s recommended)
- Traefik installed with CRDs (`IngressRoute`)

---

### Deploy the operator

#### 1. Build and push the image

```sh
make docker-build docker-push IMG=ghcr.io/cyberdotgent/kube-webapp-operator:latest
```

#### 2. Install CRDs

```sh
make install
```

#### 3. Deploy the operator

```sh
make deploy IMG=ghcr.io/cyberdotgent/kube-webapp-operator:latest
```

---

### Verify installation

```sh
kubectl get pods -n kube-webapp-operator-system
kubectl get crd webapps.webapp.cyber.gent
```

---

### Create a WebApp

```sh
kubectl apply -f example.yaml
```

Then check generated resources:

```sh
kubectl get deploy,svc,pvc -n apps
kubectl get ingressroute -n apps
```

---

## Uninstall

### Remove WebApps

```sh
kubectl delete -k config/samples/
```

### Remove CRDs

```sh
make uninstall
```

### Remove operator

```sh
make undeploy
```

---

## GitHub Container Registry

Images are automatically built and published via GitHub Actions to:

```text
ghcr.io/cyberdotgent/kube-webapp-operator
```

---

## Project Distribution

### Install via YAML bundle

```sh
make build-installer IMG=ghcr.io/cyberdotgent/kube-webapp-operator:latest
```

Then:

```sh
kubectl apply -f https://raw.githubusercontent.com/cyberdotgent/kube-webapp-operator/main/dist/install.yaml
```

---

## Design Notes

- Each `WebApp` owns all generated resources via owner references
- Deleting the `WebApp` cleans up everything automatically
- Volume names are deterministic based on app name + mount path
- Secret keys are assumed to match the environment variable name
- Ingress is optional; if omitted, no external exposure is created

---

## Contributing

Contributions are welcome. Areas for improvement include:

- Better validation (CRD schema + webhooks)
- TLS support (Traefik certificates)
- Autoscaling (HPA support)
- Advanced ingress options (middlewares, headers, etc.)
- Configurable storage classes

---

## License

Copyright 2026 CYBER.gent.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at:

http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
