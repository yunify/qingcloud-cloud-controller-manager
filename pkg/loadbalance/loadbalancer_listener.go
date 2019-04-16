package loadbalance

import (
	"fmt"
	"strings"

	qcservice "github.com/yunify/qingcloud-sdk-go/service"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/klog"
)

var (
	ErrorListenerPortConflict = fmt.Errorf("Port has been occupied")
	ErrorReuseEIPButNoName    = fmt.Errorf("If you want to reuse an eip , you must specify the name of each port in service")
	ErrorListenerNotFound     = fmt.Errorf("Failed to get listener in cloud")
)

// Listener is
type Listener struct {
	//inject services
	lbapi       *qcservice.LoadBalancerService
	jobapi      *qcservice.JobService
	backendList *BackendList

	LisenerSpec
	Status *qcservice.LoadBalancerListener
}

type LisenerSpec struct {
	lb           *LoadBalancer
	PrefixName   string
	Name         string
	ListenerPort int
	BalanceMode  string
	Protocol     string
	NodePort     int
}

func NewListener(lb *LoadBalancer, port int) (*Listener, error) {
	service := lb.GetService()
	p := checkPortInService(service, port)
	if p == nil {
		return nil, fmt.Errorf("The specified port is not in service")
	}

	result := &Listener{
		lbapi:  lb.lbapi,
		jobapi: lb.jobapi,
		LisenerSpec: LisenerSpec{
			ListenerPort: port,
			NodePort:     int(p.NodePort),
			BalanceMode:  "source",
			lb:           lb,
			PrefixName:   GetListenerPrefix(service),
		},
	}

	result.Name = result.PrefixName + fmt.Sprintf("_%d", port)

	if p.Name == "http" || p.Name == "https" {
		result.Protocol = p.Name
	} else {
		result.Protocol = "tcp"
	}

	return result, nil
}

func (l *Listener) LoadQcListener() error {
	listeners, err := GetLoadBalancerListeners(l.lbapi, *l.lb.Status.QcLoadBalancer.LoadBalancerID, l.Name)
	if err != nil {
		klog.Errorf("Failed to get listener of this service %s with port %d", l.Name, l.ListenerPort)
		return err
	}
	if len(listeners) > 1 {
		klog.Exit("Fatal ! Get multi listeners for one port, quit now")
	}
	if len(listeners) == 0 {
		return ErrorListenerNotFound
	}
	l.Status = listeners[0]
	return nil
}

func (l *Listener) LoadBackends() {
	if l.backendList == nil {
		l.backendList = NewBackendList(l.lb, l)
	}
}

func GetLoadBalancerListeners(lbapi *qcservice.LoadBalancerService, lbid, listenerPrefix string) ([]*qcservice.LoadBalancerListener, error) {
	loadBalancerListeners := []*qcservice.LoadBalancerListener{}
	resp, err := lbapi.DescribeLoadBalancerListeners(&qcservice.DescribeLoadBalancerListenersInput{
		LoadBalancer: &lbid,
		Verbose:      qcservice.Int(1),
		Limit:        qcservice.Int(pageLimt),
	})
	if err != nil {
		return nil, err
	}
	for _, l := range resp.LoadBalancerListenerSet {
		if listenerPrefix != "" {
			if strings.Contains(*l.LoadBalancerListenerName, listenerPrefix) {
				loadBalancerListeners = append(loadBalancerListeners, l)
			}
		} else {
			loadBalancerListeners = append(loadBalancerListeners, l)
		}
	}
	return loadBalancerListeners, nil
}

func (l *Listener) CheckPortConflict() (bool, error) {
	if l.lb.EIPStrategy != ReuseEIP {
		return false, nil
	}
	listeners, err := GetLoadBalancerListeners(l.lbapi, *l.lb.Status.QcLoadBalancer.LoadBalancerID, "")
	if err != nil {
		return false, err
	}
	for _, list := range listeners {
		if *list.ListenerPort == l.ListenerPort {
			return true, nil
		}
	}
	return false, nil
}

func (l *Listener) CreateQingCloudListenerWithBackends() error {
	err := l.CreateQingCloudListener()
	if err != nil {
		return err
	}
	l.LoadBackends()
	err = l.backendList.CreateBackends()
	if err != nil {
		klog.Errorf("Failed to create backends of listener %s", l.Name)
		return err
	}
	return nil
}

func (l *Listener) CreateQingCloudListener() error {
	if l.Status != nil {
		klog.Warningln("Create listener even have a listener")
	}
	yes, err := l.CheckPortConflict()
	if err != nil {
		klog.Errorf("Failed to check port conflicts")
		return err
	}
	if yes {
		return ErrorListenerPortConflict
	}
	output, err := l.lbapi.AddLoadBalancerListeners(&qcservice.AddLoadBalancerListenersInput{
		LoadBalancer: l.lb.Status.QcLoadBalancer.LoadBalancerID,
		Listeners: []*qcservice.LoadBalancerListener{
			{
				ListenerProtocol:         &l.Protocol,
				BackendProtocol:          &l.Protocol,
				BalanceMode:              &l.BalanceMode,
				ListenerPort:             &l.ListenerPort,
				LoadBalancerListenerName: &l.Name,
			},
		},
	})
	if err != nil {
		return err
	}
	l.Status = new(qcservice.LoadBalancerListener)
	l.Status.ListenerPort = &l.ListenerPort
	l.Status.LoadBalancerID = l.lb.Status.QcLoadBalancer.LoadBalancerID
	l.Status.ListenerProtocol = &l.Protocol
	l.Status.LoadBalancerListenerID = output.LoadBalancerListeners[0]
	return err
}

func (l *Listener) DeleteQingCloudListener() error {
	if l.Status == nil {
		return fmt.Errorf("Could not delete noexit listener")
	}
	klog.Infof("Deleting LoadBalancerListener :'%s'", *l.Status.LoadBalancerListenerID)
	return deleteQingCloudListener(l.lbapi, l.Status.LoadBalancerListenerID)
}
func deleteQingCloudListener(lbapi *qcservice.LoadBalancerService, id *string) error {
	output, err := lbapi.DeleteLoadBalancerListeners(&qcservice.DeleteLoadBalancerListenersInput{
		LoadBalancerListeners: []*string{id},
	})
	if err != nil {
		return err
	}
	if *output.RetCode != 0 {
		err := fmt.Errorf("Fail to delete loadbalancer lisener '%s' because of '%s'", *id, *output.Message)
		return err
	}
	return nil
}

func (l *Listener) NeedUpdate() bool {
	if l.Status == nil {
		return false
	}
	if l.BalanceMode != *l.Status.BalanceMode {
		return true
	}
	return false
}

func (l *Listener) UpdateQingCloudListener() error {
	err := l.LoadQcListener()
	//create if not exist
	if err == ErrorListenerNotFound {
		err = l.CreateQingCloudListenerWithBackends()
		if err != nil {
			klog.Errorf("Failed to create backends of listener %s of loadbalancer %s", l.Name, l.lb.Name)
			return err
		}
		return nil
	}
	if err != nil {
		return err
	}
	if !l.NeedUpdate() {
		return fmt.Errorf("Update is not needed")
	}
	klog.Infof("Modifying balanceMode of LoadBalancerTCPListener :'%s'", *l.Status.LoadBalancerListenerID)
	output, err := l.lbapi.ModifyLoadBalancerListenerAttributes(&qcservice.ModifyLoadBalancerListenerAttributesInput{
		LoadBalancerListener: l.Status.LoadBalancerListenerID,
		BalanceMode:          &l.BalanceMode,
	})
	if err != nil {
		return err
	}
	if *output.RetCode != 0 {
		err := fmt.Errorf("Fail to modify balanceMode of loadbalancer lisener '%s' because of '%s'", *l.Status.LoadBalancerListenerID, *output.Message)
		return err
	}
	return nil
}

func checkPortInService(service *corev1.Service, port int) *corev1.ServicePort {
	for index, p := range service.Spec.Ports {
		if p.Protocol != v1.ProtocolUDP && int(p.Port) == port {
			return &service.Spec.Ports[index]
		}
	}
	return nil
}

func GetListenerPrefix(service *corev1.Service) string {
	return fmt.Sprintf("listener_%s_%s", service.Namespace, service.Name)
}
