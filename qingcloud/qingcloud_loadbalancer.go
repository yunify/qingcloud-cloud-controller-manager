package qingcloud

// See https://docs.qingcloud.com/api/lb/index.html
// qingcloud loadBalancer and instance have a default strict Security Group(firewall),
// its only allow SSH and PING. So, first of all, you shoud manually add correct rules
// for all nodes and loadBalancers. You can simply add a rule by pass all tcp port traffic.
// The loadBalancers also need at least one EIP before create it, please allocate some EIPs,
// and set them in service annotation ServiceAnnotationLoadBalancerEipIds.

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/golang/glog"
	qcclient "github.com/yunify/qingcloud-sdk-go/client"
	qcservice "github.com/yunify/qingcloud-sdk-go/service"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/kubernetes/pkg/api/v1"
	"k8s.io/kubernetes/pkg/cloudprovider"
)

const (
	// ServiceAnnotationLoadBalancerEipIds is the annotation which specifies a list of eip ids.
	// The ids in list are separated by ',', e.g. "eip-j38f2h3h,eip-ornz2xq7". And this annotation should
	// NOT be used with ServiceAnnotationLoadBalancerVxnetId. Please make sure there is one and only one
	// of them being set
	ServiceAnnotationLoadBalancerEipIds = "service.beta.kubernetes.io/qingcloud-load-balancer-eip-ids"

	/* ServiceAnnotationLoadBalancerVxnetId is the annotation which indicates the very vxnet where load
	 * balancer resides. This annotation should NOT be used when ServiceAnnotationLoadBalancerEipIds is
	 * set.
	 */
	ServiceAnnotationLoadBalancerVxnetId = "service.beta.kubernetes.io/qingcloud-load-balancer-vxnet-id"

	// ServiceAnnotationLoadBalancerType is the annotation used on the
	// service to indicate that we want a qingcloud loadBalancer type.
	// value "0" means the LB can max support 5000 concurrency connections, it's default type.
	// value "1" means the LB can max support 20000 concurrency connections.
	// value "2" means the LB can max support 40000 concurrency connections.
	// value "3" means the LB can max support 100000 concurrency connections.
	// value "4" means the LB can max support 200000 concurrency connections.
	// value "5" means the LB can max support 500000 concurrency connections.
	ServiceAnnotationLoadBalancerType = "service.beta.kubernetes.io/qingcloud-load-balancer-type"
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

// GetLoadBalancer returns whether the specified load balancer exists, and
// if so, what its status is.
func (qc *QingCloud) GetLoadBalancer(clusterName string, service *v1.Service) (status *v1.LoadBalancerStatus, exists bool, err error) {
	loadBalancerName := qc.getQingCloudLoadBalancerName(service)
	glog.V(3).Infof("GetLoadBalancer(%v, %v)", clusterName, loadBalancerName)

	loadBalancer, err := qc.getLoadBalancerByName(loadBalancerName)
	if err != nil {
		return nil, false, err
	}
	if loadBalancer == nil {
		return nil, false, nil
	}

	status = &v1.LoadBalancerStatus{}
	for _, ip := range loadBalancer.Cluster {
		status.Ingress = append(status.Ingress, v1.LoadBalancerIngress{IP: *ip.EIPAddr})
	}

	return status, true, nil
}

// EnsureLoadBalancer creates a new load balancer 'name', or updates the existing one. Returns the status of the balancer
// To create a LoadBalancer for kubernetes, we do the following:
// 1. create a qingcloud loadBalancer;
// 2. create listeners for the new loadBalancer, number of listeners = number of service ports;
// 3. add backends to the new loadBalancer.
func (qc *QingCloud) EnsureLoadBalancer(clusterName string, service *v1.Service, nodes []*v1.Node) (*v1.LoadBalancerStatus, error) {
	glog.V(3).Infof("EnsureLoadBalancer(%v, %v, %v)", clusterName, service, nodes)

	tcpPortNum := 0
	k8sTCPPorts := []int{}
	k8sNodePorts := []int{}
	for _, port := range service.Spec.Ports {
		if port.Protocol == v1.ProtocolUDP {
			glog.Warningf("qingcloud not support udp port, skip [%v]", port.Port)
		} else {
			k8sTCPPorts = append(k8sTCPPorts, int(port.Port))
			k8sNodePorts = append(k8sNodePorts, int(port.NodePort))
			tcpPortNum++
		}
	}
	if tcpPortNum == 0 {
		return nil, fmt.Errorf("requested load balancer with no tcp ports")
	}
	if len(nodes) == 0 {
		return nil, fmt.Errorf("requested load balancer with empty nodes")
	}
	// QingCloud does not support user-specified ip addr for LB. We just
	// print some log and ignore the public ip.
	if service.Spec.LoadBalancerIP != "" {
		glog.Warningf("Public IP[%v] cannot be specified for qingcloud LB", service.Spec.LoadBalancerIP)
	}

	// get lb properties from k8s service spec
	lbType := service.Annotations[ServiceAnnotationLoadBalancerType]
	if lbType != "0" && lbType != "1" && lbType != "2" && lbType != "3" && lbType != "4" && lbType != "5" {
		lbType = "0"
	}
	loadBalancerType, _ := strconv.Atoi(lbType)

	lbEipIds, hasEip := service.Annotations[ServiceAnnotationLoadBalancerEipIds]
	vxnetId, hasVxnet := service.Annotations[ServiceAnnotationLoadBalancerVxnetId]

	balanceMode := "roundrobin"
	if service.Spec.SessionAffinity == v1.ServiceAffinityClientIP {
		balanceMode = "source"
	}

	loadBalancerName := qc.getQingCloudLoadBalancerName(service)

	instances := []string{}
	for _, node := range nodes {
		instances = append(instances, NodeNameToInstanceID(types.NodeName(node.Name)))
	}

	glog.V(2).Infof("Checking if qingcloud load balancer already exists: %s", loadBalancerName)
	loadBalancer, err := qc.getLoadBalancerByName(loadBalancerName)
	if err != nil {
		return nil, fmt.Errorf("Error checking if qingcloud load balancer already exists: %v", err)
	}

	// TODO: Implement a more efficient update strategy for common changes than delete & create
	// In particular, if we implement nodes update, we can get rid of UpdateHosts
	if loadBalancer != nil && *loadBalancer.Status != "ceased" {
		// enforce the loadBalancer config
		var needUpdate bool
		qyEips, qyPrivateIps, qyEipIDs, err := qc.waitLoadBalancerActive(*loadBalancer.LoadBalancerID, operationWaitTimeout)
		if err != nil {
			glog.Error(err)
			return nil, err
		}
		sum := 0
		for {
			sum++
			// check lb type
			if loadBalancerType != *loadBalancer.LoadBalancerType {
				if loadBalancerType < *loadBalancer.LoadBalancerType {
					glog.V(1).Infof("Stop lb at first before resizing it because current lb type '%d' is bigger with the one in k8s servie spec '%d', ", *loadBalancer.LoadBalancerType, loadBalancerType)
					err := qc.stopLoadBalancer(*loadBalancer.LoadBalancerID)
					if err != nil {
						glog.Error(err)
						return nil, err
					}
				}
				glog.V(1).Infof("Resize lb type because current lb type '%d' is different with the one in k8s servie spec '%d'", *loadBalancer.LoadBalancerType, loadBalancerType)
				err := qc.resizeLoadBalancer(*loadBalancer.LoadBalancerID, loadBalancerType)
				if err != nil {
					glog.Error(err)
					return nil, err
				}
				needUpdate = true
				if loadBalancerType < *loadBalancer.LoadBalancerType {
					glog.V(1).Infof("Start lb now after resizing it to take effect because previous lb type '%d' is bigger with the one in k8s servie spec '%d', ", *loadBalancer.LoadBalancerType, loadBalancerType)
					err := qc.startLoadBalancer(*loadBalancer.LoadBalancerID)
					if err != nil {
						glog.Error(err)
						return nil, err
					}
				}
			}
			// check eip and vxnet
			if hasEip {
				if *loadBalancer.VxNetID != "" && *loadBalancer.VxNetID != "vxnet-0" {
					glog.V(1).Infof("This LB '%s' used to assign with vxnet '%s', need to delete this lb and recreate it", *loadBalancer.LoadBalancerID, *loadBalancer.VxNetID)
					err := qc.deleteLoadBalancerAndSecurityGrp(*loadBalancer.LoadBalancerID, loadBalancer.SecurityGroupID)
					if err != nil {
						glog.Error(err)
						return nil, err
					}
					needUpdate = false
					break
				}

				k8sLoadBalancerEipIds := strings.Split(lbEipIds, ",")
				glog.V(1).Infof("Calvin -------11111---- print all k8sEip '%s'", lbEipIds)
				for _, qyEipID := range qyEipIDs {
					glog.V(1).Infof("Calvin ----322222------- print all qyEip '%s'", qyEipID)
				}
				for _, k8sEipID := range k8sLoadBalancerEipIds {
					if stringIndex(qyEipIDs, k8sEipID) < 0 {
						glog.V(1).Infof("Associate new EIP '%s' to LB '%s'", k8sEipID, *loadBalancer.LoadBalancerID)
						err := qc.associateEipToLoadBalancer(*loadBalancer.LoadBalancerID, k8sEipID)
						if err != nil {
							glog.Error(err)
							return nil, err
						}
						needUpdate = true
					}
				}
				for _, qyEipID := range qyEipIDs {
					if stringIndex(k8sLoadBalancerEipIds, qyEipID) < 0 {
						glog.V(1).Infof("dissociate EIP '%s' from LB '%s'", qyEipID, *loadBalancer.LoadBalancerID)
						err := qc.dissociateEipFromLoadBalancer(*loadBalancer.LoadBalancerID, qyEipID)
						if err != nil {
							glog.Error(err)
							return nil, err
						}
						needUpdate = true
					}
				}
			} else if hasVxnet {
				if vxnetId != *loadBalancer.VxNetID || len(qyEipIDs) > 0 {
					err := qc.deleteLoadBalancerAndSecurityGrp(*loadBalancer.LoadBalancerID, loadBalancer.SecurityGroupID)
					if err != nil {
						glog.Error(err)
						return nil, err
					}
					needUpdate = false
					break
				}
			}

			// check lisener: balance mode and port, add/update/delete listener
			qyLbListeners, err := qc.getLoadBalancerListeners(*loadBalancer.LoadBalancerID)
			if err != nil {
				glog.Error(err)
				return nil, err
			}
			if len(qyLbListeners) == 0 {
				err := fmt.Errorf("No listners under this load balancer '%s'", *loadBalancer.LoadBalancerID)
				glog.Error(err)
				return nil, err
			}
			qyLbListenerPorts := []int{}
			for _, qyLbListerner := range qyLbListeners {
				// sum all existing listeners' port on qingcloud
				qyLbListenerPorts = append(qyLbListenerPorts, *qyLbListerner.ListenerPort)
				if intIndex(k8sTCPPorts, *qyLbListerner.ListenerPort) < 0 {
					// this listener/port is already remove from spec, so just delete it
					err := qc.deleteLoadBalancerListener(*qyLbListerner.LoadBalancerListenerID)
					if err != nil {
						glog.Error(err)
						return nil, err
					}
					needUpdate = true
				}
			}
			for i, k8sPort := range k8sTCPPorts {
				// this is the index to locate the listener on qingcloud
				qyLbListenerPos := intIndex(qyLbListenerPorts, k8sPort)
				if qyLbListenerPos >= 0 {
					// so port in spec matches existing listener's port, then check if balance mode is modified in spec, if yes, modify listener's attr
					if balanceMode != *qyLbListeners[qyLbListenerPos].BalanceMode {
						err := qc.modifyLoadBalancerListenerAttributes(*qyLbListeners[qyLbListenerPos].LoadBalancerListenerID, *qyLbListeners[qyLbListenerPos].LoadBalancerListenerName, balanceMode)
						if err != nil {
							glog.Error(err)
							return nil, err
						}
						needUpdate = true
					}
				} else {
					// so this is new port from spec, will create a new listener for it with backend nodes
					_, err := qc.createLoadBalancerListenerWithBackends(*loadBalancer.LoadBalancerID, k8sPort, k8sNodePorts[i], balanceMode, instances)
					if err != nil {
						glog.Errorf("Couldn't create loadBalancerListener with backends")
						glog.Error(err)
						return nil, err
					}
					needUpdate = true
				}
			}
			break
		}
		if needUpdate {
			glog.V(1).Info("Update loadbalance because of service spec change")
			err = qc.updateLoadBalancer(*loadBalancer.LoadBalancerID)
			if err != nil {
				glog.Errorf("Couldn't update loadBalancer '%s'", *loadBalancer.LoadBalancerID)
				return nil, err
			}

			status := &v1.LoadBalancerStatus{}
			if hasEip {
				for _, ip := range qyEips {
					status.Ingress = append(status.Ingress, v1.LoadBalancerIngress{IP: ip})
				}
			} else {
				for _, ip := range qyPrivateIps {
					status.Ingress = append(status.Ingress, v1.LoadBalancerIngress{IP: ip})
				}
			}
			glog.Infof("Start loadBalancer '%v', ingress ip '%v'", loadBalancerName, status.Ingress)

			return status, nil
		}
	}
	glog.Infof("Create loadBalancer '%s' in zone '%s'", loadBalancerName, qc.zone)
	var loadBalancerID string
	if hasEip {
		if hasVxnet {
			err := fmt.Errorf("Both ServiceAnnotationLoadBalancerVxnetId and ServiceAnnotationLoadBalancerEipIds are set. Please set only one of them")
			glog.Error(err)
			return nil, err
		}
		loadBalancerEipIds := strings.Split(lbEipIds, ",")
		loadBalancerID, err = qc.createLoadBalancerWithEips(loadBalancerName, loadBalancerType, loadBalancerEipIds)
		if err != nil {
			glog.Errorf("Error creating loadBalancer '%s': %v", loadBalancerName, err)
			return nil, err
		}
	} else if hasVxnet {
		loadBalancerID, err = qc.createLoadBalancerWithVxnet(loadBalancerName, loadBalancerType, vxnetId)
		if err != nil {
			glog.Errorf("Error creating loadBalancer '%s': %v", loadBalancerName, err)
			return nil, err
		}
	} else {
		// use vxnetId of k8s cluster to create new load balancer
		if qc.vxNetId != "" {
			glog.Infof("As no other load balancer properties specified but set vxnet id in qingcloud.config, just create loadBalancer '%s' in zone '%s' with vxnetid '$s", loadBalancerName, qc.zone, qc.vxNetId)
			loadBalancerID, err = qc.createLoadBalancerWithVxnet(loadBalancerName, loadBalancerType, vxnetId)
			if err != nil {
				glog.Errorf("Error creating loadBalancer '%s': %v", loadBalancerName, err)
				return nil, err
			}
		} else {
			err := fmt.Errorf("Both ServiceAnnotationLoadBalancerVxnetId and ServiceAnnotationLoadBalancerEipIds are not set. Please set only one of them")
			glog.Error(err)
			return nil, err
		}
	}
	eips, privateIps, _, err := qc.waitLoadBalancerActive(loadBalancerID, operationWaitTimeout)
	if err != nil {
		glog.Error(err)
		return nil, err
	}
	// For every port(qingcloud only support tcp), we need a listener.
	for i, port := range k8sTCPPorts {
		_, err := qc.createLoadBalancerListenerWithBackends(loadBalancerID, port, k8sNodePorts[i], balanceMode, instances)
		if err != nil {
			glog.Errorf("Couldn't create loadBalancerListener with backends")
			glog.Error(err)
			return nil, err
		}
	}
	// enforce the loadBalancer config
	err = qc.updateLoadBalancer(loadBalancerID)
	if err != nil {
		glog.Errorf("Couldn't update loadBalancer '%v'", loadBalancerID)
		return nil, err
	}

	status := &v1.LoadBalancerStatus{}
	if hasEip {
		for _, ip := range eips {
			status.Ingress = append(status.Ingress, v1.LoadBalancerIngress{IP: ip})
		}
	} else {
		for _, ip := range privateIps {
			status.Ingress = append(status.Ingress, v1.LoadBalancerIngress{IP: ip})
		}
	}
	glog.Infof("Start loadBalancer '%v', ingress ip '%v'", loadBalancerName, status.Ingress)

	return status, nil
}

// UpdateLoadBalancer updates hosts under the specified load balancer.
func (qc *QingCloud) UpdateLoadBalancer(clusterName string, service *v1.Service, nodes []*v1.Node) error {
	loadBalancerName := qc.getQingCloudLoadBalancerName(service)
	glog.V(3).Infof("UpdateLoadBalancer(%v, %v, %v)", clusterName, loadBalancerName, nodes)

	loadBalancer, err := qc.getLoadBalancerByName(loadBalancerName)
	if err != nil {
		return err
	}
	if loadBalancer == nil {
		return fmt.Errorf("Couldn't find load balancer by name '%s' in zone '%s'", loadBalancerName, qc.zone)
	}

	if len(nodes) == 0 {
		return nil
	}

	// Expected instances for the load balancer.
	expected := sets.NewString()
	for _, node := range nodes {
		instanceID := NodeNameToInstanceID(types.NodeName(node.Name))
		expected.Insert(instanceID)
	}

	listenerBackendPorts := map[string]int32{}
	instanceBackendIDs := map[string][]string{}
	loadBalancerListeners, err := qc.getLoadBalancerListeners(*loadBalancer.LoadBalancerID)
	if err != nil {
		glog.Errorf("Couldn't get loadBalancer '%v' err: %v", loadBalancerName, err)
		return err
	}
	for _, listener := range loadBalancerListeners {
		nodePort, found := getNodePort(service, int32(*listener.ListenerPort), v1.ProtocolTCP)
		if !found {
			continue
		}
		listenerBackendPorts[*listener.LoadBalancerListenerID] = nodePort

		for _, backend := range listener.Backends {
			instanceBackendIDs[*backend.ResourceID] = append(instanceBackendIDs[*backend.ResourceID], *backend.LoadBalancerBackendID)
		}
	}

	// Actual instances of the load balancer.
	actual := sets.StringKeySet(instanceBackendIDs)
	addInstances := expected.Difference(actual)
	removeInstances := actual.Difference(expected)

	var needUpdate bool

	glog.V(3).Infof("For the loadBalancer, expected instances: %v, actual instances: %v, need to remove instances: %v, need to add instances: %v", expected, actual, removeInstances, addInstances)

	if len(addInstances) > 0 {
		instances := addInstances.List()
		for listenerID, port := range listenerBackendPorts {
			backends := make([]*qcservice.LoadBalancerBackend, len(instances))
			for i, instance := range instances {
				//copy for get address.
				instanceID := instance
				backends[i] = &qcservice.LoadBalancerBackend{
					ResourceID:              &instanceID,
					LoadBalancerBackendName: &instanceID,
					Port: qcservice.Int(int(port)),
				}
			}
			err := qc.addLoadBalancerBackends(listenerID, backends)
			if err != nil {
				glog.Errorf("Couldn't add backend servers '%v' to loadBalancer '%s': %v, err: %s", instances, loadBalancerName, loadBalancer, err.Error())
				return err
			}
			needUpdate = true
		}

		glog.V(1).Infof("Instances '%v' added to loadBalancer %s", instances, loadBalancerName)
	}

	if len(removeInstances) > 0 {
		instances := removeInstances.List()

		backendIDs := make([]string, 0, len(instances))
		for _, instance := range instances {
			backendIDs = append(backendIDs, instanceBackendIDs[instance]...)
		}
		err := qc.deleteLoadBalancerBackends(backendIDs)
		if err != nil {
			glog.Errorf("Couldn't remove backend servers '%v' from loadBalancer '%s': %v, err: %s", instances, loadBalancerName, loadBalancer, err.Error())
			return err
		}
		needUpdate = true
		glog.V(1).Infof("Instances '%v' removed from loadBalancer %s", instances, loadBalancerName)
	}

	if needUpdate {
		glog.V(1).Info("Enforce the loadBalancer update backends config")
		return qc.updateLoadBalancer(*loadBalancer.LoadBalancerID)
	}

	glog.V(3).Info("Skip update loadBalancer backends")

	return nil
}

// EnsureLoadBalancerDeleted deletes the specified load balancer if it
// exists, returning nil if the load balancer specified either didn't exist or
// was successfully deleted.
// This construction is useful because many cloud providers' load balancers
// have multiple underlying components, meaning a Get could say that the LB
// doesn't exist even if some part of it is still laying around.
func (qc *QingCloud) EnsureLoadBalancerDeleted(clusterName string, service *v1.Service) error {
	loadBalancerName := qc.getQingCloudLoadBalancerName(service)
	glog.V(3).Infof("EnsureLoadBalancerDeleted(%v, %v)", clusterName, loadBalancerName)

	loadBalancer, err := qc.getLoadBalancerByName(loadBalancerName)
	if err != nil {
		return err
	}
	if loadBalancer == nil {
		return nil
	}

	err = qc.deleteLoadBalancer(*loadBalancer.LoadBalancerID)
	if err != nil {
		return err
	}
	err = qc.DeleteSecurityGroup(loadBalancer.SecurityGroupID)
	if err != nil {
		glog.Errorf("Delete SecurityGroup '%s' err '%s' ", *loadBalancer.SecurityGroupID, err)
	}
	glog.Infof("Delete loadBalancer '%s' in zone '%s'", loadBalancerName, qc.zone)

	return nil
}

func (qc *QingCloud) getQingCloudLoadBalancerName(service *v1.Service) string {
	return fmt.Sprintf("k8s_%s_%s", service.Name, cloudprovider.GetLoadBalancerName(service))
}

func (qc *QingCloud) createLoadBalancerWithEips(lbName string, lbType int, lbEipIds []string) (string, error) {
	sgID, err := qc.ensureLoadBalancerSecurityGroup(lbName)
	if err != nil {
		return "", err
	}
	output, err := qc.lbService.CreateLoadBalancer(&qcservice.CreateLoadBalancerInput{
		EIPs:             qcservice.StringSlice(lbEipIds),
		LoadBalancerType: qcservice.Int(lbType),
		LoadBalancerName: qcservice.String(lbName),
		SecurityGroup:    sgID,
	})
	if err != nil {
		return "", err
	}
	qc.waitLoadBalancerActive(*output.LoadBalancerID, operationWaitTimeout)
	return *output.LoadBalancerID, nil
}

func (qc *QingCloud) createLoadBalancerWithVxnet(lbName string, lbType int, vxnetID string) (string, error) {
	sgID, err := qc.ensureLoadBalancerSecurityGroup(lbName)
	if err != nil {
		return "", err
	}
	output, err := qc.lbService.CreateLoadBalancer(&qcservice.CreateLoadBalancerInput{
		VxNet:            &vxnetID,
		LoadBalancerType: qcservice.Int(lbType),
		LoadBalancerName: qcservice.String(lbName),
		SecurityGroup:    sgID,
	})
	if err != nil {
		return "", err
	}
	qc.waitLoadBalancerActive(*output.LoadBalancerID, operationWaitTimeout)
	return *output.LoadBalancerID, nil
}

func (qc *QingCloud) deleteLoadBalancer(loadBalancerID string) error {
	output, err := qc.lbService.DeleteLoadBalancers(&qcservice.DeleteLoadBalancersInput{LoadBalancers: []*string{qcservice.String(loadBalancerID)}})
	if err != nil {
		return err
	}
	qcclient.WaitJob(qc.jobService, *output.JobID, operationWaitTimeout, waitInterval)
	return err
}

func (qc *QingCloud) addLoadBalancerBackends(loadBalancerListenerID string, backends []*qcservice.LoadBalancerBackend) error {
	_, err := qc.lbService.AddLoadBalancerBackends(&qcservice.AddLoadBalancerBackendsInput{
		Backends:             backends,
		LoadBalancerListener: &loadBalancerListenerID,
	})
	if err != nil {
		return err
	}
	return nil
}

func (qc *QingCloud) deleteLoadBalancerBackends(loadBalancerBackends []string) error {
	_, err := qc.lbService.DeleteLoadBalancerBackends(&qcservice.DeleteLoadBalancerBackendsInput{
		LoadBalancerBackends: qcservice.StringSlice(loadBalancerBackends),
	})
	return err
}

func (qc *QingCloud) addLoadBalancerListener(loadBalancerID string, listenerPort int, balanceMode string) (string, error) {

	output, err := qc.lbService.AddLoadBalancerListeners(&qcservice.AddLoadBalancerListenersInput{
		LoadBalancer: &loadBalancerID,
		Listeners: []*qcservice.LoadBalancerListener{
			{
				ListenerProtocol: qcservice.String("tcp"),
				BackendProtocol:  qcservice.String("tcp"),
				BalanceMode:      &balanceMode,
				ListenerPort:     &listenerPort,
			},
		},
	})
	if err != nil {
		return "", err
	}

	return *output.LoadBalancerListeners[0], nil
}

func (qc *QingCloud) getLoadBalancerByName(name string) (*qcservice.LoadBalancer, error) {
	status := []*string{qcservice.String("pending"), qcservice.String("active"), qcservice.String("stopped"), qcservice.String("ceased")}
	output, err := qc.lbService.DescribeLoadBalancers(&qcservice.DescribeLoadBalancersInput{
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

func (qc *QingCloud) getLoadBalancerByID(id string) (*qcservice.LoadBalancer, bool, error) {
	output, err := qc.lbService.DescribeLoadBalancers(&qcservice.DescribeLoadBalancersInput{
		LoadBalancers: []*string{&id},
	})
	if err != nil {
		return nil, false, err
	}
	if len(output.LoadBalancerSet) == 0 {
		return nil, false, nil
	}
	lb := output.LoadBalancerSet[0]
	if *lb.Status == qcclient.LoadBalancerStatusCeased || *lb.Status == qcclient.LoadBalancerStatusDeleted {
		return nil, false, nil
	}
	return lb, true, nil
}

func (qc *QingCloud) getLoadBalancerListeners(loadBalancerID string) ([]*qcservice.LoadBalancerListener, error) {
	loadBalancerListeners := []*qcservice.LoadBalancerListener{}

	for i := 0; ; i += pageLimt {
		resp, err := qc.lbService.DescribeLoadBalancerListeners(&qcservice.DescribeLoadBalancerListenersInput{
			LoadBalancer: &loadBalancerID,
			Verbose:      qcservice.Int(1),
			Offset:       qcservice.Int(i),
			Limit:        qcservice.Int(pageLimt),
		})
		if err != nil {
			return nil, err
		}
		if len(resp.LoadBalancerListenerSet) == 0 {
			break
		}

		loadBalancerListeners = append(loadBalancerListeners, resp.LoadBalancerListenerSet...)
		if len(loadBalancerListeners) >= *resp.TotalCount {
			break
		}
	}

	return loadBalancerListeners, nil
}

// enforce the loadBalancer config
func (qc *QingCloud) updateLoadBalancer(loadBalancerID string) error {
	output, err := qc.lbService.UpdateLoadBalancers(&qcservice.UpdateLoadBalancersInput{
		LoadBalancers: []*string{&loadBalancerID},
	})
	if err != nil {
		return err
	}
	qcclient.WaitJob(qc.jobService, *output.JobID, operationWaitTimeout, waitInterval)
	qc.waitLoadBalancerActive(loadBalancerID, operationWaitTimeout)
	return nil
}

func (qc *QingCloud) waitLoadBalancerActive(loadBalancerID string, timeout time.Duration) ([]string, []string, []string, error) {
	loadBalancer, err := qcclient.WaitLoadBalancerStatus(qc.lbService, loadBalancerID, qcclient.LoadBalancerStatusActive, timeout, waitInterval)
	if err == nil {
		eips := []string{}
		eipIDs := []string{}
		privateIps := []string{}
		for _, eip := range loadBalancer.Cluster {
			eipIDs = append(eipIDs, *eip.EIPID)
			eips = append(eips, *eip.EIPAddr)
		}
		for _, pip := range loadBalancer.PrivateIPs {
			privateIps = append(privateIps, *pip)
		}
		return eips, privateIps, eipIDs, nil
	}
	return nil, nil, nil, err
}

func (qc *QingCloud) waitLoadBalancerDelete(loadBalancerID string, timeout time.Duration) error {
	_, err := qcclient.WaitLoadBalancerStatus(qc.lbService, loadBalancerID, qcclient.LoadBalancerStatusDeleted, timeout, waitInterval)
	return err
}

func (qc *QingCloud) ensureLoadBalancerSecurityGroup(loadBalancerName string) (*string, error) {
	sg, err := qc.getSecurityGroupByName(loadBalancerName)
	if err != nil {
		return nil, err
	}
	var sgID *string
	if sg != nil {
		sgID = sg.SecurityGroupID
	} else {
		sg, err := qc.createSecurityGroup(&loadBalancerName, defaultLBSecurityGroupRules)
		if err != nil {
			return nil, err
		}
		sgID = sg.SecurityGroupID
	}
	return sgID, nil
}

func (qc *QingCloud) getSecurityGroupByName(name string) (*qcservice.SecurityGroup, error) {
	input := &qcservice.DescribeSecurityGroupsInput{SearchWord: &name}
	output, err := qc.securityGroupService.DescribeSecurityGroups(input)
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

func (qc *QingCloud) createSecurityGroup(sgName *string, rules []*qcservice.SecurityGroupRule) (*qcservice.SecurityGroup, error) {
	createInput := &qcservice.CreateSecurityGroupInput{SecurityGroupName: sgName}
	createOutput, err := qc.securityGroupService.CreateSecurityGroup(createInput)
	if err != nil {
		return nil, err
	}
	sgID := createOutput.SecurityGroupID
	input := &qcservice.DescribeSecurityGroupsInput{SecurityGroups: []*string{sgID}}
	output, err := qc.securityGroupService.DescribeSecurityGroups(input)
	if err != nil {
		return nil, err
	}
	sg := output.SecurityGroupSet[0]
	err = qc.addSecurityRule(sg.SecurityGroupID, rules)
	if err != nil {
		return sg, err
	}
	qc.securityGroupService.ApplySecurityGroup(&qcservice.ApplySecurityGroupInput{SecurityGroup: sg.SecurityGroupID})
	return sg, nil
}

func (qc *QingCloud) addSecurityRule(sgID *string, rules []*qcservice.SecurityGroupRule) error {
	addRuleInput := &qcservice.AddSecurityGroupRulesInput{SecurityGroup: sgID, Rules: rules}
	addRuleOutput, err := qc.securityGroupService.AddSecurityGroupRules(addRuleInput)
	if err != nil {
		return err
	}
	glog.V(4).Infof("AddSecurityGroupRules SecurityGroup: [%s], output: [%+v] ", *sgID, addRuleOutput)
	return nil
}

func (qc *QingCloud) DeleteSecurityGroup(sgID *string) error {
	input := &qcservice.DeleteSecurityGroupsInput{SecurityGroups: []*string{sgID}}
	_, err := qc.securityGroupService.DeleteSecurityGroups(input)
	if err != nil {
		return err
	}
	return nil
}

func (qc *QingCloud) createLoadBalancerListenerWithBackends(loadBalancerID string, port int, nodePort int, balanceMode string, instances []string) (string, error) {
	listenerID, err := qc.addLoadBalancerListener(loadBalancerID, port, balanceMode)

	if err != nil || listenerID == "" {
		glog.Errorf("Error create loadBalancer TCP listener (LoadBalancerId:'%s', Port: '%v'): %v", loadBalancerID, port, err)
		return "", err
	}
	glog.Infof("Created LoadBalancerTCPListener (LoadBalancerId:'%s', Port: '%v', listenerID: '%s')", loadBalancerID, port, listenerID)

	backends := make([]*qcservice.LoadBalancerBackend, len(instances))
	for j, instance := range instances {
		//copy for get address.
		instanceID := instance
		backends[j] = &qcservice.LoadBalancerBackend{
			ResourceID:              &instanceID,
			LoadBalancerBackendName: &instanceID,
			Port: qcservice.Int(int(nodePort)),
		}
	}
	if len(backends) > 0 {
		err = qc.addLoadBalancerBackends(listenerID, backends)
		if err != nil {
			glog.Errorf("Couldn't add backend servers '%v' to loadBalancer with id '%v': %v", instances, loadBalancerID, err)
			return "", err
		}
		glog.V(3).Infof("Added backend servers '%v' to loadBalancer with id '%s'", instances, loadBalancerID)
	}

	return listenerID, nil
}

func (qc *QingCloud) resizeLoadBalancer(loadBalancerID string, loadBalancerType int) error {
	glog.V(2).Infof("Starting resize loadBalancer '%s'", loadBalancerID)
	output, err := qc.lbService.ResizeLoadBalancers(&qcservice.ResizeLoadBalancersInput{
		LoadBalancerType: &loadBalancerType,
		LoadBalancers:    []*string{qcservice.String(loadBalancerID)},
	})
	if err != nil {
		return err
	}
	qcclient.WaitJob(qc.jobService, *output.JobID, operationWaitTimeout, waitInterval)
	return err
}

func (qc *QingCloud) associateEipToLoadBalancer(loadBalancerID string, eip string) error {
	glog.V(2).Infof("Starting associate Eip %s to loadBalancer '%s'", eip, loadBalancerID)
	output, err := qc.lbService.AssociateEIPsToLoadBalancer(&qcservice.AssociateEIPsToLoadBalancerInput{
		EIPs:         []*string{qcservice.String(eip)},
		LoadBalancer: &loadBalancerID,
	})
	if err != nil {
		return err
	}
	qcclient.WaitJob(qc.jobService, *output.JobID, operationWaitTimeout, waitInterval)
	return err
}
func (qc *QingCloud) dissociateEipFromLoadBalancer(loadBalancerID string, eip string) error {
	glog.V(2).Infof("Starting dissociate Eip %s from loadBalancer '%s'", eip, loadBalancerID)
	output, err := qc.lbService.DissociateEIPsFromLoadBalancer(&qcservice.DissociateEIPsFromLoadBalancerInput{
		EIPs:         []*string{qcservice.String(eip)},
		LoadBalancer: &loadBalancerID,
	})
	if err != nil {
		return err
	}
	qcclient.WaitJob(qc.jobService, *output.JobID, operationWaitTimeout, waitInterval)
	return err
}

func (qc *QingCloud) deleteLoadBalancerAndSecurityGrp(loadBalancerID string, securityGroupID *string) error {
	glog.Infof("Starting delete loadBalancer '%s' before creating", loadBalancerID)
	err := qc.deleteLoadBalancer(loadBalancerID)
	if err != nil {
		glog.V(1).Infof("Deleted loadBalancer '%s' error before creating: %v", loadBalancerID, err)
		return err
	}
	err = qc.waitLoadBalancerDelete(loadBalancerID, operationWaitTimeout)
	if err != nil {
		glog.Error(err)
		return err
	}
	err = qc.DeleteSecurityGroup(securityGroupID)
	if err != nil {
		glog.Errorf("Delete SecurityGroup '%v' err '%s' ", &securityGroupID, err)
	}
	glog.Infof("Done, deleted loadBalancer '%s'", loadBalancerID)
	return nil
}

func (qc *QingCloud) deleteLoadBalancerListener(loadBalancerListenerID string) error {
	glog.Infof("Deleting LoadBalancerTCPListener :'%s'", loadBalancerListenerID)
	output, err := qc.lbService.DeleteLoadBalancerListeners(&qcservice.DeleteLoadBalancerListenersInput{
		LoadBalancerListeners: []*string{qcservice.String(loadBalancerListenerID)},
	})
	if err != nil {
		return err
	}
	if *output.RetCode != 0 {
		err := fmt.Errorf("Fail to delete loadbalancer lisener '%s' because of '%s'", loadBalancerListenerID, *output.Message)
		return err
	}
	return nil
}

func (qc *QingCloud) modifyLoadBalancerListenerAttributes(loadBalancerListenerID string, loadBalancerListenerName string, balanceMode string) error {
	glog.Infof("Modifying balanceMode of LoadBalancerTCPListener :'%s'", loadBalancerListenerID)
	output, err := qc.lbService.ModifyLoadBalancerListenerAttributes(&qcservice.ModifyLoadBalancerListenerAttributesInput{
		LoadBalancerListener:     &loadBalancerListenerID,
		LoadBalancerListenerName: &loadBalancerListenerName,
		BalanceMode:              &balanceMode,
	})
	if err != nil {
		return err
	}
	if *output.RetCode != 0 {
		err := fmt.Errorf("Fail to modify balanceMode of loadbalancer lisener '%s' because of '%s'", loadBalancerListenerID, *output.Message)
		return err
	}
	return nil
}
func (qc *QingCloud) stopLoadBalancer(loadBalancerID string) error {
	glog.V(2).Infof("Stopping loadBalancer '%s'", loadBalancerID)
	output, err := qc.lbService.StopLoadBalancers(&qcservice.StopLoadBalancersInput{
		LoadBalancers: []*string{qcservice.String(loadBalancerID)},
	})
	if err != nil {
		return err
	}
	qcclient.WaitJob(qc.jobService, *output.JobID, operationWaitTimeout, waitInterval)
	return err
}

func (qc *QingCloud) startLoadBalancer(loadBalancerID string) error {
	glog.V(2).Infof("Starting loadBalancer '%s'", loadBalancerID)
	output, err := qc.lbService.StartLoadBalancers(&qcservice.StartLoadBalancersInput{
		LoadBalancers: []*string{qcservice.String(loadBalancerID)},
	})
	if err != nil {
		return err
	}
	qcclient.WaitJob(qc.jobService, *output.JobID, operationWaitTimeout, waitInterval)
	return err
}

//ModifyLoadBalancerListenerAttributes
