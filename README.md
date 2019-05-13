# qingcloud-cloud-controller-manager
> A kubernetes cloud-controller-manager for the qingcloud

## 如何手动部署 qingcloud 负载均衡器插件
> 注：appcenter中的k8s集群都已自动配置，无需手动安装

1. 修改所有节点的kubelet 配置参数，添加参数 KUBE_CLOUD_PROVIDER="--cloud-provider=external"，参考<https://github.com/QingCloudAppcenter/kubernetes/blob/master/confd/templates/k8s/kubelet.tmpl>*(注：这一步从测试来看并不是必须的)*
2. 如果主机的名称(hostname)已经被修改（默认都是`instance_id`），不在是`instance_id`，需要在各个节点上包括master执行下面的命令
    ```bash
    instance_id=`cat /etc/qingcloud/instance-id`
    kubectl annotate nodes {nodename} "node.beta.kubernetes.io/instance-id=${instance_id}" ##请替换nodename
    ```
3. 在**master**节点生成配置文件。下面展示了配置文件的样子，用户需要手动添加这些文件，并且去掉注释。
-  `cat /etc/kubernetes/qingcloud.conf `  此文件是cloud-controller的配置文件，文件名和文件位置都不宜更改。
    ```
    [Global] 
    qyConfigPath = /etc/qingcloud/config.yaml  #青云api密钥存放的位置，必填
    zone = ap2a
    defaultVxNetForLB = vxnet-lddzg8e #lb的默认vxnet，必填
    clusterID = "mycluster" #集群名称，必填，任意字符串，但是必须保证在一个网段内唯一。
    ```
- 生成api密钥文件，具体的文件样式可参考<https://docs.qingcloud.com/product/cli/#%E6%96%B0%E6%89%8B%E6%8C%87%E5%8D%97>
    ```
    qy_access_key_id: 'QINGCLOUDACCESSKEYID'
    qy_secret_access_key: 'QINGCLOUDSECRETACCESSKEYEXAMPLE'
    zone: 'pek1'
    ```
    创建`secret`, `kubectl create secret generic qcsecret --from-file=$secret_file -n kube-system`，替换其中的secret_file为上面生成的密钥地址。

1. 安装yaml，等待安装完成即可
   ```
   kubectl apply -f https://raw.githubusercontent.com/yunify/qingcloud-cloud-controller-manager/master/deploy/kube-cloud-controller-manager.yaml
   ```
   
# 如何使用 qingcloud 负载均衡器插件

请参考[负载均衡器配置指南](docs/configure.md)

