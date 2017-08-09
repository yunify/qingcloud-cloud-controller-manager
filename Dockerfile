FROM busybox:1.27.1

RUN ln -s /qingcloud-cloud-controller-manager /usr/bin/qingcloud-cloud-controller-manager

COPY bin/qingcloud-cloud-controller-manager /qingcloud-cloud-controller-manager