package loadbalance

import (
	"fmt"

	qcservice "github.com/yunify/qingcloud-sdk-go/service"
	"k8s.io/klog"
)

type Backend struct {
	lbapi  *qcservice.LoadBalancerService
	Name   string
	Spec   BackendSpec
	Status *qcservice.LoadBalancerBackend
}

type BackendSpec struct {
	listener   *Listener
	Weight     int
	Port       int
	InstanceID string
}

type BackendList struct {
	lbapi    *qcservice.LoadBalancerService
	Listener *Listener
	Items    []*Backend
}

func NewBackendList(lb *LoadBalancer, listener *Listener) *BackendList {
	list := make([]*Backend, 0)
	instanceIDs := lb.GetNodesInstanceIDs()
	for _, instance := range instanceIDs {
		b := &Backend{
			lbapi: lb.GetLBAPI(),
			Name:  fmt.Sprintf("backend_%s_%s", listener.Name, instance),
			Spec: BackendSpec{
				listener:   listener,
				Weight:     1,
				Port:       listener.NodePort,
				InstanceID: instance,
			},
		}
		list = append(list, b)
	}
	return &BackendList{
		lbapi:    lb.GetLBAPI(),
		Listener: listener,
		Items:    list,
	}
}

func (b *Backend) toQcBackendInput() *qcservice.LoadBalancerBackend {
	return &qcservice.LoadBalancerBackend{
		ResourceID:              &b.Spec.InstanceID,
		LoadBalancerBackendName: &b.Name,
		Port:                    &b.Spec.Port,
	}
}

func (b *Backend) LoadQcBackend() error {
	input := &qcservice.DescribeLoadBalancerBackendsInput{
		LoadBalancerListener: b.Spec.listener.Status.LoadBalancerListenerID,
		LoadBalancer:         b.Spec.listener.Status.LoadBalancerID,
	}
	output, err := b.lbapi.DescribeLoadBalancerBackends(input)
	if err != nil {
		klog.Errorf("Failed to get backend %s from qingcloud", b.Name)
		return err
	}
	if len(output.LoadBalancerBackendSet) < 1 {
		return fmt.Errorf("Backend %s Not found", b.Name)
	}
	b.Status = output.LoadBalancerBackendSet[0]
	return nil
}

func deleteBackends(lbapi *qcservice.LoadBalancerService, ids ...*string) error {
	if len(ids) == 0 {
		klog.Warningln("No backends to delete, pls check the inputs")
		return nil
	}
	output, err := lbapi.DeleteLoadBalancerBackends(&qcservice.DeleteLoadBalancerBackendsInput{
		LoadBalancerBackends: ids,
	})
	if *output.RetCode != 0 {
		err := fmt.Errorf("Fail to delete backends of because of '%s'", *output.Message)
		return err
	}
	return err
}

func (b *Backend) DeleteBackend() error {
	if b.Status == nil {
		return fmt.Errorf("Backend %s Not found", b.Name)
	}
	return deleteBackends(b.lbapi, b.Status.LoadBalancerBackendID)
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
	output, err := b.lbapi.AddLoadBalancerBackends(input)
	if err != nil {
		klog.Errorln("Failed to create backends")
		return err
	}
	if *output.RetCode != 0 {
		err := fmt.Errorf("Fail to create backends of listener %s because of '%s'", b.Listener.Name, *output.Message)
		return err
	}

	return nil
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
	input := &qcservice.ModifyLoadBalancerBackendAttributesInput{
		LoadBalancerBackend: b.Status.LoadBalancerBackendID,
		Port:                &b.Spec.Port,
		Weight:              &b.Spec.Weight,
	}
	output, err := b.lbapi.ModifyLoadBalancerBackendAttributes(input)
	if err != nil {
		klog.Errorf("Failed to modify attribute of backend %s", b.Name)
		return err
	}
	if *output.RetCode != 0 {
		err := fmt.Errorf("Fail to modify attribute of backend '%s' because of '%s'", *b.Status.LoadBalancerBackendID, *output.Message)
		return err
	}
	return nil
}
