# ## 如何部署 qingcloud 负载均衡器插件

1. 以 static pod 方式部署 qingcloud 负载均衡器插件
   复制 [kube-cloud-controller-manager.yaml](deploy/kube-cloud-controller-manager.yaml) 到 主节点 /etc/kubernetes/manifests 目录下，根据情况修改 yaml 中的日志级别 ${KUBE_LOG_LEVEL}
   注意：需保证此 cloud controller 部署在主节点上
2. 修改集群中的 kube-controller-manager.yaml，添加参数 --cloud-provider=external，[参考](https://github.com/QingCloudAppcenter/kubernetes/blob/master/confd/templates/k8s/kube-controller-manager.yaml.tmpl)
3. 修改 kubelet 配置参数，添加参数 KUBE_CLOUD_PROVIDER="--cloud-provider=external"，[参考](https://github.com/QingCloudAppcenter/kubernetes/blob/master/confd/templates/k8s/kubelet.tmpl)



## 如何使用 qingcloud 负载均衡器插件

请参考[青云官方用户手册](https://docs.qingcloud.com/product/container/k8s#%E8%B4%9F%E8%BD%BD%E5%9D%87%E8%A1%A1%E5%99%A8)，为 service 定义添加合适的 annotation

A kubernetes cloud-controller-manager for the qingcloud
