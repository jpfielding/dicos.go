FROM rockylinux:9 AS base

ARG REPO_NAME=dicos.go
ARG USERNAME=dicos
ARG USER_UID=1000
ARG USER_GID=$USER_UID

# Add ca certs so we can build the image on vpn
COPY cas.pem /etc/pki/ca-trust/source/anchors/cas.pem
RUN update-ca-trust

RUN set -eux -o pipefail && \
    dnf --setopt=***.sslverify=false install -y epel-release && \
    dnf --setopt=***.sslverify=false -y update
RUN set -eux -o pipefail && \
    dnf --setopt=***.sslverify=false install -y ca-certificates curl-minimal git iproute jq make msitools ncurses nss-tools procps sudo wget which zsh zip

FROM base as user

# Create the user
RUN groupadd --gid ${USER_GID} ${USERNAME} && \
  useradd --uid ${USER_UID} --gid ${USER_GID} -m ${USERNAME} -s /bin/zsh && \
  echo ${USERNAME} ALL=\(root\) NOPASSWD:ALL > /etc/sudoers.d/${USERNAME} && \
  chmod 0440 /etc/sudoers.d/${USERNAME}

########### NON ROOT FROM HERE DOWN ###########
USER ${USERNAME}

RUN set -eux && \
    mkdir -p ${HOME}/bin && \
    sudo mkdir -p /workdir/${REPO_NAME} && \
    sudo chown ${USERNAME}:${USERNAME} /workdir

# envs
ENV HOME "/home/${USERNAME}"
ENV SDK_HOME ${HOME}/sdk
ENV PATH ${HOME}/bin:${PATH}

# claude
ENV NODE_EXTRA_CA_CERTS="/etc/pki/ca-trust/source/anchors/cas.pem"
ENV PATH="$HOME/.local/bin:$PATH"

# go
ENV GO_VERSION "1.25.5"
ENV GOPATH ${SDK_HOME}/go
ENV PATH ${GOPATH}/bin:${HOME}/sdk/go:${PATH}
RUN set -eux && \
    export GO_ARCH="$(arch | sed 's/aarch64/arm64/' | sed 's/x86_64/amd64/')" && \
    cd /tmp && mkdir -p ${SDK_HOME} && \
    curl -k -o go.tar.gz -L https://go.dev/dl/go${GO_VERSION}.linux-${GO_ARCH}.tar.gz && \
    tar xzf go.tar.gz && mv go ${SDK_HOME} && rm /tmp/go.tar.gz && \
    cd ${SDK_HOME} && mv go go${GO_VERSION} && ln -s go${GO_VERSION} go

# pretty bash
COPY --chown=${USERNAME}:${USERNAME} .bash_pretty ${HOME}/.bash_pretty
RUN set -eux && \
    echo "source ${HOME}/.bash_pretty" >> ${HOME}/.bashrc

