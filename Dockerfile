FROM busybox:1.27.1

ADD bin/qingcloud-cloud-controller-manager /usr/bin

ENTRYPOINT ["/usr/bin/qingcloud-cloud-controller-manager"]