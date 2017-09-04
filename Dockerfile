FROM alpine:edge AS build
RUN apk update
RUN apk upgrade
RUN apk add go gcc g++ make git linux-headers bash
WORKDIR /app
ENV GOPATH /app
ADD . /app/src/github.com/yunify/qingcloud-cloud-controller-manager
RUN cd /app/src/github.com/yunify/qingcloud-cloud-controller-manager && rm -rf bin/ && make

FROM alpine:latest
MAINTAINER calvinyu <calvinyu@yunify.com>

COPY --from=build /app/src/github.com/yunify/qingcloud-cloud-controller-manager/bin/qingcloud-cloud-controller-manager /bin/qingcloud-cloud-controller-manager
ENV PATH "/bin/qingcloud-cloud-controller-manager:$PATH"