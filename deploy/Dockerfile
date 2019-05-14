# Copy the controller-manager into a thin image
FROM alpine
RUN apk --no-cache add iptables ca-certificates \
    && update-ca-certificates 2>/dev/null || true
WORKDIR /
COPY . .
ENTRYPOINT ["/manager"]