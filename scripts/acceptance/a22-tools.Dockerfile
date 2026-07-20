FROM mysql:8.4

RUN microdnf install -y jq \
    && microdnf clean all \
    && rm -rf /var/cache/dnf /var/cache/yum

WORKDIR /workspace
