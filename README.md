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

### Llama.cpp Model

```yaml
jitdi -c ./test/llama-cpp.yaml
```

```bash
docker run --rm -it host.docker.internal:8888/llama-cpp/llama-2:full-7b-chat-Q2_K-gguf --run -m /models/7b/llama-2-7b-chat.Q2_K.gguf -p "Building a website can be done in 10 simple steps:" -n 512
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
