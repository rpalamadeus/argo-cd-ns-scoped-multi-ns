#!/bin/bash
docker pull QQQQQQ/registry-k8s-io-docker-remote/kube-apiserver:v1.28.0
cat <<EOF > Dockerfile
# Use Ubuntu as the base image
FROM ubuntu:22.04

# Set environment variables
ENV KUBE_VERSION=v1.28.0

# Install dependencies
RUN apt-get update && apt-get install -y \
    ca-certificates \
    curl \
    iproute2 \
    && rm -rf /var/lib/apt/lists/*

# Copy kube-apiserver binary and required files from the official image
FROM QQQQQQ/registry-k8s-io-docker-remote/kube-apiserver:v1.28.0 AS kube_apiserver

# Use the Ubuntu image as the final build stage
FROM ubuntu:22.04

# Create necessary directories
RUN mkdir -p \
    /usr/local/bin \
    /etc/kubernetes \
    /etc/kubernetes/pki \
    /var/run/kubernetes \
    /var/lib/kubelet

# Copy the kube-apiserver binary
COPY --from=kube_apiserver /usr/local/bin/kube-apiserver /usr/local/bin/kube-apiserver

# Set execution permissions and ownership
RUN chmod -R 777 /var/run/kubernetes /etc/kubernetes /etc/kubernetes/pki /var/lib/kubelet && \
    chown -R 1001:1001 /usr/local/bin/kube-apiserver /etc/kubernetes /etc/kubernetes/pki /var/run/kubernetes /var/lib/kubelet

# Switch to non-root user for OpenShift compatibility
USER 1001

# Expose the Kubernetes API server port
EXPOSE 6443

# Run the kube-apiserver
ENTRYPOINT ["/usr/local/bin/kube-apiserver"]


EOF
docker build -t QQQQQQ/docker-prod-dma/swb/deployment-service/argocd/kube-apiserver:v1.28.0.7 .
docker push QQQQQQ/docker-prod-dma/swb/deployment-service/argocd/kube-apiserver:v1.28.0.7


docker pull QQQQQQ/docker-quay-io-remote/coreos/etcd:v3.5.0
docker tag QQQQQQ/docker-quay-io-remote/coreos/etcd:v3.5.0 QQQQQQ/docker-prod-dma/swb/deployment-service/argocd/etcd:v3.5.0.7
docker push QQQQQQ/docker-prod-dma/swb/deployment-service/argocd/etcd:v3.5.0.7