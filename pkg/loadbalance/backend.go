package loadbalance

import (
	"fmt"

	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/executor"
	qcservice "github.com/yunify/qingcloud-sdk-go/service"
	"k8s.io/klog"
)

type Backend struct {
	backendExec executor.QingCloudListenerBackendExecutor
	Name        string
	Spec        BackendSpec
	Status      *qcservice.LoadBalancerBackend
}

type BackendSpec struct {
	Listener   *Listener
	Weight     int
	Port       int
	InstanceID string
}

type BackendList struct {
	backendExec executor.QingCloudListenerBackendExecutor
	Listener    *Listener
	Items       []*Backend
}

func NewBackendList(lb *LoadBalancer, listener *Listener) *BackendList {
	list := make([]*Backend, 0)
	instanceIDs := lb.GetNodesInstanceIDs()
	exec := lb.lbExec.(executor.QingCloudListenerBackendExecutor)
	for _, instance := range instanceIDs {
		b := &Backend{
			backendExec: exec,
			Name:        fmt.Sprintf("backend_%s_%s", listener.Name, instance),
			Spec: BackendSpec{
				Listener:   listener,
				Weight:     1,
				Port:       listener.NodePort,
				InstanceID: instance,
			},
		}
		list = append(list, b)
	}
	return &BackendList{
		backendExec: exec,
		Listener:    listener,
		Items:       list,
	}
}

func (b *Backend) toQcBackendInput() *qcservice.LoadBalancerBackend {
	return &qcservice.LoadBalancerBackend{
		ResourceID:              &b.Spec.InstanceID,
		LoadBalancerBackendName: &b.Name,
		Port:                    &b.Spec.Port,
	}
}

func (b *Backend) DeleteBackend() error {
	if b.Status == nil {
		return fmt.Errorf("Backend %s Not found", b.Name)
	}
	return b.backendExec.DeleteBackends(*b.Status.LoadBalancerBackendID)
}

func (b *BackendList) CreateBackends() error {
	if len(b.Items) == 0 {
		return fmt.Errorf("No backends to create")
	}
	backends := make([]*qcservice.LoadBalancerBackend, 0)
	for _, item := range b.Items {
		backends = append(backends, item.toQcBackendInput())
	}
	input := &qcservice.AddLoadBalancerBackendsInput{
		LoadBalancerListener: b.Listener.Status.LoadBalancerListenerID,
		Backends:             backends,
	}
	return b.backendExec.CreateBackends(input)
}

func (b *Backend) LoadQcBackend() error {
	backends, err := b.backendExec.GetBackendsOfListener(*b.Spec.Listener.Status.LoadBalancerListenerID)
	if err != nil {
		return err
	}
	for _, item := range backends {
		if *item.ResourceID == b.Spec.InstanceID {
			b.Status = item
			return nil
		}
	}
	return fmt.Errorf("Cannot find any backend with instance id %s in listener %s", b.Spec.InstanceID, *b.Spec.Listener.Status.LoadBalancerListenerID)
}

func (b *Backend) NeedUpdate() bool {
	if b.Status == nil {
		err := b.LoadQcBackend()
		if err != nil {
			klog.Errorf("Unable to get qc backend %s when check updatable", b.Name)
			return false
		}
	}
	if b.Spec.Weight != *b.Status.Weight || b.Spec.Port != *b.Status.Port {
		return true
	}
	return false
}

func (b *Backend) UpdateBackend() error {
	if !b.NeedUpdate() {
		return nil
	}
	return b.backendExec.ModifyBackend(*b.Status.LoadBalancerBackendID, b.Spec.Weight, b.Spec.Port)
}
