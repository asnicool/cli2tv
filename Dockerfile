FROM golang:1.26

ARG DEBIAN_FRONTEND=noninteractive
RUN \
  apt-get update && \
  apt-get clean && \
  rm -rf /var/lib/apt/lists/*

WORKDIR /usr/local/src/cli2tv/
COPY . .

ENV GODEBUG=asyncpreemptoff=1
RUN make
RUN make install

ENTRYPOINT [ "cli2tv" ]
