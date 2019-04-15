// Copyright 2017 Yunify Inc. All rights reserved.
// Use of this source code is governed by a Apache license
// that can be found in the LICENSE file.

package qingcloud

// See https://docs.qingcloud.com/api/instance/index.html

import (
	"context"
	"errors"

	qcservice "github.com/yunify/qingcloud-sdk-go/service"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	cloudprovider "k8s.io/cloud-provider"
	"k8s.io/klog"
)

const (
	NodeAnnotationInstanceID = "node.beta.kubernetes.io/instance-id"
)

// NodeAddresses returns the addresses of the specified instance.
func (qc *QingCloud) NodeAddresses(ctx context.Context, nodeName types.NodeName) ([]v1.NodeAddress, error) {
	return qc.instanceAddress(qc.NodeNameToInstanceID(nodeName))
}

func (qc *QingCloud) NodeAddressesByProviderID(ctx context.Context, providerId string) ([]v1.NodeAddress, error) {
	return qc.instanceAddress(providerId)
}

func (qc *QingCloud) instanceAddress(instanceID string) ([]v1.NodeAddress, error) {
	ins, err := qc.GetInstanceByID(context.TODO(), instanceID)
	//TODO  cannot get instance if hostname has been changed
	if err != nil {
		klog.Errorf("error getting instance '%v': %v", instanceID, err)
		return nil, err
	}

	addrs := []v1.NodeAddress{}
	for _, vxnet := range ins.VxNets {
		// vxnet.Role 1 main nic, 0 slave nic. skip slave nic for hostnic cni plugin
		if vxnet.PrivateIP != nil && *vxnet.PrivateIP != "" && *vxnet.Role == 1 {
			addrs = append(addrs, v1.NodeAddress{Type: v1.NodeInternalIP, Address: *vxnet.PrivateIP})
		}
	}

	if ins.EIP != nil && ins.EIP.EIPAddr != nil && *ins.EIP.EIPAddr != "" {
		addrs = append(addrs, v1.NodeAddress{Type: v1.NodeExternalIP, Address: *ins.EIP.EIPAddr})
	}
	return addrs, nil
}

// ExternalID returns the cloud provider ID of the specified instance (deprecated).
// Note that if the instance does not exist or is no longer running, we must return ("", cloudprovider.InstanceNotFound)
func (qc *QingCloud) ExternalID(ctx context.Context, nodeName types.NodeName) (string, error) {
	return qc.NodeNameToInstanceID(nodeName), nil
}

// InstanceID returns the cloud provider ID of the specified instance.
func (qc *QingCloud) InstanceID(ctx context.Context, nodeName types.NodeName) (string, error) {
	klog.V(9).Infof("InstanceID(%v) called", nodeName)
	return qc.NodeNameToInstanceID(nodeName), nil
}

// InstanceType returns the type of the specified instance.
func (qc *QingCloud) InstanceType(ctx context.Context, name types.NodeName) (string, error) {
	return qc.instanceType(qc.NodeNameToInstanceID(name))
}

func (qc *QingCloud) InstanceTypeByProviderID(ctx context.Context, providerID string) (string, error) {
	return qc.instanceType(providerID)
}

func (qc *QingCloud) instanceType(instanceID string) (string, error) {
	klog.V(9).Infof("instanceType(%v) called", instanceID)
	ins, err := qc.GetInstanceByID(context.TODO(), instanceID)
	if err != nil {
		return "", err
	}
	return *ins.InstanceType, nil
}

// List lists instances that match 'filter' which is a regular expression which must match the entire instance name (fqdn)
func (qc *QingCloud) List(filter string) ([]types.NodeName, error) {
	klog.V(9).Infof("List(%v) called", filter)

	instances, err := qc.getInstancesByFilter(filter)
	if err != nil {
		klog.Errorf("error getting instances by filter '%s': %v", filter, err)
		return nil, err
	}
	result := []types.NodeName{}
	for _, ins := range instances {
		result = append(result, types.NodeName(*ins.InstanceID))
	}

	klog.V(9).Infof("List instances: %v", result)

	return result, nil
}

// AddSSHKeyToAllInstances adds an SSH public key as a legal identity for all instances.
// The method is currently only used in gce.
func (qc *QingCloud) AddSSHKeyToAllInstances(ctx context.Context, user string, keyData []byte) error {
	return errors.New("Unimplemented")
}

// CurrentNodeName returns the name of the node we are currently running on
// On most clouds (e.g. GCE) this is the hostname, so we provide the hostname
func (qc *QingCloud) CurrentNodeName(ctx context.Context, hostname string) (types.NodeName, error) {
	return types.NodeName(hostname), nil
}

func (qc *QingCloud) GetSelf() *qcservice.Instance {
	return qc.selfInstance
}

// GetInstanceByID get instance.Instance by instanceId
func (qc *QingCloud) GetInstanceByID(ctx context.Context, instanceID string) (*qcservice.Instance, error) {
	if qc.selfInstance != nil && *qc.selfInstance.InstanceID == instanceID {
		return qc.selfInstance, nil
	}

	status := []*string{qcservice.String("pending"), qcservice.String("running"), qcservice.String("stopped")}
	verbose := qcservice.Int(1)
	output, err := qc.instanceService.DescribeInstances(&qcservice.DescribeInstancesInput{
		Instances:     []*string{&instanceID},
		Status:        status,
		Verbose:       verbose,
		IsClusterNode: qcservice.Int(1),
	})
	if err != nil {
		return nil, err
	}
	if len(output.InstanceSet) == 0 {
		return nil, cloudprovider.InstanceNotFound
	}

	return output.InstanceSet[0], nil
}

// List instances that match the filter
func (qc *QingCloud) getInstancesByFilter(filter string) ([]*qcservice.Instance, error) {
	status := []*string{qcservice.String("running"), qcservice.String("stopped")}
	verbose := qcservice.Int(1)
	limit := qcservice.Int(pageLimt)

	instances := []*qcservice.Instance{}

	for i := 0; ; i += pageLimt {
		offset := qcservice.Int(i)
		output, err := qc.instanceService.DescribeInstances(&qcservice.DescribeInstancesInput{
			SearchWord: &filter,
			Status:     status,
			Verbose:    verbose,
			Offset:     offset,
			Limit:      limit,
		})
		if err != nil {
			return nil, err
		}
		if len(output.InstanceSet) == 0 {
			break
		}

		instances = append(instances, output.InstanceSet...)
		if len(instances) >= *output.TotalCount {
			break
		}
	}

	return instances, nil
}

func (qc *QingCloud) InstanceShutdownByProviderID(ctx context.Context, providerID string) (bool, error) {
	//TODO (magicsong)
	return false, nil
}
