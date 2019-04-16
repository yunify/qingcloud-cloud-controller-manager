package qingcloud

import (
	"context"
	"errors"
	"fmt"

	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/instance"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	cloudprovider "k8s.io/cloud-provider"
	"k8s.io/klog"
)

var _ cloudprovider.Instances = &QingCloud{}

// Instances returns an implementation of Instances for QingCloud.
func (qc *QingCloud) Instances() (cloudprovider.Instances, bool) {
	return qc, true
}

func (qc *QingCloud) newInstance(name string) *instance.Instance {
	return instance.NewInstance(qc.instanceService, qc.nodeInformer.Lister(), name)
}

// NodeAddresses returns the addresses of the specified instance.
// TODO(roberthbailey): This currently is only used in such a way that it
// returns the address of the calling instance. We should do a rename to
// make this clearer.
func (qc *QingCloud) NodeAddresses(_ context.Context, name types.NodeName) ([]v1.NodeAddress, error) {
	return qc.newInstance(string(name)).GetK8sAddress()
}

func (qc *QingCloud) NodeAddressesByProviderID(_ context.Context, providerID string) ([]v1.NodeAddress, error) {
	if providerID == "" {
		err := fmt.Errorf("Call NodeAddresses with an empty ProviderID")
		return nil, err
	}
	ins := qc.newInstance(providerID)
	err := ins.LoadQcInstanceByID(providerID)
	if err != nil {
		klog.Errorf("Failed to load QingCloud instance of %s", providerID)
		return nil, err
	}
	return ins.GetK8sAddress()
}

func (qc *QingCloud) InstanceID(_ context.Context, nodeName types.NodeName) (string, error) {
	return instance.NodeNameToInstanceID(string(nodeName), qc.nodeInformer.Lister()), nil
}

func (qc *QingCloud) InstanceType(ctx context.Context, name types.NodeName) (string, error) {
	ins := qc.newInstance(string(name))
	err := ins.LoadQcInstance()
	if err != nil {
		klog.Errorf("Failed to load QingCloud instance of %s", name)
		return "", err
	}
	return *ins.Status.InstanceType, nil
}

func (qc *QingCloud) InstanceTypeByProviderID(ctx context.Context, providerID string) (string, error) {
	ins := qc.newInstance(providerID)
	err := ins.LoadQcInstanceByID(providerID)
	if err != nil {
		klog.Errorf("Failed to load QingCloud instance of %s", providerID)
		return "", err
	}
	return *ins.Status.InstanceType, nil
}

// AddSSHKeyToAllInstances adds an SSH public key as a legal identity for all instances.
// The method is currently only used in gce.
func (qc *QingCloud) AddSSHKeyToAllInstances(ctx context.Context, user string, keyData []byte) error {
	return errors.New("Unimplemented")
}

func (qc *QingCloud) CurrentNodeName(ctx context.Context, hostname string) (types.NodeName, error) {
	return types.NodeName(hostname), nil
}

func (qc *QingCloud) InstanceExistsByProviderID(ctx context.Context, providerID string) (bool, error) {
	ins := qc.newInstance(providerID)
	err := ins.LoadQcInstanceByID(providerID)
	if err != nil {
		klog.Errorf("Failed to load QingCloud instance of %s", providerID)
		return false, err
	}
	if *ins.Status.Status == "terminated" || *ins.Status.Status == "ceased" {
		return false, nil
	}
	return true, nil
}

func (qc *QingCloud) InstanceShutdownByProviderID(ctx context.Context, providerID string) (bool, error) {
	ins := qc.newInstance(providerID)
	err := ins.LoadQcInstanceByID(providerID)
	if err != nil {
		klog.Errorf("Failed to load QingCloud instance of %s", providerID)
		return false, err
	}
	if *ins.Status.Status == "stopped" || *ins.Status.Status == "suspended" {
		return true, nil
	}
	return false, err
}
