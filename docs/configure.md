# 青云负载均衡器配置指南

> 大部分的配置都是基于需要使用LB插件对应Service的`Annotation`，简单来讲，`Annotation`记录了关于当前服务的额外配置信息，参考<https://kubernetes.io/docs/concepts/overview/working-with-objects/annotations/>了解如何使用

## 一、负载均衡器属性配置

- `ServiceAnnotationLoadBalancerType`：在Service的`annotations`中添加此Key，表示使用yunify负载均衡器，不添加此Key的Service在Event中会有错误。其值范围和[create_loadbalancer](https://docs.qingcloud.com/product/api/action/lb/create_loadbalancer.html)中的type相同。

- 青云负载均衡器支持http/s协议的负载均衡，如果想要使用青云负载均衡器更多的功能，请将服务的特定的端口name写为`http`或`https`，如下：
```yaml
kind: Service
apiVersion: v1
metadata:
  name:  mylbapp
  annotations:
    service.beta.kubernetes.io/qingcloud-load-balancer-type: "0"
spec:
  selector:
    app:  mylbapp
  type:  LoadBalancer 
  ports:
  - name:  http  #这里指定port name,如果有多个http端口，都可以指定为http
    port:  8088
    targetPort:  80
```
将ServicePort命名为http之后，就可以在青云控制台上查看http监控。更多的功能支持诸如`转发规则定义`、`透明代理`等功能还在开发中

## 二、公网IP配置
> 使用青云负载均衡器时需要配合一个公网ip，cloud-controller带有IP管理功能。

### 手动配置公网ip
这是本插件的默认方式，即用户需要在Service中添加公网ip的annotation。首先在Service annotation中添加如下key: `service.beta.kubernetes.io/qingcloud-load-balancer-eip-source`，并设置其值为`mannual`。如果不设置此Key，默认是`mannual`。

然后必须继续添加`service.beta.kubernetes.io/qingcloud-load-balancer-eip-ids`的key，其值公网IP的ID，如果需要绑定多个IP，以逗号分隔。请确保此IP处于可用状态。

如果该服务对应lb的已经存在，默认情况下，会将上述注解中指定的eip追加绑定到已有的lb；如果只想保留注解中指定的eip，不需要lb之前其他eip，则需添加注解`service.beta.kubernetes.io/qingcloud-load-balancer-eip-replace: "true"`。

如下：
```yaml
kind: Service
apiVersion: v1
metadata:
  name:  mylbapp
  annotations:
    service.beta.kubernetes.io/qingcloud-load-balancer-eip-ids: "eip-xxxxx"  #这里设置EIP
    service.beta.kubernetes.io/qingcloud-load-balancer-type: "0"
spec:
  selector:
    app:  mylbapp
  type:  LoadBalancer 
  ports:
  - name:  http
    port:  8088
    targetPort:  80
```

### 自动获取IP
自动获取ip有三种方式，分别是：

1. 自动获取当前账户下处于可用的EIP，如果找不到返回错误。
2. 自动获取当前账户下处于可用的EIP，如果找不到则申请一个新的
3. 不管账户下有没有可用的EIP，申请一个新EIP

如果开启EIP的自动获取的功能，需满足两个条件：
1. 集群的配置文件中`/etc/kubernetes/qingcloud.conf`配置`userID`，是因为一些用户API权限较大，会获取到其他用户的IP。
2. 配置Service Annotations中的`service.beta.kubernetes.io/qingcloud-load-balancer-eip-source`不为`mannual`，上述三种方式对应的值分别为：
   1. `use-available`
   2. `auto`
   3. `allocate`

参考下面样例：
```yaml
kind: Service
apiVersion: v1
metadata:
  name:  mylbapp
  annotations:
    service.beta.kubernetes.io/qingcloud-load-balancer-eip-source: "auto" #也可以填use-available,allocate
    service.beta.kubernetes.io/qingcloud-load-balancer-type: "0"
spec:
  selector:
    app:  mylbapp
  type:  LoadBalancer 
  ports:
  - name:  http
    port:  8088
    targetPort:  80
```

## 三、配置Service使用云上现有的LB
### 注意事项
1. 在此模式下，LB的一些功能属性，包括7层协议配置，LB容量，LB绑定的EIP都由用户配置，LB插件不会修改任何属性，除了增删一些监听器
2. 确保LB正常工作，并且和Service对应的端口不冲突。LB已有的监听器监听的端口不能和需要暴露的服务冲突。

### 如何配置
1. `service.beta.kubernetes.io/qingcloud-load-balancer-eip-strategy`设置为"reuse-lb"
2. `service.beta.kubernetes.io/qingcloud-load-balancer-id`设置为现有的LB ID，类似"lb-xxxxxx"

其余设置都会被LB插件忽略。

### 参考Service
```yaml
kind: Service
apiVersion: v1
metadata:
  name:  reuse-lb
  annotations:
    service.beta.kubernetes.io/qingcloud-load-balancer-eip-strategy: "reuse-lb"
    service.beta.kubernetes.io/qingcloud-load-balancer-id: "lb-oglqftju"
spec:
  selector:
    app:  mylbapp
  type:  LoadBalancer 
  ports:
  - name:  http
    port:  8090
    targetPort:  80
```

## 四、配置LB的监听器属性

> 详细配置说明可以查看官方文档：https://docsv4.qingcloud.com/user_guide/development_docs/api/api_list/network/loadbalancer/add_lb_listeners/

### 如何配置
1. 设置监听器的健康检查方式，`service.beta.kubernetes.io/qingcloud-lb-listener-healthycheckmethod`，对于 tcp 协议默认是 tcp 方式，对于 udp 协议默认是 udp 方式
2. 设置监听器的健康检查参数，`service.beta.kubernetes.io/qingcloud-lb-listener-healthycheckoption`，默认是 "10|5|2|5"
3. 支持 roundrobin/leastconn/source 三种负载均衡方式，`service.beta.kubernetes.io/qingcloud-lb-listener-balancemode`，默认是 roundrobin
4. 支持 http/https 协议的配置，`service.beta.kubernetes.io/qingcloud-lb-listener-protocol`，没有此注解则默认使用 Service 所用协议
5. 支持 https 协议证书的配置，`service.beta.kubernetes.io/qingcloud-lb-listener-cert`，如果配置的 https 协议，则必须配置证书
6. 支持监听器的超时时间，` service.beta.kubernetes.io/qingcloud-lb-listener-timeout`，不配置默认是50，可选范围为（10 ～ 86400），单位为s
7. 支持监听器的场景配置，`service.beta.kubernetes.io/qingcloud-lb-listener-scene`，默认是0，配置为1表示启用 Keep-Alive，详细说明参考官网文档
8. 支持http头转发字段的配置， `service.beta.kubernetes.io/qingcloud-lb-listener-forwardfor`，默认是0,配置为1表示设置 X-Forwarded-For 头字段获取客户端的真实IP传给后端，详细说明参考官网文档
9. 支持监听器附加选项的配置，`service.beta.kubernetes.io/qingcloud-lb-listener-listeneroption`，默认是0，配置为4表示启用数据压缩，详细说明参考官网文档


因为一个LB会有多个监听器，所以进行service注解设置时，通过如下格式区分不同监听器：`80:xxx,443:xxx`。

### 参考Service
```yaml
kind: Service
apiVersion: v1
metadata:
  name:  reuse-lb
  annotations:
    service.beta.kubernetes.io/qingcloud-load-balancer-eip-strategy: "reuse-lb"
    service.beta.kubernetes.io/qingcloud-load-balancer-id: "lb-oglqftju"
    service.beta.kubernetes.io/qingcloud-lb-listener-healthycheckmethod: "8090:tcp"
    service.beta.kubernetes.io/qingcloud-lb-listener-healthycheckoption: "8090:10|5|2|5"
    service.beta.kubernetes.io/qingcloud-lb-listener-balancemode: "8090:source"
    service.beta.kubernetes.io/qingcloud-lb-listener-protocol: "8090:https"
    service.beta.kubernetes.io/qingcloud-lb-listener-cert: "8090:sc-77oko7zj"
    service.beta.kubernetes.io/qingcloud-lb-listener-timeout: "8080:10"
    service.beta.kubernetes.io/qingcloud-lb-listener-scene: "8080:1"
    service.beta.kubernetes.io/qingcloud-lb-listener-forwardfor: "8080:1"
    service.beta.kubernetes.io/qingcloud-lb-listener-listeneroption: "8080:4"
spec:
  selector:
    app:  mylbapp
  type:  LoadBalancer
  ports:
  - name:  http
    port:  8090
    protocol: TCP
    targetPort:  80
```
监听器参数说明：https://docsv3.qingcloud.com/network/loadbalancer/api/listener/modify_listener_attribute/

## 五、配置LB监听器backend
### 如何配置
根据 `service` 的 `externalTrafficPolicy` 字段的取值，决定LB监听器backend的添加策略
  - `Local`: 只会添加提供服务pod所在的Worker节点为LB监听器的backend
  - `Cluster`: 如果`service`中不显式指定 `externalTrafficPolicy` 字段的值，则默认为`Cluster`；这种模式下，可以通过给服务添加相关注解来指定LB监听器backend的添加规则

`Cluster`模式下，目前支持的 `service` 注解有：

> 以下各种过滤节点的方式不能结合使用，如果指定了多个，则会按照以下注解的说明顺序选用第一个匹配到的注解，并按照该方法过滤节点；

- 使用指定Label的Worker节点作为后端服务器， `service.beta.kubernetes.io/qingcloud-lb-backend-label`，可以指定多个Label，多个Label以逗号分隔。例如：`key1=value1,key2=value2`，多个Label之间是And关系。同时，在需要成为后端服务器的Worker节点打上`key1=value1,key2=value2`的Label；只有服务指定的所有Label的key和value都和Worker节点匹配时，Worker节点会被选为服务的后端服务器；通过注解过滤节点后，如果没有满足条件的节点，为了避免服务中断，会添加默认数量的Worker节点为后端服务器；
- 使用指定数量的Worker节点作为后端服务器，`service.beta.kubernetes.io/qingcloud-lb-backend-count`，通过此注解指定该服务使用的负载均衡器backend节点数量；如果集群中节点状态发生变化，但是当前服务的lb后端数量就是用户指定的数量，并且已添加的所有lb后端节点在集群中都是ready状态的，则不会更新lb的backend；如果指定的数量为0或者不在可用节点数量的范围内，则使用默认值；

关于负载均衡器backend节点数量默认值的说明：

`Cluster`模式下，如果服务添加了上述任意注解，就认为用户想要限制负载均衡器backend数量，如果通过选用注解过滤后的节点数量为0，则会使用默认值，即：集群所有节点的1/3；默认值特殊情况说明：
  - 如果集群总worker节点数少于3个，则添加所有worker节点为backend，不再按照比例计算节点数；
  - 如果集群总worker节点数多于3个，若按照比例计算后少于3个，则设置为3个;

> 本章节所说的"所有Worker节点"特指所有 `Ready` 状态的Worker节点；

### 参考示例
#### Local模式
将服务的`externalTrafficPolicy`指定为`Local`，只添加pod所在Worker节点为backend

```yaml
kind: Service
apiVersion: v1
metadata:
  name:  reuse-lb
  annotations:
    service.beta.kubernetes.io/qingcloud-load-balancer-eip-strategy: "reuse-lb"
    service.beta.kubernetes.io/qingcloud-load-balancer-id: "lb-oglqftju"
spec:
  externalTrafficPolicy: Local
  selector:
    app:  mylbapp
  type:  LoadBalancer
  ports:
  - name:  http
    port:  8090
    protocol: TCP
    targetPort:  80
```

#### Cluster模式
##### 使用指定Label的Worker节点作为后端服务器
将服务的`externalTrafficPolicy`指定为`Cluster`，并在服务的注解`service.beta.kubernetes.io/qingcloud-lb-backend-label`中指定要添加为backend的worker节点的label:

```yaml
kind: Service
apiVersion: v1
metadata:
  name:  reuse-lb
  annotations:
    service.beta.kubernetes.io/qingcloud-load-balancer-eip-strategy: "reuse-lb"
    service.beta.kubernetes.io/qingcloud-load-balancer-id: "lb-oglqftju"
    service.beta.kubernetes.io/qingcloud-lb-backend-label: "test-label-key1=value1,test-label-key2=value2"
spec:
  externalTrafficPolicy: Cluster
  selector:
    app:  mylbapp
  type:  LoadBalancer
  ports:
  - name:  http
    port:  8090
    protocol: TCP
    targetPort:  80
```

Worker节点如果要添加为上述服务的backend,则必须同时有下面两个label；注意只有其中一个label并不会添加为上述服务的backend:

```yaml
apiVersion: v1
kind: Node
metadata:
  annotations:
    csi.volume.kubernetes.io/nodeid: '{"csi-qingcloud":"i-k5rl67a6"}'
    node.alpha.kubernetes.io/ttl: "0"
    node.beta.kubernetes.io/instance-id: i-k5rl67a6
  creationTimestamp: "2022-07-15T07:59:59Z"
  labels:
    test-label-key1: value1
    test-label-key2: value2
  name: worker-p001
  resourceVersion: "13094827"
  uid: b711ce74-9785-41bf-b0ac-777e3c9c0c34
spec:
  podCIDR: 10.10.1.0/24
  podCIDRs:
  - 10.10.1.0/24
status:
  ...
```

##### 使用指定数量的Worker节点作为后端服务器

```yaml
kind: Service
apiVersion: v1
metadata:
  name:  reuse-lb
  annotations:
    service.beta.kubernetes.io/qingcloud-load-balancer-eip-strategy: "reuse-lb"
    service.beta.kubernetes.io/qingcloud-load-balancer-id: "lb-oglqftju"
    service.beta.kubernetes.io/qingcloud-lb-backend-count: "3"
spec:
  externalTrafficPolicy: Cluster
  selector:
    app:  mylbapp
  type:  LoadBalancer
  ports:
  - name:  http
    port:  8090
    protocol: TCP
    targetPort:  80
```

## 配置内网负载均衡器
### 已知问题
k8s在ipvs模式下，kube-proxy会把内网负载均衡器的ip绑定在ipvs接口上，这样会导致从LB过来的包被drop（进来的是主网卡，但是出去的时候发现ipvs有这么一个ip，路由不一致）故目前无法在IPVS模式下使用内网负载均衡器。参考[issue](https://github.com/kubernetes/kubernetes/issues/79783)
### 注意事项
1. 必须手动指定`service.beta.kubernetes.io/qingcloud-load-balancer-network-type`为`internal`，如果不指定或者填写其他值，都默认为公网LB，需要配置EIP
2. 可选指定LB所在的Vxnet，默认为创建LB插件配置文件中的`defaultVxnet`，手动配置vxnet的annotation为`service.beta.kubernetes.io/qingcloud-load-balancer-vxnet-id`
3. 可选指定LB 内网ip，通过`service.beta.kubernetes.io/qingcloud-load-balancer-internal-ip`指定。**注意，当前不支持更换内网IP**
### 参考Service
```yaml
kind: Service
apiVersion: v1
metadata:
  name:  mylbapp
  namespace: default
  annotations:
    service.beta.kubernetes.io/qingcloud-load-balancer-type: "0"
    service.beta.kubernetes.io/qingcloud-load-balancer-network-type: "internal"
    service.beta.kubernetes.io/qingcloud-load-balancer-internal-ip: "192.168.0.1" ##如果要路由器自动分配删掉这一行
spec:
  selector:
    app:  mylbapp
  type:  LoadBalancer 
  ports:
  - name:  http
    port:  80
    targetPort:  80
```