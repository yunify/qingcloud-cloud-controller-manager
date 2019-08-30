package loadbalance

import (
	"fmt"
	"strconv"

	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/executor"
	qcservice "github.com/yunify/qingcloud-sdk-go/service"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog"
)

var (
	ErrorListenerPortConflict = fmt.Errorf("Port has been occupied")
	ErrorReuseEIPButNoName    = fmt.Errorf("If you want to reuse an eip , you must specify the name of each port in service")
	ErrorListenerNotFound     = fmt.Errorf("Failed to get listener in cloud")
)

// Listener is
type Listener struct {
	backendList  *BackendList
	listenerExec executor.QingCloudListenerExecutor
	backendExec  executor.QingCloudListenerBackendExecutor
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
		LisenerSpec: LisenerSpec{
			ListenerPort: port,
			NodePort:     int(p.NodePort),
			BalanceMode:  "source",
			lb:           lb,
			PrefixName:   GetListenerPrefix(service),
		},
	}
	if lsnExec, ok := lb.lbExec.(executor.QingCloudListenerExecutor); ok {
		result.listenerExec = lsnExec
	}
	if bakExec, ok := lb.lbExec.(executor.QingCloudListenerBackendExecutor); ok {
		result.backendExec = bakExec
	}
	result.Name = result.PrefixName + strconv.Itoa(int(p.Port))
	if p.Protocol == corev1.ProtocolTCP && p.Name == "http" {
		result.Protocol = "http"
	} else if p.Protocol == corev1.ProtocolUDP {
		result.Protocol = "udp"
	} else {
		result.Protocol = "tcp"
	}
	return result, nil
}

// LoadQcListener get real lb in qingcloud
func (l *Listener) LoadQcListener() error {
	listener, err := l.listenerExec.GetListenerByName(*l.lb.Status.QcLoadBalancer.LoadBalancerID, l.Name)
	if err != nil {
		klog.Errorf("Failed to get listener of this service %s with port %d", l.Name, l.ListenerPort)
		return err
	}
	if listener == nil {
		return ErrorListenerNotFound
	}
	l.Status = listener
	return nil
}

func (l *Listener) LoadBackends() {
	if l.backendList == nil {
		l.backendList = NewBackendList(l.lb, l)
	}
}

func (l *Listener) CheckPortConflict() (bool, error) {
	if l.lb.EIPStrategy != ReuseEIP {
		return false, nil
	}
	listeners, err := l.listenerExec.GetListenersOfLB(*l.lb.Status.QcLoadBalancer.LoadBalancerID, "")
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
	input := &qcservice.AddLoadBalancerListenersInput{
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
	}
	if l.Protocol == "udp" {
		input.Listeners[0].HealthyCheckMethod = qcservice.String("udp")
	}
	listener, err := l.listenerExec.CreateListener(input)
	if err != nil {
		return err
	}
	l.Status = listener
	return nil
}

func (l *Listener) GetBackends() *BackendList {
	return l.backendList
}

func (l *Listener) DeleteQingCloudListener() error {
	if l.Status == nil {
		return fmt.Errorf("Could not delete noexit listener")
	}
	klog.Infof("Deleting LoadBalancerListener :'%s'", *l.Status.LoadBalancerListenerID)
	return l.listenerExec.DeleteListener(*l.Status.LoadBalancerListenerID)
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
func (l *Listener) UpdateBackends() error {
	l.LoadBackends()
	useless, err := l.backendList.LoadAndGetUselessBackends()
	if err != nil {
		klog.Errorf("Failed to load backends of listener %s", l.Name)
		return err
	}
	if len(useless) > 0 {
		klog.V(1).Infof("Delete useless backends")
		err := l.backendExec.DeleteBackends(useless...)
		if err != nil {
			klog.Errorf("Failed to delete useless backends of listener %s", l.Name)
			return err
		}
	}
	for _, b := range l.backendList.Items {
		err := b.LoadQcBackend()
		if err != nil {
			if err == ErrorBackendNotFound {
				err = b.Create()
				if err != nil {
					klog.Errorf("Failed to create backend of instance %s of listener %s", b.Spec.InstanceID, l.Name)
					return err
				}
			} else {
				return err
			}
		} else {
			err = b.UpdateBackend()
			if err != nil {
				klog.Errorf("Failed to update backend %s of listener %s", b.Name, l.Name)
				return err
			}
		}
	}
	return nil
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
		klog.Errorf("Failed to load listener %s in qingcloud", l.Name)
		return err
	}
	err = l.UpdateBackends()
	if err != nil {
		return err
	}
	if !l.NeedUpdate() {
		return nil
	}
	klog.Infof("Modifying balanceMode of LoadBalancerTCPListener :'%s'", *l.Status.LoadBalancerListenerID)
	return l.listenerExec.ModifyListener(*l.Status.LoadBalancerListenerID, l.BalanceMode)
}

func checkPortInService(service *corev1.Service, port int) *corev1.ServicePort {
	for index, p := range service.Spec.Ports {
		if int(p.Port) == port {
			return &service.Spec.Ports[index]
		}
	}
	return nil
}

func GetListenerPrefix(service *corev1.Service) string {
	return fmt.Sprintf("listener_%s_%s_", service.Namespace, service.Name)
}
