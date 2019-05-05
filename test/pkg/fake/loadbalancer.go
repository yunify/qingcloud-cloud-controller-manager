package fake

import (
	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/loadbalance"
	corev1 "k8s.io/api/core/v1"
)

type FakeQingCloudLoadBalancer struct {
}

func (f *FakeQingCloudLoadBalancer) LoadQcLoadBalancer() error {
	panic("not implemented")
}

func (f *FakeQingCloudLoadBalancer) Start() error {
	panic("not implemented")
}

func (f *FakeQingCloudLoadBalancer) Stop() error {
	panic("not implemented")
}

func (f *FakeQingCloudLoadBalancer) LoadListeners() error {
	panic("not implemented")
}

func (f *FakeQingCloudLoadBalancer) GetListeners() []*loadbalance.Listener {
	panic("not implemented")
}

func (f *FakeQingCloudLoadBalancer) LoadSecurityGroup() error {
	panic("not implemented")
}

func (f *FakeQingCloudLoadBalancer) EnsureLoadBalancerSecurityGroup() error {
	panic("not implemented")
}

func (f *FakeQingCloudLoadBalancer) NeedResize() bool {
	panic("not implemented")
}

func (f *FakeQingCloudLoadBalancer) NeedChangeIP() (yes bool, toadd []string, todelete []string) {
	panic("not implemented")
}

func (f *FakeQingCloudLoadBalancer) EnsureEIP() error {
	panic("not implemented")
}

func (f *FakeQingCloudLoadBalancer) CreateQingCloudLB() error {
	panic("not implemented")
}

func (f *FakeQingCloudLoadBalancer) Resize() error {
	panic("not implemented")
}

func (f *FakeQingCloudLoadBalancer) UpdateQingCloudLB() error {
	panic("not implemented")
}

func (f *FakeQingCloudLoadBalancer) AssociateEipToLoadBalancer(eips ...string) error {
	panic("not implemented")
}

func (f *FakeQingCloudLoadBalancer) DissociateEipFromLoadBalancer(eips ...string) error {
	panic("not implemented")
}

func (f *FakeQingCloudLoadBalancer) ConfirmQcLoadBalancer() error {
	panic("not implemented")
}

func (f *FakeQingCloudLoadBalancer) GetService() *corev1.Service {
	panic("not implemented")
}

func (f *FakeQingCloudLoadBalancer) DeleteQingCloudLB() error {
	panic("not implemented")
}

func (f *FakeQingCloudLoadBalancer) NeedUpdate() bool {
	panic("not implemented")
}

func (f *FakeQingCloudLoadBalancer) GenerateK8sLoadBalancer() error {
	panic("not implemented")
}

func (f *FakeQingCloudLoadBalancer) GetNodesInstanceIDs() []string {
	panic("not implemented")
}

func (f *FakeQingCloudLoadBalancer) ClearNoUseListener() error {
	panic("not implemented")
}
