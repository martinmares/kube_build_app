# Build Kubernetes manifests easily

Simple abstraction over kubernetes manifests.

## Typical workflow

You want create deployment for your awesome **brand new product**

```bash
mkdir ~/my-brand-new-product-k8s
cd ~/my-brand-new-product-k8s
```

Inside `my-brand-new-product-k8s` directory

```bash
cd ~/my-brand-new-product-k8s

mkdir environments
mkdir deployments
```

Inside `environments` (you want create deployment for two separate `environments`, first => `test`, second => `production`)

```bash
cd  ~/my-brand-new-product-k8s/environments

mkdir -p test/apps
mkdir -p production/apps
mkdir -p test/assets
mkdir -p production/assets
```

Inside `deployments` (you want create deployment for two separate `targets`, first => `test`, second => `production`)

```bash
cd  ~/my-brand-new-product-k8s/deployments

mkdir -p test/deploy
mkdir -p production/deploy
```

You should see the following directory structure (inside `my-brand-new-product-k8s` directory)

```bash
cd ~/my-brand-new-product-k8s
tree -d

.
├── deployments
│   ├── production
│   │   └── deploy
│   └── test
│       └── deploy
└── environments
    ├── production
    │   ├── apps
    │   └── assets
    └── test
        ├── apps
        └── assets

12 directories
```

You will now create a sample file describing the `application` for `test` environment

Create file with name `brand-new-product.yml` (inside directory `~/my-brand-new-product-k8s/environments/test/apps`). With the following content

```yaml
name: brand-new-product
disable_shared_assets: true
replicas: 1
containers:
  - name: brand-new-product
    image: docker.io/library/nginx:stable
    env_vars:
      - name: ENV_VAR
        value: "some value ..."
      - name: ANOTHER_VAR
        value: "another value ..."
    assets:
      - file: assets/nginx.conf
        to: /etc/nginx/conf.d/default.conf
    ports:
      - name: http
        port: 8080
    resources:
      cpu:
        from: "100m"
        to: "250m"
      memory:
        from: "50Mi"
        to: "100Mi"
    health:
    health:
      http:
        path:
          live: /index.html
          ready: /index.html
        port: 8080
```

Create file with name `nginx.conf` (inside directory `~/my-brand-new-product-k8s/environments/test/assets`). With the following content

```nginx
daemon off;
worker_processes  2;

server {
    listen       8080;
    server_name  localhost;

    location / {
        root   /usr/share/nginx/html;
        index  index.html index.htm;
    }

    error_page   500 502 503 504  /50x.html;
    location = /50x.html {
        root   /usr/share/nginx/html;
    }
}
```

You should see the following directory structure (inside `~/my-brand-new-product-k8s` directory)

```bash
cd ~/my-brand-new-product-k8s
tree -f

.
├── ./deployments
│   ├── ./deployments/production
│   │   └── ./deployments/production/deploy
│   └── ./deployments/test
│       └── ./deployments/test/deploy
├── ./env.secured.json
├── ./env.unsecured.json
├── ./environments
│   ├── ./environments/production
│   │   ├── ./environments/production/apps
│   │   └── ./environments/production/assets
│   └── ./environments/test
│       ├── ./environments/test/apps
│       │   └── ./environments/test/apps/brand-new-product.yml
│       └── ./environments/test/assets
│           └── ./environments/test/assets/nginx.conf
└── ./shared.assets.yml

12 directories, 5 files
```

**You are now ready to create your first `deployment`, run!**

```bash
kube_build_app -e test -t deployments/test/deploy
```

```log
Build 0 shared asset/s
Application brand-new-product, with 1 container/s
 => container [1] brand-new-product has 1 asset/s
 => asset [1]: brand-new-product-asset-66535d3
 => container [1] brand-new-product has 1 port/s
 => service [1] brand-new-product, has 1 port/s
   => has external (ingress) brand-new-product
```

You should see the following directory/file structure (inside `~/my-brand-new-product-k8s/deployments/test/deploy` directory)

```bash
cd ~/my-brand-new-product-k8s/deployments/test/deploy
tree -f

.
├── ./assets
│   └── ./assets/brand-new-product-asset-66535d3.yml
├── ./deployments
│   └── ./deployments/brand-new-product-deployment.yml
└── ./services
    ├── ./services/brand-new-product-service.yml
    └── ./services/external
        └── ./services/external/brand-new-product-ingress.yml

4 directories, 4 files
```

```yaml
---
apiVersion: v1
data:
  nginx.conf: |
    daemon off;
    worker_processes  2;

    server {
        listen       8080;
        server_name  localhost;

        location / {
            root   /usr/share/nginx/html;
            index  index.html index.htm;
        }

        error_page   500 502 503 504  /50x.html;
        location = /50x.html {
            root   /usr/share/nginx/html;
        }
    }
kind: ConfigMap
metadata:
  name: brand-new-product-asset-66535d3
  namespace:
```

```yaml
---
apiVersion: v1
kind: Service
metadata:
  name: brand-new-product
  namespace:
spec:
  selector:
    app.kubernetes.io/name: brand-new-product
  ports:
    - name: http-80
      port: 80
      targetPort: 8080
```

```yaml
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: brand-new-product
  namespace:
spec:
  rules:
    - host: brand-new-product.my-domain-name.io
      http:
        paths:
          - path: "/"
            backend:
              service:
                name: brand-new-product
                port:
                  number: 80
            pathType: ImplementationSpecific
```

```yaml
---
apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
    app.kubernetes.io/name: brand-new-product
  name: brand-new-product
  namespace:
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: brand-new-product
  strategy:
    rollingUpdate:
      maxSurge: 25%
      maxUnavailable: 25%
    type: RollingUpdate
  template:
    metadata:
      labels:
        app.kubernetes.io/name: brand-new-product
    spec:
      containers:
        - name: brand-new-product
          image: docker.io/library/nginx:stable
          ports:
            - name: http
              containerPort: 8080
              protocol: TCP
          env:
            - name: ENV_VAR
              value: some value ...
            - name: ANOTHER_VAR
              value: another value ...
          imagePullPolicy: Always
          resources:
            requests:
              cpu: 100m
              memory: 50Mi
            limits:
              cpu: 250m
              memory: 100Mi
          volumeMounts:
            - mountPath: "/etc/nginx/conf.d/default.conf"
              name: brand-new-product-asset-66535d3
              readOnly: true
              subPath: nginx.conf
          livenessProbe:
            httpGet:
              path: "/index.html"
              port: 8080
            initialDelaySeconds:
            periodSeconds:
            timeoutSeconds:
            successThreshold:
            failureThreshold:
          readinessProbe:
            httpGet:
              path: "/index.html"
              port: 8080
            initialDelaySeconds:
            periodSeconds:
            timeoutSeconds:
            successThreshold:
            failureThreshold:
      imagePullSecrets: []
      volumes:
        - name: brand-new-product-asset-66535d3
          configMap:
            defaultMode: 420
            name: brand-new-product-asset-66535d3
```
