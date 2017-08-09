FROM busybox:1.27.1-glibc

COPY bin/qingcloud-cloud-controller-manager /qingcloud-cloud-controller-manager

RUN ln -s /qingcloud-cloud-controller-manager /bin/qingcloud-cloud-controller-manager