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
这是本插件的默认方式，即用户需要在Service中添加公网ip的annotation。首先在Service annotation中添加如下key: `service.beta.kubernetes.io/qingcloud-load-balancer-eip-source`，并设置其值为`mannual`。如果不设置此Key，默认是`mannual`。然后必须继续添加`service.beta.kubernetes.io/qingcloud-load-balancer-eip-ids`的key，其值公网IP的ID，如果需要绑定多个IP，以逗号分隔。请确保此IP处于可用状态。如下：
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

## 三、配置多个Service共享一个EIP

由于公网IP是稀缺资源，我们提供了多个Service共享一个EIP的模式。使用这个模式有一定限制：
1. EIP被设置为多个Service共享模式之后，所有手动指定这个EIP且不设置共享的Service将无法正确获取LB
2. 多个Service对外暴露的端口不能冲突，例如不能有两个Service同时暴露8080端口，这种情况下，我们建议将其中一个改为8081
3. 一个EIP最多能被十个端口共享，即如果一个Service只需要对外暴露一个端口的话，那么可以最多有10个Service共享这个EIP

### 如何配置
1. `service.beta.kubernetes.io/qingcloud-load-balancer-eip-source`必须为`manual`
2. `service.beta.kubernetes.io/qingcloud-load-balancer-eip-ids`中填入共享的EIP ID
3. `service.beta.kubernetes.io/qingcloud-load-balancer-eip-strategy`设置为`reuse`

参考下面的例子，下面两个service共有了一个EIP
```yaml
kind: Service
apiVersion: v1
metadata:
  name:  reuse-eip1
  annotations:
    service.beta.kubernetes.io/qingcloud-load-balancer-eip-ids: "eip-xxxxxx"
    service.beta.kubernetes.io/qingcloud-load-balancer-type: "0"
    service.beta.kubernetes.io/qingcloud-load-balancer-eip-strategy: "reuse"
spec:
  selector:
    app:  mylbapp
  type:  LoadBalancer 
  ports:
  - name:  http
    port:  8089
    targetPort:  80
---

kind: Service
apiVersion: v1
metadata:
  name:  reuse-eip2
  annotations:
    service.beta.kubernetes.io/qingcloud-load-balancer-eip-ids: "eip-xxxxxx"
    service.beta.kubernetes.io/qingcloud-load-balancer-type: "0"
    service.beta.kubernetes.io/qingcloud-load-balancer-eip-strategy: "reuse"
spec:
  selector:
    app:  mylbapp
  type:  LoadBalancer 
  ports:
  - name:  http
    port:  8090
    targetPort:  80
```

## 四、配置Service使用云上现有的LB
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

## 配置内网负载均衡器

### 注意事项
1. 必须手动指定`service.beta.kubernetes.io/qingcloud-load-balancer-network-type`为`internal`，如果不指定或者填写其他值，都默认为公网LB，需要配置EIP
2. 可选指定LB所在的Vxnet，默认为创建LB插件配置文件中的`defaultVxnet`
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
spec:
  selector:
    app:  mylbapp
  type:  LoadBalancer 
  ports:
  - name:  http
    port:  80
    targetPort:  80
```