# Copyright (c) 2022 Intel Corporation
# SPDX-License-Identifier: Apache-2.0

FROM golang:1.19.1 AS builder

WORKDIR /ctlplane
COPY . ./
RUN go mod download && make build

FROM scratch
WORKDIR /
COPY --from=builder /ctlplane/bin/ctlplane .
EXPOSE 31000
ENTRYPOINT ["/ctlplane"]
