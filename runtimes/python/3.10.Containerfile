FROM ubuntu:24.04

RUN apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get install -y \
        python3.10 \
        python3-pip \
        ca-certificates \
        && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

# Create symlink for compatibility
RUN ln -sf /usr/bin/python3.10 /usr/local/bin/python3.10 && \
    ln -sf /usr/bin/python3 /usr/local/bin/python3 && \
    ln -sf /usr/bin/pip3 /usr/local/bin/pip3

ENV PYTHONUNBUFFERED=1
