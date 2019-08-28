package executor

import (
	qcservice "github.com/yunify/qingcloud-sdk-go/service"
)

type QingCloudListenerExecutor interface {
	GetListenersOfLB(lbid, prefix string) ([]*qcservice.LoadBalancerListener, error)
	GetListenerByID(id string) (*qcservice.LoadBalancerListener, error)
	DeleteListener(lsnid string) error
	CreateListener(*qcservice.AddLoadBalancerListenersInput) (*qcservice.LoadBalancerListener, error)
	ModifyListener(id, balanceMode string) error
	GetListenerByName(lbid, name string) (*qcservice.LoadBalancerListener, error)
}

type QingCloudListenerBackendExecutor interface {
	DeleteBackends(ids ...string) error
	GetBackendsOfListener(lsnid string) ([]*qcservice.LoadBalancerBackend, error)
	GetBackendByID(bid string) (*qcservice.LoadBalancerBackend, error)
	CreateBackends(*qcservice.AddLoadBalancerBackendsInput) error
	ModifyBackend(id string, weight int, port int) error
}

// CloudLoadBalancer do stuff on qingcloud
type QingCloudLoadBalancerExecutor interface {
	// LoadQcLoadBalancer use qingcloud api to get lb in cloud, return err if not found
	GetLoadBalancerByName(name string) (*qcservice.LoadBalancer, error)
	GetLoadBalancerByID(id string) (*qcservice.LoadBalancer, error)
	// Start start loadbalancer in qingcloud
	Start(id string) error
	// Stop stop loadbalancer in qingcloud
	Stop(id string) error
	// CreateQingCloudLB do create a lb in qingcloud
	Create(input *qcservice.CreateLoadBalancerInput) (*qcservice.LoadBalancer, error)

	// Resize change the type of lb in qingcloud
	Resize(id string, newtype int) error
	// UpdateQingCloudLB update some attrs of qingcloud lb

	Modify(input *qcservice.ModifyLoadBalancerAttributesInput) error
	// AssociateEipToLoadBalancer bind the eips to lb in qingcloud
	AssociateEip(lbid string, eips ...string) error
	// DissociateEipFromLoadBalancer unbind the eips from lb in qingcloud
	DissociateEip(lbid string, eips ...string) error
	// ConfirmQcLoadBalancer make sure each operation is taken effects
	Confirm(id string) error

	Delete(id string) error

	GetLBAPI() *qcservice.LoadBalancerService

	QingCloudListenerExecutor
	QingCloudListenerBackendExecutor
}

type QingCloudSecurityGroupExecutor interface {
	// GetSecurityGroupByName return SecurityGroup in qingcloud using name
	GetSecurityGroupByName(name string) (*qcservice.SecurityGroup, error)
	//EnsureSecurityGroup will create a SecurityGroup if not exists
	EnsureSecurityGroup(name string) (*qcservice.SecurityGroup, error)

	GetSecurityGroupByID(id string) (*qcservice.SecurityGroup, error)

	GetSgAPI() *qcservice.SecurityGroupService

	Delete(id string) error

	CreateSecurityGroup(gName string, rules []*qcservice.SecurityGroupRule) (*qcservice.SecurityGroup, error)
}
