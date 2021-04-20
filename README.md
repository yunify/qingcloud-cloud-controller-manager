# qingcloud-cloud-controller-manager
> A kubernetes cloud-controller-manager for the qingcloud

## 注意事项
1. **appcenter中的k8s集群都已自动配置，无需手动安装**
2. **集群至少需要两个节点，因为k8s不会将master节点加入到lb后端**

## 如何手动部署 qingcloud 负载均衡器插件
注意：appcenter中的k8s集群都已自动配置，无需手动安装

1. 如果主机的名称(hostname)已经被修改（默认都是`instance_id`），不再是`instance_id`，需要在各个节点上包括master执行下面的命令
    
   ```bash
   instance_id=`cat /etc/qingcloud/instance-id`
   kubectl annotate nodes {nodename} "node.beta.kubernetes.io/instance-id=${instance_id}" ##请替换nodename
   ```
   
2. 生成api密钥文件，具体生成方法可参考<https://docs.qingcloud.com/product/cli/#%E6%96%B0%E6%89%8B%E6%8C%87%E5%8D%97>
   
   `touch secret.yaml; vim secret.yaml` , 填充通过上面方法获取到的内容， 内容例子如下：
   
   ```yaml
   qy_access_key_id: 'QINGCLOUDACCESSKEYID' #修改为自己的qy_access_key_id
   qy_secret_access_key: 'QINGCLOUDSECRETACCESSKEYEXAMPLE' #修改为自己的qy_secret_access_key
   zone: 'pek3' #修改为k8s集群节点所在zone
   ```
    
   文件保存之后使用下面的命令创建配置文件
   ```bash
   kubectl create secret generic qcsecret --from-file=config.yaml=./secret.yaml -n kube-system
   ```

3. 生成负载均衡器插件配置文件。
  
    `touch qingcloud.yaml; vim qingcloud.yaml `，修改其中一些字段：
    ```yaml
    zone: pek3  #必填，值与第二步中的zone相同
    defaultVxNetForLB: vxnet-lddzg8e #k8s集群主机所在私有网络ID， 可以通过IAAS控制台“网络与CDN/私有网络”查看
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

4. 安装yaml，等待安装完成即可
   ```
   kubectl apply -f https://raw.githubusercontent.com/yunify/qingcloud-cloud-controller-manager/master/deploy/kube-cloud-controller-manager.yaml
   ```
   
# 如何使用 qingcloud 负载均衡器插件

请参考[负载均衡器配置指南](docs/configure.md)

