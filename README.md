# `JITDI` (Just in Time Distribution Image)





## Why JITDI ?

Taking below usage scenarios as example, to show you how JITDI can greatly improve operational efficiency, especially for AI/ML use cases:


1. Model serving scenario (assuming we use NFS or other shared storage to store models, datasets, etc.)
    * Before: Each model serving instance (container) mounts the PVC (using NFS's CSI method) to mount the model at the NFS path. When each container starts, the model weights need to be moved from NFS storage to the GPU, and this loading speed is limited by the NFS storage/network performance. So, assuming 6 hosts, with 10 containers each starting one by one, the data transfer from NFS to GPU will occur 6X10 times, leading to high bandwidth usage.
    * Now: Using JITDI to describe the model's location: When the first container of the above 6X10 containers starts, it requests to pull the image from JITDI. JITDI's server will pull the model from NFS and build the corresponding image, allowing the first container to start. Then when other containers start, if the image has already been pulled locally, it can reuse the local image cache. So the first advantage: the performance speed of model loading is from the local disk (locally pulled image) to the GPU, which is much faster than before from NFS to GPU; the second advantage: except for the first container of each node, other containers can reuse the local image cache for model loading (ImagePullPolicy=IfNotPresent), without the need for further network transmission.

2. ML development experiment scenario ( we may need to combine different Python versions, development frameworks, and models in pairs. below we simplify to Python and model combinations)
    * Before: For example, there are 4 Python versions: 3.12, 3.11, 3.10, 3.8, and 5 models: mistral, stable-diffusion, bloom, falcon, llama; then you would need to build 4X5=20 images in advance and maintain 20 Dockerfile configuration files.
    * Now: Just provide single JITDI configuration file, with elegant and flexible wildcard configurations, and you can cover those combinations all. Users can combine Python:3.12-bloom as needed to create the required images, which is very convenient.


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
