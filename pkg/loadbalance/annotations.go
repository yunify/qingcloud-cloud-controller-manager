package loadbalance

import (
	"fmt"
	"strconv"
	"strings"
)

// Policy is the type representing lb shared policy
type Policy string

type EIPAllocateSource string

const (
	// ServiceAnnotationLoadBalancerEipIds is the annotation which specifies a list of eip ids.
	// The ids in list are separated by ',', e.g. "eip-j38f2h3h,eip-ornz2xq7". And this annotation should
	// NOT be used with ServiceAnnotationLoadBalancerVxnetId. Please make sure there is one and only one
	// of them being set
	ServiceAnnotationLoadBalancerEipIds = "service.beta.kubernetes.io/qingcloud-load-balancer-eip-ids"

	// ServiceAnnotationLoadBalancerVxnetId is the annotation which indicates the very vxnet where load
	// balancer resides. This annotation should NOT be used when ServiceAnnotationLoadBalancerEipIds is
	// set.
	ServiceAnnotationLoadBalancerVxnetID         = "service.beta.kubernetes.io/qingcloud-load-balancer-vxnet-id"
	ServiceAnnotationLoadBalancerInternalIP      = "service.beta.kubernetes.io/qingcloud-load-balancer-internal-ip"
	ServiceAnnotationLoadBalancerInternalReuseID = "service.beta.kubernetes.io/qingcloud-load-balancer-reuse-id"
	// ServiceAnnotationLoadBalancerType is the annotation used on the
	// service to indicate that we want a qingcloud loadBalancer type.
	// value "0" means the LB can max support 5000 concurrency connections, it's default type.
	// value "1" means the LB can max support 20000 concurrency connections.
	// value "2" means the LB can max support 40000 concurrency connections.
	// value "3" means the LB can max support 100000 concurrency connections.
	// value "4" means the LB can max support 200000 concurrency connections.
	// value "5" means the LB can max support 500000 concurrency connections.
	ServiceAnnotationLoadBalancerType = "service.beta.kubernetes.io/qingcloud-load-balancer-type"

	// 	ServiceAnnotationLoadBalancerNetworkType represents the network mode of lb, one of "public" or "internal", public is the default mode which needs at least an eip
	ServiceAnnotationLoadBalancerNetworkType = "service.beta.kubernetes.io/qingcloud-load-balancer-network-type"
	// ServiceAnnotationLoadBalancerID is needed when user want to use exsiting lb
	ServiceAnnotationLoadBalancerID = "service.beta.kubernetes.io/qingcloud-load-balancer-id"
	// ServiceAnnotationLoadBalancerPolicy is usd to specify EIP use strategy
	// reuse represent the EIP can be shared with other service which has no port conflict
	// exclusive is the default value, means every service has its own EIP
	ServiceAnnotationLoadBalancerPolicy = "service.beta.kubernetes.io/qingcloud-load-balancer-eip-strategy"

	ServiceAnnotationLoadBalancerEipSource = "service.beta.kubernetes.io/qingcloud-load-balancer-eip-source"

	//ReuseExistingLB  use existing loadbalancer on the cloud
	//We can't change any properties.
	ReuseExistingLB Policy = "reuse-lb"
	// Shared represent the EIP can be shared with other service which has no port conflict
	Shared Policy = "reuse"
	// Exclusive is the default value, means every service has its own EIP
	Exclusive Policy = "exclusive"

	ManualSet                 EIPAllocateSource = "manual"
	UseAvailableOrAllocateOne EIPAllocateSource = "auto"
	UseAvailableOnly          EIPAllocateSource = "use-available"
	AllocateOnly              EIPAllocateSource = "allocate"

	NetworkModePublic   string = "public"
	NetworkModeInternal        = "internal"
)

type AnnotaionConfig struct {
	ScaleType   int
	NetworkType string
	EipIDs      []string
	EIPAllocateSource
	Policy
	InternalIP      string
	InternalReuseID string
	VxnetID         string
	ReuseLBID       string
}

func ParseAnnotation(annotation map[string]string, skipCheck bool) (AnnotaionConfig, error) {
	config := AnnotaionConfig{}
	if strategy, ok := annotation[ServiceAnnotationLoadBalancerPolicy]; ok {
		switch strategy {
		case string(Shared):
			config.Policy = Shared
		case string(ReuseExistingLB):
			config.Policy = ReuseExistingLB
			config.ReuseLBID, ok = annotation[ServiceAnnotationLoadBalancerID]
			if !ok && !skipCheck {
				return config, fmt.Errorf("must specify 'service.beta.kubernetes.io/qingcloud-load-balancer-id' if 'service.beta.kubernetes.io/qingcloud-load-balancer-eip-strategy'=reuse-lb")
			}
			//if is reuse-lb, following codes are unneccessary
			return config, nil
		default:
			config.Policy = Exclusive
		}
	}
	if networkType, ok := annotation[ServiceAnnotationLoadBalancerNetworkType]; ok {
		if networkType == NetworkModeInternal {
			config.NetworkType = NetworkModeInternal
		}
	}
	if config.NetworkType == "" {
		config.NetworkType = NetworkModePublic

		if source, ok := annotation[ServiceAnnotationLoadBalancerEipSource]; ok {
			switch source {
			case string(AllocateOnly):
				config.EIPAllocateSource = AllocateOnly
			case string(UseAvailableOnly):
				config.EIPAllocateSource = UseAvailableOnly
			case string(UseAvailableOrAllocateOne):
				config.EIPAllocateSource = UseAvailableOrAllocateOne
			default:
				config.EIPAllocateSource = ManualSet
			}
		} else {
			config.EIPAllocateSource = ManualSet
		}

		if config.Policy == ReuseExistingLB {
			config.EIPAllocateSource = ManualSet
		}

		if config.EIPAllocateSource == ManualSet {
			lbEipIds, hasEip := annotation[ServiceAnnotationLoadBalancerEipIds]
			if hasEip {
				config.EipIDs = strings.Split(lbEipIds, ",")
			}
		}
	} else {
		//internal type
		if vxnet, ok := annotation[ServiceAnnotationLoadBalancerVxnetID]; ok {
			config.VxnetID = vxnet
		}
		if ip, ok := annotation[ServiceAnnotationLoadBalancerInternalIP]; ok {
			config.InternalIP = ip
		}
		if reuseid, ok := annotation[ServiceAnnotationLoadBalancerInternalReuseID]; ok {
			config.InternalReuseID = reuseid
		}
		if !skipCheck {
			if config.Policy == Shared && config.InternalReuseID == "" && config.InternalIP == "" {
				return config, fmt.Errorf("Must specify reuse-id or internalip if wants to use shared internal lb")
			}
		}
	}

	lbType := annotation[ServiceAnnotationLoadBalancerType]
	if skipCheck {
		config.ScaleType = 0
	} else {
		if lbType == "" {
			config.ScaleType = 0
		} else {
			t, err := strconv.Atoi(lbType)
			if err != nil {
				return config, fmt.Errorf("Pls spec a valid value of loadBalancer type, acceptted values are '0-3',err: %s", err.Error())
			}
			if t > 3 || t < 0 {
				return config, fmt.Errorf("Pls spec a valid value of loadBalancer type , acceptted values are '0-3'")
			}
			config.ScaleType = t
		}
	}
	return config, nil
}
