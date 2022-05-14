package apis

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type EIPSpec struct {
	EIPName   *string `json:"eip_name" name:"eip_name"`
	Bandwidth *int    `json:"bandwidth" name:"bandwidth"`
}

type EIPStatus struct {
	InUse *bool   `json:"inUse" name:"inUse"`
	EIPID *string `json:"eip_id" name:"eip_id"`
}

type EIP struct {
	metav1.TypeMeta `json:",inline"`
	// Standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	Spec   EIPSpec   `json:"spec,omitempty" protobuf:"bytes,2,opt,name=spec"`
	Status EIPStatus `json:"status,omitempty" protobuf:"bytes,3,opt,name=status"`
}

type LoadBalancerSpec struct {
	LoadBalancerName *string   `json:"loadbalancer_name" name:"loadbalancer_name"`
	EIPs             []*string `json:"eips" name:"eips" location:"params"`

	// LoadBalancerType's available values: 0, 1, 2, 3, 4, 5
	LoadBalancerType *int `json:"loadbalancer_type" name:"loadbalancer_type"`
	NodeCount        *int `json:"node_count" name:"node_count"`

	PrivateIPs []*string `json:"private_ips" name:"private_ips"`
	VxNetID    *string   `json:"vxnet_id" name:"vxnet_id"`

	SecurityGroups *string `json:"securityGroups" name:"securityGroups"`
}

type LoadBalancerStatus struct {
	LoadBalancerID        *string `json:"loadbalancer_id" name:"loadbalancer_id"`
	VIP                   []string
	CreatedEIPs           []*string
	LoadBalancerListeners []LoadBalancerListener
	IsApplied             *int
}

type LoadBalancer struct {
	metav1.TypeMeta `json:",inline"`
	// Standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`
	Spec              LoadBalancerSpec   `json:"spec,omitempty" protobuf:"bytes,2,opt,name=spec"`
	Status            LoadBalancerStatus `json:"status,omitempty" protobuf:"bytes,3,opt,name=status"`
}

type LoadBalancerBackendSpec struct {
	LoadBalancerBackendName *string `json:"loadbalancer_backend_name" name:"loadbalancer_backend_name"`
	Port                    *int    `json:"port" name:"port"`
	ResourceID              *string `json:"resource_id" name:"resource_id"`
	LoadBalancerListenerID  *string `json:"loadbalancer_listener_id" name:"loadbalancer_listener_id"`
}

type LoadBalancerBackendStatus struct {
	LoadBalancerBackendID *string `json:"loadbalancer_backend_id" name:"loadbalancer_backend_id"`
}

type LoadBalancerBackend struct {
	metav1.TypeMeta `json:",inline"`
	// Standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`
	Spec              LoadBalancerBackendSpec   `json:"spec,omitempty" protobuf:"bytes,2,opt,name=spec"`
	Status            LoadBalancerBackendStatus `json:"status,omitempty" protobuf:"bytes,3,opt,name=status"`
}

type LoadBalancerListenerSpec struct {
	BackendProtocol *string `json:"backend_protocol" name:"backend_protocol"`
	// BalanceMode's available values: roundrobin, leastconn, source
	ListenerPort             *int    `json:"listener_port" name:"listener_port"`
	ListenerProtocol         *string `json:"listener_protocol" name:"listener_protocol"`
	LoadBalancerListenerName *string `json:"loadbalancer_listener_name" name:"loadbalancer_listener_name"`
	LoadBalancerID           *string `json:"loadbalancer_id" name:"loadbalancer_id"`
	HealthyCheckMethod       *string `json:"healthy_check_method" name:"healthy_check_method"`
	HealthyCheckOption       *string `json:"healthy_check_option" name:"healthy_check_option"`
}

type LoadBalancerListenerStatus struct {
	LoadBalancerListenerID *string `json:"loadbalancer_listener_id" name:"loadbalancer_listener_id"`
	LoadBalancerBackends   []*LoadBalancerBackend
}

type LoadBalancerListener struct {
	metav1.TypeMeta `json:",inline"`
	// Standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`
	Spec              LoadBalancerListenerSpec   `json:"spec,omitempty" protobuf:"bytes,2,opt,name=spec"`
	Status            LoadBalancerListenerStatus `json:"status,omitempty" protobuf:"bytes,3,opt,name=status"`
}

type SecurityGroupSpec struct {
	SecurityGroupName *string `json:"security_group_name" name:"security_group_name"`
}

type SecurityGroupStatus struct {
	SecurityGroupID      *string `json:"security_group_id" name:"security_group_id"`
	SecurityGroupRuleIDs []*string
	IsApplied            *int `json:"is_applied" name:"is_applied"`
}

type SecurityGroup struct {
	metav1.TypeMeta `json:",inline"`
	// Standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`
	Spec              SecurityGroupSpec   `json:"spec,omitempty" protobuf:"bytes,2,opt,name=spec"`
	Status            SecurityGroupStatus `json:"status,omitempty" protobuf:"bytes,3,opt,name=status"`
}
