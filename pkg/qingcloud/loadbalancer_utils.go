package qingcloud

import (
	"fmt"
	"k8s.io/api/core/v1"
	"strconv"
	"strings"

	qcservice "github.com/yunify/qingcloud-sdk-go/service"

	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/apis"
)

const defaultListenerHeathyCheckOption = "10|5|2|5"
const defaultListenerBalanceMode = "roundrobin"

// Make sure qingcloud instance hostname or override-hostname (if provided) is equal to InstanceId
// Recommended to use override-hostname
func nodeToInstanceIDs(node *v1.Node) string {
	var result string

	if instanceid, ok := node.GetAnnotations()[NodeAnnotationInstanceID]; ok {
		result = instanceid
	} else {
		result = node.Name
	}

	return result
}

func convertLoadBalancerStatus(status *apis.LoadBalancerStatus) *v1.LoadBalancerStatus {
	var result []v1.LoadBalancerIngress

	for _, ip := range status.VIP {
		result = append(result, v1.LoadBalancerIngress{
			IP:       ip,
			Hostname: "",
		})
	}

	return &v1.LoadBalancerStatus{Ingress: result}
}

//The reason for this is that it is compatible with old logic, and future updates to the
//load balancer will be placed in the relevant crd controller.
func needUpdateAttr(config *LoadBalancerConfig, status *apis.LoadBalancer) *apis.LoadBalancer {
	result := &apis.LoadBalancer{
		Status: apis.LoadBalancerStatus{
			LoadBalancerID: status.Status.LoadBalancerID,
		},
	}
	update := false

	if config.NodeCount != nil && *config.NodeCount != *status.Spec.NodeCount {
		update = true
		result.Spec.NodeCount = config.NodeCount
	}

	if config.InternalIP != nil {
		found := false
		for _, ip := range status.Spec.PrivateIPs {
			if strings.Compare(*ip, *config.InternalIP) == 0 {
				found = true
				break
			}
		}
		if !found {
			update = true
			result.Spec.PrivateIPs = append(result.Spec.PrivateIPs, config.InternalIP)
		}
	}

	if update {
		return result
	}
	return nil
}

//The load balancer will be shared, filtering out its own listeners.
func filterListeners(listeners []apis.LoadBalancerListener, prefix string) []*string {
	var result []*string

	for _, listener := range listeners {
		if strings.HasPrefix(*listener.Spec.LoadBalancerListenerName, prefix) {
			result = append(result, listener.Status.LoadBalancerListenerID)
		}
	}

	return result
}

func getHealthyCheck(annotationConf map[int]healthyChek, port int, proto string) healthyChek {
	option := defaultListenerHeathyCheckOption
	healthyCheck := healthyChek{
		method: &proto,
		option: &option,
	}
	if annotationConf != nil {
		hc := annotationConf[port]
		if hc.option != nil {
			healthyCheck.option = hc.option
		}
		if hc.method != nil {
			healthyCheck.method = hc.method
		}
	}
	return healthyCheck
}

func getBalanceMode(annotationConf map[int]string, port int) *string {
	balanceMode := defaultListenerBalanceMode
	if annotationConf != nil {
		bm := annotationConf[port]
		if bm != "" {
			return &bm
		}
	}
	return &balanceMode
}

func diffListeners(listeners []*apis.LoadBalancerListener, conf *LoadBalancerConfig, ports []v1.ServicePort) (toDelete []*string, toAdd []v1.ServicePort) {
	svcNodePort := make(map[string]int)
	for _, listener := range listeners {
		if len(listener.Status.LoadBalancerBackends) > 0 {
			svcNodePort[*listener.Status.LoadBalancerListenerID] = *listener.Status.LoadBalancerBackends[0].Spec.Port
		}
	}

	hcs, _ := parseHeathyCheck(conf)
	bms, _ := parseBalanceMode(conf)
	for _, port := range ports {
		add := true
		healthyCheck := getHealthyCheck(hcs, int(port.Port), strings.ToLower(string(port.Protocol)))
		balanceMode := getBalanceMode(bms, int(port.Port))
		for _, listener := range listeners {
			if *listener.Spec.ListenerPort == int(port.Port) &&
				strings.EqualFold(*listener.Spec.ListenerProtocol, string(port.Protocol)) &&
				svcNodePort[*listener.Status.LoadBalancerListenerID] == int(port.NodePort) &&
				*balanceMode == *listener.Spec.BalanceMode &&
				(*healthyCheck.option == *listener.Spec.HealthyCheckOption && *healthyCheck.method == *listener.Spec.HealthyCheckMethod) {
				add = false
				break
			}
		}
		if add {
			toAdd = append(toAdd, port)
		}
	}

	for _, listener := range listeners {
		delete := true
		for _, port := range ports {
			healthyCheck := getHealthyCheck(hcs, int(port.Port), strings.ToLower(string(port.Protocol)))
			balanceMode := getBalanceMode(bms, int(port.Port))
			if *listener.Spec.ListenerPort == int(port.Port) &&
				strings.EqualFold(*listener.Spec.ListenerProtocol, string(port.Protocol)) &&
				svcNodePort[*listener.Status.LoadBalancerListenerID] == int(port.NodePort) &&
				*balanceMode == *listener.Spec.BalanceMode &&
				(*healthyCheck.option == *listener.Spec.HealthyCheckOption && *healthyCheck.method == *listener.Spec.HealthyCheckMethod) {
				delete = false
				break
			}
		}
		if delete {
			toDelete = append(toDelete, listener.Status.LoadBalancerListenerID)
		}
	}

	return
}

func getLoadBalancerListenerNodePort(listener *apis.LoadBalancerListener, ports []v1.ServicePort) *int {
	var result *int

	if len(listener.Status.LoadBalancerBackends) > 0 {
		return listener.Status.LoadBalancerBackends[0].Spec.Port
	}

	for _, port := range ports {
		if *listener.Spec.ListenerPort == int(port.Port) &&
			strings.EqualFold(*listener.Spec.ListenerProtocol, string(port.Protocol)) {
			result = qcservice.Int(int(port.NodePort))
			return result
		}
	}

	return result
}

func generateLoadBalancerBackends(nodes []*v1.Node, listener *apis.LoadBalancerListener, ports []v1.ServicePort) []*apis.LoadBalancerBackend {
	var backends []*apis.LoadBalancerBackend

	for _, node := range nodes {
		nodeName := nodeToInstanceIDs(node)
		backend := &apis.LoadBalancerBackend{
			Spec: apis.LoadBalancerBackendSpec{
				LoadBalancerListenerID:  listener.Status.LoadBalancerListenerID,
				LoadBalancerBackendName: &nodeName,
				ResourceID:              &nodeName,
				Port:                    getLoadBalancerListenerNodePort(listener, ports),
			},
		}
		backends = append(backends, backend)
	}

	return backends
}

type healthyChek struct {
	method *string
	option *string
}

// data format like this: port1:conf,port2:conf,port3:conf
// Example:
//  1)healthycheckmethod: "80:tcp,443:tcp"
//  2)healthycheckoption: "80:10|5|2|5,443:10|5|2|5"
//  3)balancemode: "80:roundrobin,443:leastconn,8080:source"
func parseLsnAnnotaionData(data string) (map[int]string, error) {
	methods := strings.Split(data, ",")
	rst := make(map[int]string, len(methods))
	for _, method := range methods {
		m := strings.Split(method, ":")
		if len(m) != 2 {
			return nil, fmt.Errorf("wrong format: (%s)", data)
		}
		port, err := strconv.Atoi(m[0])
		if err != nil {
			return nil, fmt.Errorf("wrong format: (%s)", data)
		}
		rst[port] = m[1]
	}
	return rst, nil
}

func parseHeathyCheck(conf *LoadBalancerConfig) (map[int]healthyChek, error) {
	if conf == nil ||
		(conf.healthyCheckMethod == nil && conf.healthyCheckOption == nil) {
		return nil, nil
	}

	var methods map[int]string
	var options map[int]string
	var err error
	if conf.healthyCheckMethod != nil {
		methods, err = parseLsnAnnotaionData(*conf.healthyCheckMethod)
	}
	if conf.healthyCheckOption != nil {
		options, err = parseLsnAnnotaionData(*conf.healthyCheckOption)
	}

	if err != nil {
		return nil, err
	}

	h := map[int]healthyChek{}
	for port, method := range methods {
		h[port] = healthyChek{
			method: qcservice.String(method),
		}
	}

	for port, option := range options {
		if data, ok := h[port]; ok {
			data.option = qcservice.String(option)
			h[port] = data
		} else {
			h[port] = healthyChek{
				option: qcservice.String(option),
			}
		}
	}

	return h, nil
}

func parseBalanceMode(conf *LoadBalancerConfig) (map[int]string, error) {
	if conf == nil || conf.balanceMode == nil {
		return nil, nil
	}

	return parseLsnAnnotaionData(*conf.balanceMode)
}

func generateLoadBalancerListeners(conf *LoadBalancerConfig, lb *apis.LoadBalancer, ports []v1.ServicePort) ([]*apis.LoadBalancerListener, error) {
	hcs, err := parseHeathyCheck(conf)
	if err != nil {
		return nil, err
	}

	bms, err := parseBalanceMode(conf)
	if err != nil {
		return nil, err
	}

	var result []*apis.LoadBalancerListener
	for _, port := range ports {
		protocol := ""
		switch port.Protocol {
		case v1.ProtocolUDP:
			protocol = "udp"
		case v1.ProtocolTCP:
			protocol = "tcp"
		default:
			return nil, fmt.Errorf("loadbalance not support protocol %s", port.Protocol)
		}

		healthyCheck := getHealthyCheck(hcs, int(port.Port), strings.ToLower(string(port.Protocol)))
		balanceMode := getBalanceMode(bms, int(port.Port))
		result = append(result, &apis.LoadBalancerListener{
			Spec: apis.LoadBalancerListenerSpec{
				BackendProtocol:          &protocol,
				ListenerProtocol:         &protocol,
				ListenerPort:             qcservice.Int(int(port.Port)),
				LoadBalancerListenerName: &conf.listenerName,
				LoadBalancerID:           lb.Status.LoadBalancerID,
				HealthyCheckMethod:       healthyCheck.method,
				HealthyCheckOption:       healthyCheck.option,
				BalanceMode:              balanceMode,
			},
		})
	}

	if len(result) <= 0 {
		return nil, fmt.Errorf("service has not port")
	}

	return result, nil
}

func nodesHasBackend(backend string, nodes []*v1.Node) bool {
	for _, node := range nodes {
		if strings.Contains(backend, nodeToInstanceIDs(node)) {
			return true
		}
	}

	return false
}

func backendsHasNode(node *v1.Node, backends []*apis.LoadBalancerBackend) bool {
	for _, backend := range backends {
		if strings.Contains(*backend.Spec.LoadBalancerBackendName, nodeToInstanceIDs(node)) {
			return true
		}
	}

	return false
}

func diffBackend(listener *apis.LoadBalancerListener, nodes []*v1.Node) (toDelete []*string, toAdd []*v1.Node) {
	for _, backend := range listener.Status.LoadBalancerBackends {
		if !nodesHasBackend(*backend.Spec.LoadBalancerBackendName, nodes) {
			toDelete = append(toDelete, backend.Status.LoadBalancerBackendID)
		}
	}

	for _, node := range nodes {
		if !backendsHasNode(node, listener.Status.LoadBalancerBackends) {
			toAdd = append(toAdd, node)
		}
	}

	return
}
