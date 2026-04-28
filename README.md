# kube-webapp-operator

A lightweight Kubernetes operator that introduces a `WebApp` Custom Resource to simplify deploying web-based containers with minimal boilerplate.

## Description

`kube-webapp-operator` allows you to define a web application using a simple custom resource and automatically provisions all required Kubernetes resources:

- `Deployment`
- `Service`
- `PersistentVolumeClaims` (optional)
- Traefik `IngressRoute` (optional)
- Database (PostgreSQL or MariaDB, optional)

Instead of writing repetitive YAML for each app, you define a single `WebApp` resource, and the operator handles the rest.

This is especially useful for homelabs, internal platforms, or lightweight PaaS-style workflows on clusters such as k3s with Traefik.

---

## Example

### Simple web app without database

```yaml
apiVersion: webapp.cyber.gent/v1
kind: WebApp
metadata:
  name: hello-world
  namespace: apps
spec:
  image: ghcr.io/cyberdotgent/helloworld:latest
  port: 3000
  proto: http
  ingress: hello.example.com

  env:
    - name: LOG_LEVEL
      value: info

  volumes:
    - mountPath: /app/data
      size: 10Gi
```

### WordPress with MariaDB

```yaml
apiVersion: webapp.cyber.gent/v1
kind: WebApp
metadata:
  name: wordpress
  namespace: apps
spec:
  image: wordpress:latest
  port: 80
  proto: http
  ingress: wordpress.example.com

  database:
    type: mariadb
    volumeSize: 20Gi
    dbUserVar: WORDPRESS_DB_USER
    dbPassVar: WORDPRESS_DB_PASSWORD
    dbHostVar: WORDPRESS_DB_HOST
    dbNameVar: WORDPRESS_DB_NAME

  env:
    - name: WORDPRESS_TABLE_PREFIX
      value: "wp_"

  volumes:
    - mountPath: /var/www/html
      size: 20Gi
```

This automatically creates:

- A `Deployment` running the container
- A `Service` exposing it internally
- `PersistentVolumeClaims` for each volume (including database storage)
- A Traefik `IngressRoute` for external access
- A managed database (PostgreSQL or MariaDB) with auto-generated credentials
- Environment variables injected with database connection details

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
- **Managed databases** (optional):
  - PostgreSQL (default version: 18.6.2)
  - MariaDB (default version: 25.0.10)
  - Auto-generated credentials stored in Secret
  - Customizable environment variable names
  - Persistent storage with configurable size
- Traefik integration via `IngressRoute`
- Supports `http` and `https` backends

---

## Database Configuration

The `database` field in `spec` is optional and enables automatic provisioning of PostgreSQL or MariaDB:

```yaml
database:
  type: postgres              # or 'mariadb'
  volumeSize: 10Gi            # default: 10Gi
  chartVersion: "18.6.2"      # optional, uses defaults per engine
  dbUserVar: DB_USER          # env var for username (default: DB_USER)
  dbPassVar: DB_PASS          # env var for password (default: DB_PASS)
  dbHostVar: DB_HOST          # env var for hostname (default: DB_HOST)
  dbNameVar: DB_NAME          # env var for database name (default: DB_NAME)
  jdbcVar: DB_JDBC            # env var for JDBC URL (default: DB_JDBC)
```

### How it works

- **Auto-generated credentials**: A Secret is created with random password on first deploy (immutable on updates)
- **Deterministic naming**: Database host is derived from app name (e.g., `myapp-db-postgresql.namespace.svc.cluster.local`)
- **Configurable env vars**: Specify environment variable names to match your app's expectations
- **Bitnami charts**: Uses official Bitnami Helm charts from `charts.bitnami.com/bitnami`
- **Persistent storage**: Database volume is created with specified size (default 10Gi)

### Example: PostgreSQL with custom env var names

```yaml
database:
  type: postgres
  volumeSize: 20Gi
  dbUserVar: PGUSER
  dbPassVar: PGPASSWORD
  dbHostVar: PGHOST
  dbNameVar: PGDATABASE
```

### Updating chart versions

Default versions are defined in `internal/controller/defaults.go`:

```go
const (
    DefaultPostgresChartVersion = "18.6.2"
    DefaultMariaDBChartVersion  = "25.0.10"
)
```

To use a different version, either override per-app or update the constants and rebuild the operator.

---

## Getting Started

### Prerequisites

- Go 1.24+
- Docker
- kubectl
- Kubernetes cluster (k3s recommended)
- Traefik installed with CRDs (`IngressRoute`)
- For database support: k3s with built-in Helm Chart CRD (`helm.cattle.io/v1`)

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
- Deleting the `WebApp` cleans up everything automatically (including database and credentials)
- Volume names are deterministic based on app name + mount path
- Secret keys are assumed to match the environment variable name
- Ingress is optional; if omitted, no external exposure is created
- Database credentials are generated once on creation and never overwritten on reconciliation
- Database host and JDBC URLs are computed deterministically from app name and namespace
- Database environment variable names are customizable to match app requirements

---

## Contributing

Contributions are welcome. Areas for improvement include:

- Better validation (CRD schema + webhooks)
- TLS support (Traefik certificates)
- Autoscaling (HPA support)
- Advanced ingress options (middlewares, headers, etc.)
- Configurable storage classes
- Database backup/restore utilities
- Multi-database instances per WebApp

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
