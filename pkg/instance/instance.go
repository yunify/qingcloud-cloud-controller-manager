package instance

import (
	"fmt"

	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/qcapiwrapper"
	qcservice "github.com/yunify/qingcloud-sdk-go/service"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	corev1lister "k8s.io/client-go/listers/core/v1"
	cloudprovider "k8s.io/cloud-provider"
	"k8s.io/klog"
)

const (
	NodeAnnotationInstanceID = "node.beta.kubernetes.io/instance-id"
)

type Instance struct {
	Name        string
	instanceApi *qcservice.InstanceService
	nodeLister  corev1lister.NodeLister
	Status      *qcservice.Instance
}

type InstanceSpec struct {
}

func NewInstance(qcapi *qcapiwrapper.QingcloudAPIWrapper, nodeLister corev1lister.NodeLister, name string) *Instance {
	return &Instance{
		Name:        name,
		instanceApi: qcapi.InstanceService,
		nodeLister:  nodeLister,
	}
}

func (i *Instance) GetInstanceID() string {
	return NodeNameToInstanceID(i.Name, i.nodeLister)
}

func (i *Instance) LoadQcInstance() error {
	id := i.GetInstanceID()
	return i.LoadQcInstanceByID(id)
}

func (i *Instance) LoadQcInstanceByID(id string) error {
	status := []*string{qcservice.String("pending"), qcservice.String("running"), qcservice.String("stopped")}
	verbose := qcservice.Int(1)
	output, err := i.instanceApi.DescribeInstances(&qcservice.DescribeInstancesInput{
		Instances: []*string{&id},
		Status:    status,
		Verbose:   verbose,
	})
	if err != nil {
		return err
	}
	if len(output.InstanceSet) == 0 {
		return cloudprovider.InstanceNotFound
	}
	i.Status = output.InstanceSet[0]
	return nil
}

func (i *Instance) GetK8sAddress() ([]v1.NodeAddress, error) {
	if i.Status == nil {
		err := i.LoadQcInstance()
		if err != nil {
			klog.Errorf("error getting instance '%v'", i.Name)
			return nil, err
		}
	}
	addrs := []v1.NodeAddress{}
	for _, vxnet := range i.Status.VxNets {
		// vxnet.Role 1 main nic, 0 slave nic. skip slave nic for hostnic cni plugin
		if vxnet.PrivateIP != nil && *vxnet.PrivateIP != "" && *vxnet.Role == 1 {
			addrs = append(addrs, v1.NodeAddress{Type: v1.NodeInternalIP, Address: *vxnet.PrivateIP})
		}
	}

	if i.Status.EIP != nil && i.Status.EIP.EIPAddr != nil && *i.Status.EIP.EIPAddr != "" {
		addrs = append(addrs, v1.NodeAddress{Type: v1.NodeExternalIP, Address: *i.Status.EIP.EIPAddr})
	}
	if len(addrs) == 0 {
		err := fmt.Errorf("The instance %s maybe broken because it has no ip", *i.Status.InstanceID)
		return nil, err
	}
	return addrs, nil
}

// Make sure qingcloud instance hostname or override-hostname (if provided) is equal to InstanceId
// Recommended to use override-hostname
func NodeNameToInstanceID(name string, nodeLister corev1lister.NodeLister) string {
	node, err := nodeLister.Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			return name
		}
		klog.Errorf("Failed to get instance id of node %s, err:", name)
		return ""
	}
	if instanceid, ok := node.GetAnnotations()[NodeAnnotationInstanceID]; ok {
		return instanceid
	}
	return node.Name
}
