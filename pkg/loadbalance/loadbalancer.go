package loadbalance

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/eip"

	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/instance"
	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/util"
	qcclient "github.com/yunify/qingcloud-sdk-go/client"
	qcservice "github.com/yunify/qingcloud-sdk-go/service"
	corev1 "k8s.io/api/core/v1"
	corev1lister "k8s.io/client-go/listers/core/v1"
	"k8s.io/klog"
)

var defaultLBSecurityGroupRules = []*qcservice.SecurityGroupRule{
	{
		Priority: qcservice.Int(0),
		Protocol: qcservice.String("icmp"),
		Action:   qcservice.String("accept"),
		Val1:     qcservice.String("8"), //Echo
		Val2:     qcservice.String("0"), //Echo request
		Val3:     nil,
	},
	//allow all tcp port, lb only open listener's port,
	//security group is for limit ip source.
	{
		Priority: qcservice.Int(1),
		Protocol: qcservice.String("tcp"),
		Action:   qcservice.String("accept"),
		Val1:     qcservice.String("1"),
		Val2:     qcservice.String("65535"),
		Val3:     nil,
	},
}

var (
	ErrorLBNotFoundInCloud = fmt.Errorf("Cannot find lb in qingcloud")
	ErrorSGNotFoundInCloud = fmt.Errorf("Cannot find security group in qingcloud")
)

type LoadBalancer struct {
	eip.EIPHelper
	//inject service
	lbapi      *qcservice.LoadBalancerService
	sgapi      *qcservice.SecurityGroupService
	jobapi     *qcservice.JobService
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
}

type LoadBalancerStatus struct {
	K8sLoadBalancerStatus *corev1.LoadBalancerStatus
	QcLoadBalancer        *qcservice.LoadBalancer
	QcSecurityGroup       *qcservice.SecurityGroup
}

type NewLoadBalancerOption struct {
	LoadBalanceApi   *qcservice.LoadBalancerService
	SecurityGroupApi *qcservice.SecurityGroupService
	JobApi           *qcservice.JobService
	EIPApi           *qcservice.EIPService
	UserID           string
	NodeLister       corev1lister.NodeLister
	K8sNodes         []*corev1.Node
	K8sService       *corev1.Service
	Context          context.Context
	ClusterName      string
	SkipCheck        bool
}

// NewLoadBalancer create loadbalancer in memory, not in cloud, call 'CreateQingCloudLB' to create a real loadbalancer in qingcloud
func NewLoadBalancer(opt *NewLoadBalancerOption) (*LoadBalancer, error) {
	result := &LoadBalancer{
		lbapi:      opt.LoadBalanceApi,
		sgapi:      opt.SecurityGroupApi,
		jobapi:     opt.JobApi,
		nodeLister: opt.NodeLister,
	}
	result.Name = GetLoadBalancerName(opt.ClusterName, opt.K8sService)
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
	if strategy, ok := opt.K8sService.Annotations[ServiceAnnotationLoadBalancerEipStrategy]; ok && strategy == string(ReuseEIP) {
		result.EIPStrategy = ReuseEIP
	} else {
		result.EIPStrategy = Exclusive
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
	result.EIPHelper = eip.NewEIPHelperOfQingCloud(eip.NewEIPHelperOfQingCloudOption{
		JobAPI: opt.JobApi,
		EIPAPI: opt.EIPApi,
		UserID: opt.UserID,
	})
	t, n := util.GetPortsOfService(opt.K8sService)
	result.TCPPorts = t
	result.NodePorts = n
	result.service = opt.K8sService
	result.Nodes = opt.K8sNodes
	result.clusterName = opt.ClusterName
	if result.EIPAllocateSource == ManualSet {
		lbEipIds, hasEip := opt.K8sService.Annotations[ServiceAnnotationLoadBalancerEipIds]
		if hasEip {
			result.EIPs = strings.Split(lbEipIds, ",")
		}
	}
	return result, nil
}

// LoadQcLoadBalancer use qingcloud api to get lb in cloud, return err if not found
func (l *LoadBalancer) LoadQcLoadBalancer() error {
	realLb, err := GetLoadBalancerByName(l.lbapi, l.Name)
	if err != nil {
		klog.Errorf("Failed to get lb from Qingcloud")
		return err
	}
	if realLb == nil {
		return ErrorLBNotFoundInCloud
	}
	l.Status.QcLoadBalancer = realLb
	return nil
}

// Start start loadbalancer in qingcloud
func (l *LoadBalancer) Start() error {
	if l.Status.QcLoadBalancer == nil {
		return fmt.Errorf("Should create a loadbalancer before starting")
	}
	klog.V(2).Infof("Starting loadBalancer '%s'", *l.Status.QcLoadBalancer.LoadBalancerID)
	output, err := l.lbapi.StartLoadBalancers(&qcservice.StartLoadBalancersInput{
		LoadBalancers: []*string{l.Status.QcLoadBalancer.LoadBalancerID},
	})
	if err != nil {
		klog.Errorln("Failed to start loadbalancer")
		return err
	}
	return qcclient.WaitJob(l.jobapi, *output.JobID, operationWaitTimeout, waitInterval)
}

// Stop stop loadbalancer in qingcloud
func (l *LoadBalancer) Stop() error {
	if l.Status.QcLoadBalancer == nil {
		return fmt.Errorf("Should create a loadbalancer before stopping")
	}
	klog.V(2).Infof("Stopping loadBalancer '%s'", *l.Status.QcLoadBalancer.LoadBalancerID)
	output, err := l.lbapi.StopLoadBalancers(&qcservice.StopLoadBalancersInput{
		LoadBalancers: []*string{l.Status.QcLoadBalancer.LoadBalancerID},
	})
	if err != nil {
		klog.Errorln("Failed to start loadbalancer")
		return err
	}
	return qcclient.WaitJob(l.jobapi, *output.JobID, operationWaitTimeout, waitInterval)
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
	sg, err := GetSecurityGroupByName(l.sgapi, l.Name)
	if err != nil {
		klog.Errorf("Failed to get security group of lb %s", l.Name)
		return err
	}
	if sg != nil {
		l.Status.QcSecurityGroup = sg
	}
	return nil
}

// Equal is used to tell if we need  a update on this loadbalancer
func (l *LoadBalancer) Equal(a *LoadBalancer) bool {
	if l.Name != a.Name || l.EIPStrategy != a.EIPStrategy || l.Type != a.Type {
		return false
	}
	if !util.TwoArrayEqual(l.TCPPorts, a.TCPPorts) || !util.TwoArrayEqual(l.NodePorts, a.NodePorts) {
		return false
	}
	return true
}

// GetSecurityGroupByName return SecurityGroup in qingcloud using name
func GetSecurityGroupByName(sgaip *qcservice.SecurityGroupService, name string) (*qcservice.SecurityGroup, error) {
	input := &qcservice.DescribeSecurityGroupsInput{SearchWord: &name}
	output, err := sgaip.DescribeSecurityGroups(input)
	if err != nil {
		return nil, err
	}
	if len(output.SecurityGroupSet) == 0 {
		return nil, nil
	}
	for _, sg := range output.SecurityGroupSet {
		if sg.SecurityGroupName != nil && *sg.SecurityGroupName == name {
			return sg, nil
		}
	}
	return nil, nil
}

//EnsureLoadBalancerSecurityGroup will create a SecurityGroup if not exists
func (l *LoadBalancer) EnsureLoadBalancerSecurityGroup() error {
	sg, err := GetSecurityGroupByName(l.sgapi, l.Name)
	if err != nil {
		return err
	}
	if sg != nil {
		l.Status.QcSecurityGroup = sg
	} else {
		sg, err := CreateSecurityGroup(l.sgapi, l.Name, defaultLBSecurityGroupRules)
		if err != nil {
			return err
		}
		l.Status.QcSecurityGroup = sg
	}
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
	if l.Status.QcLoadBalancer == nil {
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
	if l.EIPAllocateSource == AllocateOnly {
		klog.V(2).Infof("Allocate a new ip for lb %s", l.Name)
		eip, err := l.AllocateEIP()
		if err != nil {
			return err
		}
		l.EIPs = []string{eip.ID}
	} else if l.EIPAllocateSource == UseAvailableOnly {
		klog.V(2).Infof("Retrieve available ip for lb %s", l.Name)
		eips, err := l.GetAvaliableEIPs()
		if err != nil {
			return err
		}
		l.EIPs = []string{eips[0].ID}
	} else if l.EIPAllocateSource == UseAvailableOrAllocateOne {
		klog.V(2).Infof("Retrieve available ip or allocate a new ip for lb %s", l.Name)
		eip, err := l.GetAvaliableOrAllocateEIP()
		if err != nil {
			return err
		}
		l.EIPs = []string{eip.ID}
	} else {
		if len(l.EIPs) == 0 {
			return fmt.Errorf("Must specify a eip on service if you want to use lb")
		}
	}
	klog.V(2).Infof("Will use eip %s for lb %s", l.EIPs, l.Name)
	return nil
}

// CreateQingCloudLB do create a lb in qingcloud
func (l *LoadBalancer) CreateQingCloudLB() error {
	err := l.EnsureEIP()
	if err != nil {
		return err
	}
	err = l.EnsureLoadBalancerSecurityGroup()
	if err != nil {
		return err
	}
	output, err := l.lbapi.CreateLoadBalancer(&qcservice.CreateLoadBalancerInput{
		EIPs:             qcservice.StringSlice(l.EIPs),
		LoadBalancerType: &l.Type,
		LoadBalancerName: &l.Name,
		SecurityGroup:    l.Status.QcSecurityGroup.SecurityGroupID,
	})
	if err != nil {
		return err
	}
	klog.V(2).Infof("Waiting for Lb %s starting", l.Name)
	err = l.waitLoadBalancerActive(*output.LoadBalancerID)
	if err != nil {
		klog.Errorf("LoadBalancer %s start failed", *output.LoadBalancerID)
		return err
	}
	klog.V(2).Infof("Lb %s is successfully started", l.Name)
	klog.V(2).Infof("Waiting for Listeners of Lb %s starting", l.Name)
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
	err = l.ConfirmQcLoadBalancer()
	if err != nil {
		klog.Errorf("Failed to make loadbalancer %s go into effect", l.Name)
		return err
	}
	klog.V(1).Infof("Loadbalancer %s created succeefully", l.Name)
	return nil
}

// Resize change the type of lb in qingcloud
func (l *LoadBalancer) Resize() error {
	if l.Status.QcLoadBalancer == nil {
		err := l.LoadQcLoadBalancer()
		if err != nil {
			return err
		}
	}
	if l.NeedResize() {
		//Stop
		klog.V(2).Infof("Detect lb size changed, begin to resize the lb %s", l.Name)
		err := l.Stop()
		if err != nil {
			klog.Errorf("Failed to stop lb %s when try to resize", l.Name)
		}
		klog.V(2).Infof("Resizing the lb %s", l.Name)
		output, err := l.lbapi.ResizeLoadBalancers(&qcservice.ResizeLoadBalancersInput{
			LoadBalancerType: &l.Type,
			LoadBalancers:    []*string{l.Status.QcLoadBalancer.LoadBalancerID},
		})
		if err != nil {
			return err
		}
		err = qcclient.WaitJob(l.jobapi, *output.JobID, operationWaitTimeout, waitInterval)
		if err != nil {
			klog.Errorf("Failed to waiting for lb resizing done")
			return err
		}
		return l.Start()
	}
	return nil
}

// UpdateQingCloudLB update some attrs of qingcloud lb
func (l *LoadBalancer) UpdateQingCloudLB() error {
	if l.Status.QcLoadBalancer == nil {
		klog.Warningf("Nothing can do before loading qingcloud loadBalancer %s", l.Name)
		return nil
	}
	if l.NeedResize() {
		err := l.Resize()
		if err != nil {
			klog.Errorf("Failed to resize lb %s", l.Name)
			return err
		}
	}

	if yes, toadd, todelete := l.NeedChangeIP(); yes {
		klog.V(2).Infof("Adding eips %s to and deleting %s from lb %s", toadd, todelete, l.Name)
		err := l.AssociateEipToLoadBalancer(toadd...)
		if err != nil {
			klog.Errorf("Failed to add eips %s to lb %s", toadd, l.Name)
			return err
		}
		err = l.DissociateEipFromLoadBalancer(todelete...)
		if err != nil {
			klog.Errorf("Failed to add eips %s to lb %s", todelete, l.Name)
			return err
		}
	}

	if l.NeedUpdate() {
		output, err := l.lbapi.ModifyLoadBalancerAttributes(&qcservice.ModifyLoadBalancerAttributesInput{
			LoadBalancerName: &l.Name,
			LoadBalancer:     l.Status.QcLoadBalancer.LoadBalancerID,
		})
		if err != nil {
			klog.Errorf("Couldn't update loadBalancer '%s'", *l.Status.QcLoadBalancer.LoadBalancerID)
			return err
		}
		if *output.RetCode != 0 {
			err := fmt.Errorf("Fail to update loadbalancer %s  because of '%s'", l.Name, *output.Message)
			return err
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
	err = l.ConfirmQcLoadBalancer()
	if err != nil {
		klog.Errorf("Failed to make loadbalancer %s go into effect", l.Name)
		return err
	}
	return l.waitLoadBalancerActive(*l.Status.QcLoadBalancer.LoadBalancerID)
}

// AssociateEipToLoadBalancer bind the eips to lb in qingcloud
func (l *LoadBalancer) AssociateEipToLoadBalancer(eips ...string) error {
	if len(eips) == 0 {
		return nil
	}
	klog.V(2).Infof("Starting to associate Eip %s to loadBalancer '%s'", eips, l.Name)
	output, err := l.lbapi.AssociateEIPsToLoadBalancer(&qcservice.AssociateEIPsToLoadBalancerInput{
		EIPs:         qcservice.StringSlice(eips),
		LoadBalancer: l.Status.QcLoadBalancer.LoadBalancerID,
	})
	if err != nil {
		klog.Errorf("Failed to add eip %s to lb %s", eips, l.Name)
		return err
	}
	return qcclient.WaitJob(l.jobapi, *output.JobID, operationWaitTimeout, waitInterval)
}

// DissociateEipFromLoadBalancer unbind the eips from lb in qingcloud
func (l *LoadBalancer) DissociateEipFromLoadBalancer(eips ...string) error {
	if len(eips) == 0 {
		return nil
	}
	klog.V(2).Infof("Starting to dissociate Eip %s to loadBalancer '%s'", eips, l.Name)
	output, err := l.lbapi.DissociateEIPsFromLoadBalancer(&qcservice.DissociateEIPsFromLoadBalancerInput{
		EIPs:         qcservice.StringSlice(eips),
		LoadBalancer: l.Status.QcLoadBalancer.LoadBalancerID,
	})
	if err != nil {
		klog.Errorf("Failed to add eip %s to lb %s", eips, l.Name)
		return err
	}
	return qcclient.WaitJob(l.jobapi, *output.JobID, operationWaitTimeout, waitInterval)
}

// ConfirmQcLoadBalancer make sure each operation is taken effects
func (l *LoadBalancer) ConfirmQcLoadBalancer() error {
	output, err := l.lbapi.UpdateLoadBalancers(&qcservice.UpdateLoadBalancersInput{
		LoadBalancers: []*string{l.Status.QcLoadBalancer.LoadBalancerID},
	})
	if err != nil {
		klog.Errorf("Couldn't confirm updates on loadBalancer '%s'", l.Name)
		return err
	}
	klog.V(2).Infof("Waiting for updates of lb %s taking effects", l.Name)
	return qcclient.WaitJob(l.jobapi, *output.JobID, operationWaitTimeout, waitInterval)
}

//GetLBAPI return qingcloud LoadBalancer API
func (l *LoadBalancer) GetLBAPI() *qcservice.LoadBalancerService {
	return l.lbapi
}

//GetJobAPI return qingcloud Job API
func (l *LoadBalancer) GetJobAPI() *qcservice.JobService {
	return l.jobapi
}

// GetService return service of this loadbalancer
func (l *LoadBalancer) GetService() *corev1.Service {
	return l.service
}

func (l *LoadBalancer) deleteSecurityGroup() error {
	if l.Status.QcSecurityGroup == nil {
		err := l.LoadSecurityGroup()
		if err != nil {
			return err
		}
		if l.Status.QcSecurityGroup == nil {
			return nil
		}
	}

	input := &qcservice.DeleteSecurityGroupsInput{SecurityGroups: []*string{l.Status.QcSecurityGroup.SecurityGroupID}}
	_, err := l.sgapi.DeleteSecurityGroups(input)
	if err != nil {
		return err
	}
	return nil
}

func (l *LoadBalancer) deleteListenersOnlyIfOK() (bool, error) {
	if l.Status.QcLoadBalancer == nil {
		return false, nil
	}
	listeners, err := GetLoadBalancerListeners(l.lbapi, *l.Status.QcLoadBalancer.LoadBalancerID, "")
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
	if isUsedByAnotherSevice {
		for _, listener := range toDelete {
			err = deleteQingCloudListener(l.lbapi, listener.LoadBalancerListenerID)
			if err != nil {
				klog.Errorf("Failed to delete listener %s", *listener.LoadBalancerListenerName)
				return false, err
			}
		}
	}
	return isUsedByAnotherSevice, nil
}

func (l *LoadBalancer) DeleteQingCloudLB() error {
	if l.Status.QcLoadBalancer == nil {
		err := l.LoadQcLoadBalancer()
		if err != nil {
			if err == ErrorLBNotFoundInCloud {
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
	//record eip id before deleting
	ip := l.Status.QcLoadBalancer.Cluster[0]
	output, err := l.lbapi.DeleteLoadBalancers(&qcservice.DeleteLoadBalancersInput{LoadBalancers: []*string{l.Status.QcLoadBalancer.LoadBalancerID}})
	if err != nil {
		klog.Errorf("Failed to start job to delete lb %s", *l.Status.QcLoadBalancer.LoadBalancerID)
		return err
	}
	err = qcclient.WaitJob(l.jobapi, *output.JobID, operationWaitTimeout, waitInterval)
	if err != nil {
		klog.Errorf("Failed to excute deletion of lb %s", *l.Status.QcLoadBalancer.LoadBalancerName)
		return err
	}
	err = l.deleteSecurityGroup()
	if err != nil {
		klog.Errorf("Failed to delete SecurityGroup of lb %s err '%s' ", l.Name, err)
	}

	if l.EIPAllocateSource != ManualSet && *ip.EIPName == eip.AllocateEIPName {
		klog.V(2).Infof("Detect eip %s of lb %s is allocated, release it", *ip.EIPID, l.Name)
		err := l.ReleaseEIP(*ip.EIPID)
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
			if err == ErrorLBNotFoundInCloud {
				return nil
			}
			klog.Errorf("Failed to load qc loadbalance of %s", l.Name)
			return err
		}
	}
	status := &corev1.LoadBalancerStatus{}
	for _, eip := range l.Status.QcLoadBalancer.Cluster {
		status.Ingress = append(status.Ingress, corev1.LoadBalancerIngress{IP: *eip.EIPAddr})
	}
	for _, ip := range l.Status.QcLoadBalancer.EIPs {
		status.Ingress = append(status.Ingress, corev1.LoadBalancerIngress{IP: *ip.EIPAddr})
	}
	if len(status.Ingress) == 0 {
		return fmt.Errorf("Have no ip yet")
	}
	l.Status.K8sLoadBalancerStatus = status
	return nil
}

func (l *LoadBalancer) waitLoadBalancerActive(id string) error {
	loadBalancer, err := qcclient.WaitLoadBalancerStatus(l.lbapi, id, qcclient.LoadBalancerStatusActive, operationWaitTimeout, waitInterval)
	if err == nil {
		l.Status.QcLoadBalancer = loadBalancer
		l.GenerateK8sLoadBalancer()
	}
	return err
}

/// -----Shared  functions-------

// GetLoadBalancerName generate lb name for each service. The name of a service is fixed and predictable
func GetLoadBalancerName(clusterName string, service *corev1.Service) string {
	defaultName := fmt.Sprintf("k8s_lb_%s_%s_%s", clusterName, service.Name, util.GetFirstUID(string(service.UID)))
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
	listeners, err := GetLoadBalancerListeners(l.lbapi, *l.Status.QcLoadBalancer.LoadBalancerID, GetListenerPrefix(l.service))
	if err != nil {
		klog.Errorf("Failed to get qingcloud listeners of lb %s", l.Name)
		return err
	}

	for _, listener := range listeners {
		if util.IntIndex(l.TCPPorts, *listener.ListenerPort) == -1 {
			err := deleteQingCloudListener(l.lbapi, listener.LoadBalancerListenerID)
			if err != nil {
				klog.Errorf("Failed to delete listener %s", *listener.LoadBalancerListenerName)
				return err
			}
		}
	}
	return nil
}

// GetLoadBalancerByName return nil if not found
func GetLoadBalancerByName(lbapi *qcservice.LoadBalancerService, name string) (*qcservice.LoadBalancer, error) {
	status := []*string{qcservice.String("pending"), qcservice.String("active"), qcservice.String("stopped")}
	output, err := lbapi.DescribeLoadBalancers(&qcservice.DescribeLoadBalancersInput{
		Status:     status,
		SearchWord: &name,
	})
	if err != nil {
		return nil, err
	}
	if len(output.LoadBalancerSet) == 0 {
		return nil, nil
	}
	for _, lb := range output.LoadBalancerSet {
		if lb.LoadBalancerName != nil && *lb.LoadBalancerName == name {
			return lb, nil
		}
	}
	return nil, nil
}

func GetLoadBalancerByID(lbapi *qcservice.LoadBalancerService, id string) (*qcservice.LoadBalancer, error) {
	output, err := lbapi.DescribeLoadBalancers(&qcservice.DescribeLoadBalancersInput{
		LoadBalancers: []*string{&id},
	})
	if err != nil {
		return nil, err
	}
	if len(output.LoadBalancerSet) == 0 {
		return nil, nil
	}
	lb := output.LoadBalancerSet[0]
	if *lb.Status == qcclient.LoadBalancerStatusCeased || *lb.Status == qcclient.LoadBalancerStatusDeleted {
		return nil, nil
	}
	return lb, nil
}

// GetSecurityGroupByID return SecurityGroup in qingcloud using ID
func GetSecurityGroupByID(sgapi *qcservice.SecurityGroupService, id string) (*qcservice.SecurityGroup, error) {
	input := &qcservice.DescribeSecurityGroupsInput{SecurityGroups: []*string{&id}}
	output, err := sgapi.DescribeSecurityGroups(input)
	if err != nil {
		return nil, err
	}
	if len(output.SecurityGroupSet) > 0 {
		return output.SecurityGroupSet[0], nil
	}
	return nil, nil
}

// CreateSecurityGroup create a SecurityGroup in qingcloud
func CreateSecurityGroup(sgapi *qcservice.SecurityGroupService, sgName string, rules []*qcservice.SecurityGroupRule) (*qcservice.SecurityGroup, error) {
	createInput := &qcservice.CreateSecurityGroupInput{SecurityGroupName: &sgName}
	createOutput, err := sgapi.CreateSecurityGroup(createInput)
	if err != nil {
		return nil, err
	}
	sgID := createOutput.SecurityGroupID
	addRuleOutput, err := sgapi.AddSecurityGroupRules(&qcservice.AddSecurityGroupRulesInput{SecurityGroup: sgID, Rules: rules})
	if err != nil {
		klog.Errorf("Failed to add security rule to group %s", *sgID)
		return nil, err
	}
	klog.V(4).Infof("AddSecurityGroupRules SecurityGroup: [%s], output: [%+v] ", *sgID, addRuleOutput)
	o, err := sgapi.ApplySecurityGroup(&qcservice.ApplySecurityGroupInput{SecurityGroup: sgID})
	if err != nil {
		klog.Errorf("Failed to apply security rule to group %s", *sgID)
		return nil, err
	}
	if *o.RetCode != 0 {
		err := fmt.Errorf("Failed to apply security group,err: %s", *o.Message)
		return nil, err
	}
	sg, _ := GetSecurityGroupByID(sgapi, *sgID)
	return sg, nil
}
