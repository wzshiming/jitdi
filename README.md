# `JITDI` (Just in Time Distribution Image)

## Examples

### Embedded

Below is an example of how to embed external content into the image with docker

#### Binary

```yaml
jitdi -c ./test/file.yaml
```

```bash
docker run -it --rm host.docker.internal:8888/k8s/alpine/kubectl:v1.29.3 ls -lh /usr/local/bin/
```

#### Ollama Model

```yaml
jitdi -c ./test/ollama.yaml
```

```bash
docker run -it --rm host.docker.internal:8888/ollama/llama2:7b
```

### Allow insecure registries

#### Dockerd

`/etc/docker/daemon.json`

```yaml
...
  "insecure-registries": [
    "host.docker.internal:8888",
  ],
...
```

#### Containerd cri

`/etc/containerd/config.toml`

```yaml
[plugins."io.containerd.grpc.v1.cri".registry.mirrors."localhost:30888"]
   endpoint = ["http://localhost:30888"]
```
