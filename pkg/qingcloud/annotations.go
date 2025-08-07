package qingcloud

import (
	"fmt"
	"strconv"
	"strings"

	qcservice "github.com/yunify/qingcloud-sdk-go/service"
	v1 "k8s.io/api/core/v1"

	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/util"
)

const (
	NodeAnnotationInstanceID = "node.beta.kubernetes.io/instance-id"

	//1. Configure Network
	ServiceAnnotationLoadBalancerNetworkType        = "service.beta.kubernetes.io/qingcloud-load-balancer-network-type"
	NetworkModePublic                        string = "public"
	NetworkModeInternal                             = "internal"

	//1.1 Configure EIP
	// ServiceAnnotationLoadBalancerEipIds is the annotation which specifies a list of eip ids.
	// The ids in list are separated by ',', e.g. "eip-j38f2h3h,eip-ornz2xq7". And this annotation should
	// NOT be used with ServiceAnnotationLoadBalancerVxnetId. Please make sure there is one and only one
	// of them being set
	ServiceAnnotationLoadBalancerEipIds    = "service.beta.kubernetes.io/qingcloud-load-balancer-eip-ids"
	ServiceAnnotationLoadBalancerEipSource = "service.beta.kubernetes.io/qingcloud-load-balancer-eip-source"
	ManualSet                              = "manual"
	UseAvailableOrAllocateOne              = "auto"
	UseAvailableOnly                       = "use-available"
	AllocateOnly                           = "allocate"

	ServiceAnnotationEipReplace = "service.beta.kubernetes.io/qingcloud-load-balancer-eip-replace"

	//1.2 Configure vxnet
	// ServiceAnnotationLoadBalancerVxnetId is the annotation which indicates the very vxnet where load
	// balancer resides. This annotation should NOT be used when ServiceAnnotationLoadBalancerEipIds is
	// set.
	ServiceAnnotationLoadBalancerVxnetID         = "service.beta.kubernetes.io/qingcloud-load-balancer-vxnet-id"
	ServiceAnnotationLoadBalancerInternalIP      = "service.beta.kubernetes.io/qingcloud-load-balancer-internal-ip"
	ServiceAnnotationLoadBalancerInternalReuseID = "service.beta.kubernetes.io/qingcloud-load-balancer-reuse-id"

	//2. Configure loadbalance
	//2.1 Configure loadbalance name
	// ServiceAnnotationLoadBalancerID is needed when user want to use exsiting lb
	ServiceAnnotationLoadBalancerID = "service.beta.kubernetes.io/qingcloud-load-balancer-id"
	// ServiceAnnotationLoadBalancerPolicy is usd to specify EIP use strategy
	// reuse represent the EIP can be shared with other service which has no port conflict
	// exclusive is the default value, means every service has its own EIP
	ServiceAnnotationLoadBalancerPolicy = "service.beta.kubernetes.io/qingcloud-load-balancer-eip-strategy"
	//ReuseExistingLB  use existing loadbalancer on the cloud
	ReuseExistingLB = "reuse-lb"
	// Exclusive is the default value, means every service has its own EIP
	Exclusive = "exclusive"
	// Shared represent the EIP can be shared with other service which has no port conflict
	Shared = "reuse"

	//2.2 Configure loadbalance attributes
	// ServiceAnnotationLoadBalancerType is the annotation used on the
	// service to indicate that we want a qingcloud loadBalancer type.
	// value "0" means the LB can max support 5000 concurrency connections, it's default type.
	// value "1" means the LB can max support 20000 concurrency connections.
	// value "2" means the LB can max support 40000 concurrency connections.
	// value "3" means the LB can max support 100000 concurrency connections.
	// value "4" means the LB can max support 200000 concurrency connections.
	// value "5" means the LB can max support 500000 concurrency connections.
	ServiceAnnotationLoadBalancerType = "service.beta.kubernetes.io/qingcloud-load-balancer-type"
	//LoadBalancer node number
	ServiceAnnotationLoadBalancerNodes = "service.beta.kubernetes.io/qingcloud-load-balancer-nodes"

	//3. Configure sg

	//4. Configure listener
	// tcp or http, such as "80:tcp,443:tcp"
	ServiceAnnotationListenerHealthyCheckMethod = "service.beta.kubernetes.io/qingcloud-lb-listener-healthycheckmethod"
	// inter | timeout | fall | rise , such as "80:10|5|2|5,443:10|5|2|5", default is "*:10|5|2|5"
	ServiceAnnotationListenerHealthyCheckOption = "service.beta.kubernetes.io/qingcloud-lb-listener-healthycheckoption"
	// roundrobin / leastconn / source
	ServiceAnnotationListenerBalanceMode = "service.beta.kubernetes.io/qingcloud-lb-listener-balancemode"
	// port:certificate, such as "6443:sc-77oko7zj,8443:sc-77oko7zj"
	ServiceAnnotationListenerServerCertificate = "service.beta.kubernetes.io/qingcloud-lb-listener-cert"
	// port:protocol, such as "443:https,80:http"
	ServiceAnnotationListenerProtocol = "service.beta.kubernetes.io/qingcloud-lb-listener-protocol"
	// port:timeout, such as "443:50", the value must in range 10 ～ 86400
	ServiceAnnotationListenerTimeout = "service.beta.kubernetes.io/qingcloud-lb-listener-timeout"
	// port:scene, such as "443:0", default is "*:0"
	ServiceAnnotationListenerScene = "service.beta.kubernetes.io/qingcloud-lb-listener-scene"
	// port:forwardforoption, such as "443:0", default is "*:0"
	ServiceAnnotationListenerForwardfor = "service.beta.kubernetes.io/qingcloud-lb-listener-forwardfor"
	//port:listeneroption, such as "443:0", default is "*:0"
	ServiceAnnotationListenerListenerOption = "service.beta.kubernetes.io/qingcloud-lb-listener-listeneroption"

	// 5. Configure backend
	// backend label, such as "key1=value1,key2=value2"
	ServiceAnnotationBackendLabel = "service.beta.kubernetes.io/qingcloud-lb-backend-label"
	// backend count limit, if value is 0 or greater than cluster ready worker, will use default value : 1/3 of cluster ready worker
	ServiceAnnotationBackendCount = "service.beta.kubernetes.io/qingcloud-lb-backend-count"
)

type LoadBalancerConfig struct {
	//Network
	EipIDs     []*string
	EipSource  *string
	VxNetID    *string
	EipReplace bool

	//Attribute
	LoadBalancerType *int
	NodeCount        *int
	InternalIP       *string
	PlaceGroupID     *string
	SecurityGroupID  *string //if not set, will use default security group

	//listener attrs
	healthyCheckMethod *string
	healthyCheckOption *string
	balanceMode        *string
	ServerCertificate  *string
	Protocol           *string
	Timeout            *string
	Scene              *string
	Forwardfor         *string
	ListenerOption     *string

	//backend
	BackendLabel       string
	BackendCountConfig string
	BackendCountResult int

	//It's just for defining names, nothing more.
	NetworkType      string
	Policy           string
	InternalReuseID  *string
	ReuseLBID        string
	LoadBalancerName string
	listenerName     string
	sgName           string
	InstanceIDs      []*string
}

func LBBackendName(config *LoadBalancerConfig, instance string) string {
	return fmt.Sprintf("backend_%s_%s", config.listenerName, instance)
}

func (qc *QingCloud) ParseServiceLBConfig(cluster string, service *v1.Service) (*LoadBalancerConfig, error) {
	annotation := service.Annotations
	if len(annotation) <= 0 {
		return nil, fmt.Errorf("service %s annotation is empty", service.Namespace+"/"+service.Name)
	}

	config := &LoadBalancerConfig{}

	lbEipIds, hasEip := annotation[ServiceAnnotationLoadBalancerEipIds]
	if hasEip && lbEipIds != "" {
		config.EipIDs = qcservice.StringSlice(strings.Split(lbEipIds, ","))
	} else {
		source := annotation[ServiceAnnotationLoadBalancerEipSource]
		config.EipSource = &source
		switch source {
		case AllocateOnly:
		case UseAvailableOnly:
		case UseAvailableOrAllocateOne:
		default:
			config.EipSource = nil
		}
	}
	if eipReplace, ok := annotation[ServiceAnnotationEipReplace]; ok {
		switch eipReplace {
		case "true":
			config.EipReplace = true
		case "false":
			config.EipReplace = false
		default:
			config.EipReplace = false
		}
	}

	if vxnetID, ok := annotation[ServiceAnnotationLoadBalancerVxnetID]; ok {
		config.VxNetID = &vxnetID
	}
	if internalIP, ok := annotation[ServiceAnnotationLoadBalancerInternalIP]; ok {
		config.InternalIP = &internalIP
	}
	if internalReuseID, ok := annotation[ServiceAnnotationLoadBalancerInternalReuseID]; ok {
		config.InternalReuseID = &internalReuseID
	}
	//plg
	if qc.Config.PlaceGroupID != "" {
		config.PlaceGroupID = qcservice.String(qc.Config.PlaceGroupID)
	}
	//sg
	if qc.Config.SecurityGroupID != "" {
		config.SecurityGroupID = qcservice.String(qc.Config.SecurityGroupID)
	}

	//listener annotation
	if healthyCheckMethod, ok := annotation[ServiceAnnotationListenerHealthyCheckMethod]; ok {
		if err := validListenerStringConfig(ServiceAnnotationListenerHealthyCheckMethod, healthyCheckMethod); err != nil {
			return nil, err
		}
		config.healthyCheckMethod = &healthyCheckMethod
	}
	if healthyCheckOption, ok := annotation[ServiceAnnotationListenerHealthyCheckOption]; ok {
		if err := validListenerStringConfig(ServiceAnnotationListenerHealthyCheckOption, healthyCheckOption); err != nil {
			return nil, err
		}
		config.healthyCheckOption = &healthyCheckOption
	}
	if balanceMode, ok := annotation[ServiceAnnotationListenerBalanceMode]; ok {
		if err := validListenerStringConfig(ServiceAnnotationListenerBalanceMode, balanceMode); err != nil {
			return nil, err
		}
		config.balanceMode = &balanceMode
	}
	if serverCertificate, ok := annotation[ServiceAnnotationListenerServerCertificate]; ok {
		if err := validListenerStringConfig(ServiceAnnotationListenerServerCertificate, serverCertificate); err != nil {
			return nil, err
		}
		config.ServerCertificate = &serverCertificate
	}
	if protocol, ok := annotation[ServiceAnnotationListenerProtocol]; ok {
		if err := validListenerStringConfig(ServiceAnnotationListenerProtocol, protocol); err != nil {
			return nil, err
		}
		config.Protocol = &protocol
	}

	if timeoutConf, ok := annotation[ServiceAnnotationListenerTimeout]; ok {
		if err := validListenerTimeout(timeoutConf); err != nil {
			return nil, err
		}
		config.Timeout = &timeoutConf
	}
	if scene, ok := annotation[ServiceAnnotationListenerScene]; ok {
		if err := validListenerScene(scene); err != nil {
			return nil, err
		}
		config.Scene = &scene
	}
	if forwardfor, ok := annotation[ServiceAnnotationListenerForwardfor]; ok {
		if err := validListenerIntConfig(ServiceAnnotationListenerForwardfor, forwardfor); err != nil {
			return nil, err
		}
		config.Forwardfor = &forwardfor
	}
	if listenerOption, ok := annotation[ServiceAnnotationListenerListenerOption]; ok {
		if err := validListenerIntConfig(ServiceAnnotationListenerListenerOption, listenerOption); err != nil {
			return nil, err
		}
		config.ListenerOption = &listenerOption
	}

	//backend annotation
	if backendLabel, ok := annotation[ServiceAnnotationBackendLabel]; ok {
		config.BackendLabel = backendLabel
	}
	if backendCount, ok := annotation[ServiceAnnotationBackendCount]; ok {
		_, err := strconv.Atoi(backendCount)
		if err != nil {
			return nil, fmt.Errorf("please spec a valid value of loadBalancer backend count")
		}
		config.BackendCountConfig = backendCount
	}

	networkType := annotation[ServiceAnnotationLoadBalancerNetworkType]
	switch networkType {
	case NetworkModePublic:
		config.NetworkType = networkType
	case NetworkModeInternal:
		config.NetworkType = networkType
		if config.VxNetID == nil && qc.Config.DefaultVxNetForLB != "" {
			config.VxNetID = qcservice.String(qc.Config.DefaultVxNetForLB)
		}
	default:
		config.NetworkType = NetworkModePublic
	}

	if lbType, ok := annotation[ServiceAnnotationLoadBalancerType]; ok {
		t, err := strconv.Atoi(lbType)
		if err != nil {
			return nil, fmt.Errorf("pls spec a valid value of loadBalancer type, acceptted values are '0-3',err: %s", err.Error())
		}
		config.LoadBalancerType = &t
	}
	if nodes, ok := annotation[ServiceAnnotationLoadBalancerNodes]; ok {
		num, err := strconv.Atoi(nodes)
		if err != nil {
			return nil, fmt.Errorf("pls spec a valid value of loadBalancer node number")
		}
		config.NodeCount = &num
	}

	config.ReuseLBID = annotation[ServiceAnnotationLoadBalancerID]
	strategy := annotation[ServiceAnnotationLoadBalancerPolicy]
	switch strategy {
	case ReuseExistingLB:
		config.Policy = ReuseExistingLB
		if config.ReuseLBID == "" {
			return nil, fmt.Errorf("must specify 'service.beta.kubernetes.io/qingcloud-load-balancer-id' if 'service.beta.kubernetes.io/qingcloud-load-balancer-eip-strategy'=reuse-lb")
		}
	case Shared:
		if config.Policy == Shared {
			if config.NetworkType == NetworkModePublic {
				config.LoadBalancerName = fmt.Sprintf("k8s_lb_%s_%s", cluster, annotation[ServiceAnnotationLoadBalancerEipIds])
				break
			} else {
				if config.InternalIP == nil || config.InternalReuseID == nil {
					return nil, fmt.Errorf("must specify reuse-id or internalip if wants to use shared internal lb")
				}

				if config.InternalReuseID != nil {
					config.LoadBalancerName = fmt.Sprintf("k8s_lb_%s_%s", cluster, *config.InternalReuseID)
				} else {
					config.LoadBalancerName = fmt.Sprintf("k8s_lb_%s_%s", cluster, *config.InternalIP)
				}
			}
		}
	default:
		config.LoadBalancerName = fmt.Sprintf("k8s_lb_%s_%s_%s_%s", cluster, service.Namespace, service.Name, util.GetFirstUID(string(service.UID)))
		config.Policy = Exclusive
	}

	config.listenerName = fmt.Sprintf("listener_%s_%s_", service.Namespace, service.Name)
	config.sgName = config.LoadBalancerName

	return config, nil
}
