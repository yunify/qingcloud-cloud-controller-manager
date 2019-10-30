package loadbalance

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/eip"
	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/errors"
	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/executor"
	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/instance"
	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/util"
	qcservice "github.com/yunify/qingcloud-sdk-go/service"
	corev1 "k8s.io/api/core/v1"
	corev1lister "k8s.io/client-go/listers/core/v1"
	"k8s.io/klog"
)

type LoadBalancer struct {
	eipExec eip.EIPHelper
	lbExec  executor.QingCloudLoadBalancerExecutor
	sgExec  executor.QingCloudSecurityGroupExecutor
	//inject service
	nodeLister corev1lister.NodeLister
	listeners  []*Listener

	LoadBalancerSpec
	Status LoadBalancerStatus
}

type LoadBalancerSpec struct {
	service           *corev1.Service
	EIPAllocateSource EIPAllocateSource
	EIPStrategy       EIPStrategy
	EIPs              []string
	Type              int
	TCPPorts          []int
	NodePorts         []int
	Nodes             []*corev1.Node
	Name              string
	clusterName       string
	LoadBalancerID    string
	NetworkType       string
	VxnetID           string
}

type LoadBalancerStatus struct {
	K8sLoadBalancerStatus *corev1.LoadBalancerStatus
	QcLoadBalancer        *qcservice.LoadBalancer
	QcSecurityGroup       *qcservice.SecurityGroup
}

type NewLoadBalancerOption struct {
	EipHelper  eip.EIPHelper
	LbExecutor executor.QingCloudLoadBalancerExecutor
	SgExecutor executor.QingCloudSecurityGroupExecutor
	NodeLister corev1lister.NodeLister

	K8sNodes     []*corev1.Node
	K8sService   *corev1.Service
	Context      context.Context
	ClusterName  string
	SkipCheck    bool
	DefaultVxnet string
}

// NewLoadBalancer create loadbalancer in memory, not in cloud, call 'CreateQingCloudLB' to create a real loadbalancer in qingcloud
func NewLoadBalancer(opt *NewLoadBalancerOption) (*LoadBalancer, error) {
	result := &LoadBalancer{
		eipExec:    opt.EipHelper,
		lbExec:     opt.LbExecutor,
		sgExec:     opt.SgExecutor,
		nodeLister: opt.NodeLister,
	}
	result.Name = GetLoadBalancerName(opt.ClusterName, opt.K8sService)
	t, n := util.GetPortsOfService(opt.K8sService)
	result.TCPPorts = t
	result.NodePorts = n
	result.service = opt.K8sService
	result.Nodes = opt.K8sNodes
	result.clusterName = opt.ClusterName
	if networkType, ok := opt.K8sService.Annotations[ServiceAnnotationLoadBalancerNetworkType]; ok {
		if networkType == NetworkModeInternal {
			result.NetworkType = NetworkModeInternal
		}
	}
	if result.NetworkType == "" {
		result.NetworkType = NetworkModePublic
		if strategy, ok := opt.K8sService.Annotations[ServiceAnnotationLoadBalancerEipStrategy]; ok {
			switch strategy {
			case string(ReuseEIP):
				result.EIPStrategy = ReuseEIP
			case string(ReuseLB):
				result.EIPStrategy = ReuseLB
				result.LoadBalancerID, ok = opt.K8sService.Annotations[ServiceAnnotationLoadBalancerID]
				if !ok {
					return nil, fmt.Errorf("must specify 'service.beta.kubernetes.io/qingcloud-load-balancer-id' if 'service.beta.kubernetes.io/qingcloud-load-balancer-eip-strategy'=reuse-lb")
				}
				//if is reuse-lb, following codes are unneccessary
				return result, nil
			default:
				result.EIPStrategy = Exclusive
			}
		}
		if source, ok := opt.K8sService.Annotations[ServiceAnnotationLoadBalancerEipSource]; ok {
			switch source {
			case string(AllocateOnly):
				result.EIPAllocateSource = AllocateOnly
			case string(UseAvailableOnly):
				result.EIPAllocateSource = UseAvailableOnly
			case string(UseAvailableOrAllocateOne):
				result.EIPAllocateSource = UseAvailableOrAllocateOne
			default:
				result.EIPAllocateSource = ManualSet
			}
		} else {
			result.EIPAllocateSource = ManualSet
		}

		if result.EIPStrategy == ReuseLB {
			result.EIPAllocateSource = ManualSet
		}

		if result.EIPAllocateSource == ManualSet {
			lbEipIds, hasEip := opt.K8sService.Annotations[ServiceAnnotationLoadBalancerEipIds]
			if hasEip {
				result.EIPs = strings.Split(lbEipIds, ",")
			}
		}
	} else {
		//internal type
		if vxnet, ok := opt.K8sService.Annotations[ServiceAnnotationLoadBalancerVxnetID]; ok {
			result.VxnetID = vxnet
		} else {
			result.VxnetID = opt.DefaultVxnet
		}
	}

	lbType := opt.K8sService.Annotations[ServiceAnnotationLoadBalancerType]
	if opt.SkipCheck {
		result.Type = 0
	} else {
		if lbType == "" {
			result.Type = 0
		} else {
			t, err := strconv.Atoi(lbType)
			if err != nil {
				err = fmt.Errorf("Pls spec a valid value of loadBalancer for service %s, accept values are '0-3',err: %s", opt.K8sService.Name, err.Error())
				return nil, err
			}
			if t > 3 || t < 0 {
				err = fmt.Errorf("Pls spec a valid value of loadBalancer for service %s, accept values are '0-3'", opt.K8sService.Name)
				return nil, err
			}
			result.Type = t
		}
	}

	return result, nil
}

// LoadQcLoadBalancer use qingcloud api to get lb in cloud, return err if not found
func (l *LoadBalancer) LoadQcLoadBalancer() (err error) {
	var lb *qcservice.LoadBalancer
	if l.EIPStrategy == ReuseLB {
		lb, err = l.lbExec.GetLoadBalancerByID(l.LoadBalancerID)
	} else {
		lb, err = l.lbExec.GetLoadBalancerByName(l.Name)
	}
	if err != nil {
		return err
	}
	l.Status.QcLoadBalancer = lb
	return nil
}

// LoadListeners use should mannually load listener because sometimes we do not need load entire topology. For example, deletion
func (l *LoadBalancer) LoadListeners() error {
	result := make([]*Listener, 0)
	for _, port := range l.TCPPorts {
		listener, err := NewListener(l, port)
		if err != nil {
			return err
		}
		result = append(result, listener)
	}
	l.listeners = result
	return nil
}

// GetListeners return listeners of this service
func (l *LoadBalancer) GetListeners() []*Listener {
	return l.listeners
}

// LoadSecurityGroup read SecurityGroup in qingcloud related with this service
func (l *LoadBalancer) LoadSecurityGroup() error {
	sg, err := l.sgExec.GetSecurityGroupByName(l.Name)
	if err != nil {
		klog.Errorf("Failed to get security group of lb %s", l.Name)
		return err
	}
	l.Status.QcSecurityGroup = sg
	return nil
}

//EnsureLoadBalancerSecurityGroup will create a SecurityGroup if not exists
func (l *LoadBalancer) EnsureLoadBalancerSecurityGroup() error {
	sg, err := l.sgExec.EnsureSecurityGroup(l.Name)
	if err != nil {
		return err
	}
	l.Status.QcSecurityGroup = sg
	return nil
}

// NeedResize tell us if we should resize the lb in qingcloud
func (l *LoadBalancer) NeedResize() bool {
	if l.Status.QcLoadBalancer == nil {
		return false
	}
	if l.Type != *l.Status.QcLoadBalancer.LoadBalancerType {
		return true
	}
	return false
}

func (l *LoadBalancer) NeedChangeIP() (yes bool, toadd []string, todelete []string) {
	if l.Status.QcLoadBalancer == nil || l.EIPAllocateSource != ManualSet || l.NetworkType == NetworkModeInternal {
		return
	}
	yes = true
	new := strings.Split(l.service.Annotations[ServiceAnnotationLoadBalancerEipIds], ",")
	old := make([]string, 0)
	for _, ip := range l.Status.QcLoadBalancer.Cluster {
		old = append(old, *ip.EIPID)
	}
	for _, ip := range new {
		if util.StringIndex(old, ip) == -1 {
			toadd = append(toadd, ip)
		}
	}
	for _, ip := range old {
		if util.StringIndex(new, ip) == -1 {
			todelete = append(todelete, ip)
		}
	}
	if len(toadd) == 0 && len(todelete) == 0 {
		yes = false
	}
	return
}

func (l *LoadBalancer) EnsureEIP() error {
	if l.NetworkType == NetworkModeInternal {
		return nil
	}
	if l.EIPAllocateSource == AllocateOnly {
		klog.V(2).Infof("Allocate a new ip for lb %s", l.Name)
		eip, err := l.eipExec.AllocateEIP()
		if err != nil {
			return err
		}
		l.EIPs = []string{eip.ID}
	} else if l.EIPAllocateSource == UseAvailableOnly {
		klog.V(2).Infof("Retrieve available ip for lb %s", l.Name)
		eips, err := l.eipExec.GetAvaliableEIPs()
		if err != nil {
			return err
		}
		l.EIPs = []string{eips[0].ID}
	} else if l.EIPAllocateSource == UseAvailableOrAllocateOne {
		klog.V(2).Infof("Retrieve available ip or allocate a new ip for lb %s", l.Name)
		eip, err := l.eipExec.GetAvaliableOrAllocateEIP()
		if err != nil {
			return err
		}
		l.EIPs = []string{eip.ID}
	} else {
		if len(l.EIPs) == 0 {
			klog.V(3).Infof("Current service annotation %+v", l.service.Annotations)
			return fmt.Errorf("Must specify a eip on service %s, current eip source :%s", l.service.Name, l.EIPAllocateSource)
		}
	}
	klog.V(2).Infof("Will use eip %s for lb %s", l.EIPs, l.Name)
	return nil
}

func (l *LoadBalancer) EnsureQingCloudLB() error {
	err := l.LoadQcLoadBalancer()
	if err != nil {
		if errors.IsResourceNotFound(err) && l.EIPStrategy != ReuseLB {
			err = l.CreateQingCloudLB()
			if err != nil {
				klog.Errorf("Failed to create lb in qingcloud of service %s", l.service.Name)
				return err
			}
			return nil
		}
		return err
	}
	err = l.UpdateQingCloudLB()
	if err != nil {
		klog.Errorf("Failed to update lb %s in qingcloud of service %s", l.Name, l.service.Name)
		return err
	}
	return l.GenerateK8sLoadBalancer()
}

// CreateQingCloudLB do create a lb in qingcloud
func (l *LoadBalancer) CreateQingCloudLB() error {

	err := l.EnsureLoadBalancerSecurityGroup()
	if err != nil {
		return err
	}
	createInput := &qcservice.CreateLoadBalancerInput{
		LoadBalancerType: &l.Type,
		LoadBalancerName: &l.Name,
		SecurityGroup:    l.Status.QcSecurityGroup.SecurityGroupID,
	}

	if l.NetworkType == NetworkModePublic {
		err := l.EnsureEIP()
		if err != nil {
			return err
		}
		createInput.EIPs = qcservice.StringSlice(l.EIPs)
	} else {
		createInput.VxNet = &l.VxnetID
	}
	lb, err := l.lbExec.Create(createInput)
	if err != nil {
		klog.Errorf("Failed to create a lb %s in qingcloud", l.Name)
		return err
	}
	l.Status.QcLoadBalancer = lb
	err = l.LoadListeners()
	if err != nil {
		klog.Errorf("Failed to generate listener of loadbalancer %s", l.Name)
		return err
	}
	for _, listener := range l.listeners {
		err = listener.CreateQingCloudListenerWithBackends()
		if err != nil {
			klog.Errorf("Failed to create listener %s of loadbalancer %s", listener.Name, l.Name)
			return err
		}
	}
	err = l.lbExec.Confirm(*lb.LoadBalancerID)
	if err != nil {
		klog.Errorf("Failed to make loadbalancer %s go into effect", l.Name)
		return err
	}
	l.GenerateK8sLoadBalancer()
	klog.V(1).Infof("Loadbalancer %s created succeefully", l.Name)
	return nil
}

// UpdateQingCloudLB update some attrs of qingcloud lb
func (l *LoadBalancer) UpdateQingCloudLB() error {
	if l.Status.QcLoadBalancer == nil {
		klog.Warningf("Nothing can do before loading qingcloud loadBalancer %s", l.Name)
		return nil
	}
	lbid := *l.Status.QcLoadBalancer.LoadBalancerID
	if l.EIPStrategy != ReuseLB {
		if l.NeedResize() {
			klog.V(2).Infof("Detect lb size changed, begin to resize the lb %s", l.Name)
			err := l.lbExec.Resize(*l.Status.QcLoadBalancer.LoadBalancerID, l.Type)
			if err != nil {
				klog.Errorf("Failed to resize lb %s", l.Name)
				return err
			}
		}

		if yes, toadd, todelete := l.NeedChangeIP(); yes {
			klog.V(2).Infof("Adding eips %s to and deleting %s from lb %s", toadd, todelete, l.Name)
			err := l.lbExec.AssociateEip(lbid, toadd...)
			if err != nil {
				klog.Errorf("Failed to add eips %s to lb %s", toadd, l.Name)
				return err
			}
			err = l.lbExec.DissociateEip(lbid, todelete...)
			if err != nil {
				klog.Errorf("Failed to add eips %s to lb %s", todelete, l.Name)
				return err
			}
		}

		if l.NeedUpdate() {
			modifyInput := &qcservice.ModifyLoadBalancerAttributesInput{
				LoadBalancerName: &l.Name,
				LoadBalancer:     l.Status.QcLoadBalancer.LoadBalancerID,
			}
			err := l.lbExec.Modify(modifyInput)
			if err != nil {
				klog.Errorf("Failed to update lb %s in qingcloud", l.Name)
				return err
			}
		}
	}
	err := l.LoadListeners()
	if err != nil {
		klog.Errorf("Failed to generate listener of loadbalancer %s", l.Name)
		return err
	}
	for _, listener := range l.listeners {
		err = listener.UpdateQingCloudListener()
		if err != nil {
			klog.Errorf("Failed to create/update listener %s of loadbalancer %s", listener.Name, l.Name)
			return err
		}
	}
	klog.V(2).Infoln("Clear useless listeners")
	err = l.ClearNoUseListener()
	if err != nil {
		klog.Errorf("Failed to clear listeners of service %s", l.service.Name)
		return err
	}
	err = l.lbExec.Confirm(*l.Status.QcLoadBalancer.LoadBalancerID)
	if err != nil {
		klog.Errorf("Failed to make loadbalancer %s go into effect", l.Name)
		return err
	}
	return nil
}

// GetService return service of this loadbalancer
func (l *LoadBalancer) GetService() *corev1.Service {
	return l.service
}

func (l *LoadBalancer) deleteSecurityGroup() error {
	if l.Status.QcLoadBalancer != nil {
		return l.sgExec.Delete(*l.Status.QcLoadBalancer.SecurityGroupID)
	}
	err := l.LoadSecurityGroup()
	if err != nil {
		if errors.IsResourceNotFound(err) {
			return nil
		}
		klog.Errorf("Failed to load sg of lb %s", l.Name)
		return err
	}
	return l.sgExec.Delete(*l.Status.QcSecurityGroup.SecurityGroupID)
}

func (l *LoadBalancer) deleteListenersOnlyIfOK() (bool, error) {
	if l.Status.QcLoadBalancer == nil {
		return false, nil
	}
	listeners, err := l.lbExec.GetListenersOfLB(*l.Status.QcLoadBalancer.LoadBalancerID, "")
	if err != nil {
		klog.Errorf("Failed to check current listeners of lb %s", l.Name)
		return false, err
	}
	prefix := GetListenerPrefix(l.service)
	toDelete := make([]*qcservice.LoadBalancerListener, 0)
	isUsedByAnotherSevice := false
	for _, listener := range listeners {
		if !strings.HasPrefix(*listener.LoadBalancerListenerName, prefix) {
			isUsedByAnotherSevice = true
		} else {
			toDelete = append(toDelete, listener)
		}
	}
	if l.EIPStrategy == ReuseLB {
		isUsedByAnotherSevice = true
	}
	if isUsedByAnotherSevice {
		for _, listener := range toDelete {
			err = l.lbExec.DeleteListener(*listener.LoadBalancerListenerID)
			if err != nil {
				klog.Errorf("Failed to delete listener %s", *listener.LoadBalancerListenerName)
				return false, err
			}
		}
		err = l.lbExec.Confirm(*l.Status.QcLoadBalancer.LoadBalancerID)
		if err != nil {
			klog.Errorf("Failed to confirm listeners deleted")
			return false, err
		}
	}
	return isUsedByAnotherSevice, nil
}

func (l *LoadBalancer) DeleteQingCloudLB() error {
	if l.Status.QcLoadBalancer == nil {
		err := l.LoadQcLoadBalancer()
		if err != nil {
			if errors.IsResourceNotFound(err) {
				klog.V(1).Infof("Cannot find the lb %s in cloud, maybe is deleted", l.Name)
				err = l.deleteSecurityGroup()
				if err != nil {
					klog.Errorf("Failed to delete SecurityGroup of lb %s ", l.Name)
					return err
				}
				return nil
			}
			return err
		}
	}
	ok, err := l.deleteListenersOnlyIfOK()
	if err != nil {
		return err
	}
	if ok {
		klog.Infof("Detect lb %s is used by another service, delete listeners only", l.Name)
		return nil
	}
	var ip *qcservice.EIP
	if l.NetworkType == NetworkModePublic {
		//record eip id before deleting
		ip = l.Status.QcLoadBalancer.Cluster[0]
	}
	err = l.lbExec.Delete(*l.Status.QcLoadBalancer.LoadBalancerID)
	if err != nil {
		klog.Errorf("Failed to excute deletion of lb %s", *l.Status.QcLoadBalancer.LoadBalancerName)
		return err
	}

	err = l.deleteSecurityGroup()
	if err != nil {
		klog.Errorf("Failed to delete SecurityGroup of lb %s err '%s' ", l.Name, err)
		return err
	}

	if l.NetworkType == NetworkModePublic && l.EIPAllocateSource != ManualSet && *ip.EIPName == eip.AllocateEIPName {
		klog.V(2).Infof("Detect eip %s of lb %s is allocated, release it", *ip.EIPID, l.Name)
		err := l.eipExec.ReleaseEIP(*ip.EIPID)
		if err != nil {
			klog.Errorf("Fail to release  eip %s of lb %s err '%s' ", *ip.EIPID, l.Name, err)
		}
	}
	klog.Infof("Successfully delete loadBalancer '%s'", l.Name)
	return nil
}

// NeedUpdate tell us whether an update to loadbalancer is needed
func (l *LoadBalancer) NeedUpdate() bool {
	if l.Status.QcLoadBalancer == nil {
		return false
	}
	if l.Name != *l.Status.QcLoadBalancer.LoadBalancerName {
		return true
	}
	return false
}

// GenerateK8sLoadBalancer get a corev1.LoadBalancerStatus for k8s
func (l *LoadBalancer) GenerateK8sLoadBalancer() error {
	if l.Status.QcLoadBalancer == nil {
		err := l.LoadQcLoadBalancer()
		if err != nil {
			klog.V(1).Infof("Failed to load qc loadbalance of %s", l.Name)
			return err
		}
	}
	status := &corev1.LoadBalancerStatus{}

	if l.NetworkType == NetworkModeInternal {
		for _, ip := range l.Status.QcLoadBalancer.PrivateIPs {
			status.Ingress = append(status.Ingress, corev1.LoadBalancerIngress{IP: *ip})
		}
	} else {
		for _, eip := range l.Status.QcLoadBalancer.Cluster {
			status.Ingress = append(status.Ingress, corev1.LoadBalancerIngress{IP: *eip.EIPAddr})
		}
		for _, ip := range l.Status.QcLoadBalancer.EIPs {
			status.Ingress = append(status.Ingress, corev1.LoadBalancerIngress{IP: *ip.EIPAddr})
		}
	}

	if len(status.Ingress) == 0 {
		return fmt.Errorf("Have no ip yet")
	}
	l.Status.K8sLoadBalancerStatus = status
	return nil
}

// GetNodesInstanceIDs return resource ids for listener to create backends
func (l *LoadBalancer) GetNodesInstanceIDs() []string {
	if len(l.Nodes) == 0 {
		return nil
	}
	result := make([]string, 0)
	for _, node := range l.Nodes {
		result = append(result, instance.NodeNameToInstanceID(node.Name, l.nodeLister))
	}
	return result
}

// ClearNoUseListener delete uneccassary listeners in qingcloud, used when service ports changed
func (l *LoadBalancer) ClearNoUseListener() error {
	if l.Status.QcLoadBalancer == nil {
		return nil
	}
	listeners, err := l.lbExec.GetListenersOfLB(*l.Status.QcLoadBalancer.LoadBalancerID, GetListenerPrefix(l.service))
	if err != nil {
		if errors.IsResourceNotFound(err) {
			return nil
		}
		klog.Errorf("Failed to get qingcloud listeners of lb %s", l.Name)
		return err
	}

	for _, listener := range listeners {
		if util.IntIndex(l.TCPPorts, *listener.ListenerPort) == -1 {
			err := l.lbExec.DeleteListener(*listener.LoadBalancerListenerID)
			if err != nil {
				klog.Errorf("Failed to delete listener %s", *listener.LoadBalancerListenerName)
				return err
			}
		}
	}
	return nil
}

/// -----Shared  functions-------

// GetLoadBalancerName generate lb name for each service. The name of a service is fixed and predictable
func GetLoadBalancerName(clusterName string, service *corev1.Service) string {
	defaultName := fmt.Sprintf("k8s_lb_%s_%s_%s_%s", clusterName, service.Namespace, service.Name, util.GetFirstUID(string(service.UID)))
	annotation := service.GetAnnotations()
	if annotation == nil {
		return defaultName
	}
	if strategy, ok := annotation[ServiceAnnotationLoadBalancerEipStrategy]; ok {
		if strategy == string(ReuseEIP) {
			return fmt.Sprintf("k8s_lb_%s_%s", clusterName, annotation[ServiceAnnotationLoadBalancerEipIds])
		}
	}
	return defaultName
}
