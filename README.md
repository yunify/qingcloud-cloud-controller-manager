# qingcloud-cloud-controller-manager
> A kubernetes cloud-controller-manager for the qingcloud

## 如何手动部署 qingcloud 负载均衡器插件
> 注：appcenter中的k8s集群都已自动配置，无需手动安装

1. 如果主机的名称(hostname)已经被修改（默认都是`instance_id`），不再是`instance_id`，需要在各个节点上包括master执行下面的命令
    ```bash
    instance_id=`cat /etc/qingcloud/instance-id`
    kubectl annotate nodes {nodename} "node.beta.kubernetes.io/instance-id=${instance_id}" ##请替换nodename
    ```
2. 生成配置文件。下面展示了配置文件的样子(Yaml格式)。
  `cat qingcloud.yaml `，修改其中一些字段：
    ```yaml
    qyConfigPath: /etc/qingcloud/config.yaml  #青云api密钥存放的位置，必填
    zone: ap2a
    defaultVxNetForLB: vxnet-lddzg8e #lb的默认vxnet，必填
    clusterID: "mycluster" #集群名称，必填，任意字符串，但是必须保证在一个网段内唯一。
    ```
    对于QKE，还需要在上面的配置文件中添加下面的字段
    ```yaml
    isApp: true
    tagIDs:
    - tag-xxxxxx # 替换tagid
    ```
    文件保存之后使用下面的命令创建配置文件
    ```bash
    kubectl create configmap lbconfig --from-file=qingcloud.yaml -n kube-system
    ```
3. 生成api密钥文件，具体的文件样式可参考<https://docs.qingcloud.com/product/cli/#%E6%96%B0%E6%89%8B%E6%8C%87%E5%8D%97>
    ```
    qy_access_key_id: 'QINGCLOUDACCESSKEYID'
    qy_secret_access_key: 'QINGCLOUDSECRETACCESSKEYEXAMPLE'
    zone: 'pek1'
    ```
    创建`secret`, `kubectl create secret generic qcsecret --from-file=$secret_file -n kube-system`，替换其中的secret_file为上面生成的密钥地址。

3. 安装yaml，等待安装完成即可
   ```
   kubectl apply -f https://raw.githubusercontent.com/yunify/qingcloud-cloud-controller-manager/master/deploy/kube-cloud-controller-manager.yaml
   ```
   
# 如何使用 qingcloud 负载均衡器插件

请参考[负载均衡器配置指南](docs/configure.md)

