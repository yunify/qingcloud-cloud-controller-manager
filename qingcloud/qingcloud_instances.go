package qingcloud

// See https://docs.qingcloud.com/api/instance/index.html

import (
	"errors"

	"github.com/golang/glog"
	qcservice "github.com/yunify/qingcloud-sdk-go/service"
	"k8s.io/kubernetes/pkg/cloudprovider"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/kubernetes/pkg/api/v1"
)

// NodeAddresses returns the addresses of the specified instance.
func (qc *QingCloud) NodeAddresses(nodeName types.NodeName) ([]v1.NodeAddress, error) {
	glog.V(4).Infof("NodeAddresses(%v) called", nodeName)

	ins, err := qc.GetInstanceByID(NodeNameToInstanceID(nodeName))
	if err != nil {
		glog.Errorf("error getting instance '%q': %v", nodeName, err)
		return nil, err
	}

	addrs := []v1.NodeAddress{}
	for _, vxnet := range ins.VxNets {
		if vxnet.PrivateIP != nil && *vxnet.PrivateIP != "" {
			addrs = append(addrs, v1.NodeAddress{Type: v1.NodeInternalIP, Address: *vxnet.PrivateIP})
		}
	}
	if ins.EIP != nil && ins.EIP.EIPAddr != nil && *ins.EIP.EIPAddr != "" {
		addrs = append(addrs, v1.NodeAddress{Type: v1.NodeExternalIP, Address: *ins.EIP.EIPAddr})
	}

	glog.V(4).Infof("NodeAddresses: %v, %v", nodeName, addrs)
	return addrs, nil
}

// ExternalID returns the cloud provider ID of the specified instance (deprecated).
// Note that if the instance does not exist or is no longer running, we must return ("", cloudprovider.InstanceNotFound)
func (qc *QingCloud) ExternalID(nodeName types.NodeName) (string, error) {
	glog.V(4).Infof("ExternalID(%v) called", nodeName)

	ins, err := qc.GetInstanceByID(NodeNameToInstanceID(nodeName))
	if err != nil {
		return "", err
	}

	return *ins.InstanceID, nil
}

// InstanceID returns the cloud provider ID of the specified instance.
func (qc *QingCloud) InstanceID(nodeName types.NodeName) (string, error) {
	glog.V(4).Infof("InstanceID(%v) called", nodeName)
	return NodeNameToInstanceID(nodeName), nil
}

// InstanceType returns the type of the specified instance.
func (qc *QingCloud) InstanceType(name types.NodeName) (string, error) {
	glog.V(4).Infof("InstanceType(%v) called", name)

	ins, err := qc.GetInstanceByID(NodeNameToInstanceID(name))
	if err != nil {
		return "", err
	}

	return *ins.InstanceType, nil
}

// List lists instances that match 'filter' which is a regular expression which must match the entire instance name (fqdn)
func (qc *QingCloud) List(filter string) ([]types.NodeName, error) {
	glog.V(4).Infof("List(%v) called", filter)

	instances, err := qc.getInstancesByFilter(filter)
	if err != nil {
		glog.Errorf("error getting instances by filter '%s': %v", filter, err)
		return nil, err
	}
	result := []types.NodeName{}
	for _, ins := range instances {
		result = append(result, types.NodeName(*ins.InstanceID))
	}

	glog.V(4).Infof("List instances: %v", result)

	return result, nil
}

func (qc *QingCloud) NodeAddressesByProviderID(providerId string) ([]v1.NodeAddress, error){
	//TODO
	return nil, nil
}
func (qc *QingCloud) InstanceTypeByProviderID(providerID string) (string, error){
	//TODO
	return "", nil
}

// AddSSHKeyToAllInstances adds an SSH public key as a legal identity for all instances.
// The method is currently only used in gce.
func (qc *QingCloud) AddSSHKeyToAllInstances(user string, keyData []byte) error {
	return errors.New("Unimplemented")
}

// CurrentNodeName returns the name of the node we are currently running on
// On most clouds (e.g. GCE) this is the hostname, so we provide the hostname
func (qc *QingCloud) CurrentNodeName(hostname string) (types.NodeName, error) {
	return types.NodeName(hostname), nil
}

func (qc *QingCloud) GetSelf() (*qcservice.Instance) {
	return qc.selfInstance
}

// GetInstanceByID get instance.Instance by instanceId
func (qc *QingCloud) GetInstanceByID(instanceID string) (*qcservice.Instance, error) {

	if qc.selfInstance != nil && *qc.selfInstance.InstanceID == instanceID {
		return qc.selfInstance, nil
	}

	status := []*string{qcservice.String("pending"), qcservice.String("running"), qcservice.String("stopped")}
	verbose := qcservice.Int(1)
	output, err := qc.instanceService.DescribeInstances(&qcservice.DescribeInstancesInput{
		Instances: []*string{&instanceID},
		Status:    status,
		Verbose:   verbose,
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
