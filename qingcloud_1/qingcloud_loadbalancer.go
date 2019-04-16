// Copyright 2017 Yunify Inc. All rights reserved.
// Use of this source code is governed by a Apache license
// that can be found in the LICENSE file.

package qingcloud

// See https://docs.qingcloud.com/api/lb/index.html
// qingcloud loadBalancer and instance have a default strict Security Group(firewall),
// its only allow SSH and PING. So, first of all, you shoud manually add correct rules
// for all nodes and loadBalancers. You can simply add a rule by pass all tcp port traffic.
// The loadBalancers also need at least one EIP before create it, please allocate some EIPs,
// and set them in service annotation ServiceAnnotationLoadBalancerEipIds.

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	qcservice "github.com/yunify/qingcloud-sdk-go/service"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog"
)

const (
	// ServiceAnnotationLoadBalancerEipIds is the annotation which specifies a list of eip ids.
	// The ids in list are separated by ',', e.g. "eip-j38f2h3h,eip-ornz2xq7". And this annotation should
	// NOT be used with ServiceAnnotationLoadBalancerVxnetId. Please make sure there is one and only one
	// of them being set
	ServiceAnnotationLoadBalancerEipIds = "service.beta.kubernetes.io/qingcloud-load-balancer-eip-ids"

	// ServiceAnnotationLoadBalancerVxnetId is the annotation which indicates the very vxnet where load
	// balancer resides. This annotation should NOT be used when ServiceAnnotationLoadBalancerEipIds is
	// set.
	//
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

	ServiceAnnotationLoadBalancerEipStrategy = "service.beta.kubernetes.io/qingcloud-load-balancer-eip-strategy"
)

type EIPStrategy string

const (
	ReuseEIP  EIPStrategy = "reuse"
	Exclusive EIPStrategy = "exclusive"
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
func (qc *QingCloud) GetLoadBalancer(ctx context.Context, clusterName string, service *v1.Service) (status *v1.LoadBalancerStatus, exists bool, err error) {
	loadBalancerName := qc.GetLoadBalancerName(ctx, clusterName, service)
	klog.V(3).Infof("GetLoadBalancer(%v, %v)", clusterName, loadBalancerName)

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

// GetLoadBalancerName returns the name of the load balancer. Implementations must treat the
// *v1.Service parameter as read-only and not modify it.
func (qc *QingCloud) GetLoadBalancerName(ctx context.Context, clusterName string, service *v1.Service) string {
	defaultName := fmt.Sprintf("k8s_lb_%s_%s_%s", clusterName, service.Name, GetFirstUID(string(service.UID)))
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

// EnsureLoadBalancer creates a new load balancer 'name', or updates the existing one. Returns the status of the balancer
// To create a LoadBalancer for kubernetes, we do the following:
// 1. create a qingcloud loadBalancer;
// 2. create listeners for the new loadBalancer, number of listeners = number of service ports;
// 3. add backends to the new loadBalancer.
// will update this LB for below cases, otherwise, deleting existing one and recreate it
// 1. LB type is changed
// 2. balance mode is changed
// 3. previously use eip and now still use another eip
// 4. ports is different with previous setting, this will just bring changes on listeners
func (qc *QingCloud) EnsureLoadBalancer(ctx context.Context, clusterName string, service *v1.Service, nodes []*v1.Node) (*v1.LoadBalancerStatus, error) {
	klog.V(2).Infof("EnsureLoadBalancer(%v, %v)", clusterName, service.Name)

	k8sTCPPorts, k8sNodePorts := GetPortsOfService(service)
	if len(k8sTCPPorts) == 0 {
		return nil, fmt.Errorf("requested load balancer with no tcp ports")
	}
	if len(nodes) == 0 {
		return nil, fmt.Errorf("requested load balancer with empty nodes")
	}
	// QingCloud does not support user-specified ip addr for LB. We just
	// print some log and ignore the public ip.
	if service.Spec.LoadBalancerIP != "" {
		klog.Warningf("Public IP[%v] cannot be specified for qingcloud LB", service.Spec.LoadBalancerIP)
	}

	// get lb properties from k8s service spec
	lbType := service.Annotations[ServiceAnnotationLoadBalancerType]
	if lbType != "0" && lbType != "1" && lbType != "2" && lbType != "3" && lbType != "4" && lbType != "5" {
		lbType = "0"
	}
	loadBalancerType, _ := strconv.Atoi(lbType)

	lbEipIds, hasEip := service.Annotations[ServiceAnnotationLoadBalancerEipIds]

	balanceMode := "roundrobin"
	if service.Spec.SessionAffinity == v1.ServiceAffinityClientIP {
		balanceMode = "source"
	}

	loadBalancerName := qc.GetLoadBalancerName(ctx, clusterName, service)

	instances := []string{}
	for _, node := range nodes {
		instances = append(instances, qc.NodeNameToInstanceID(types.NodeName(node.Name)))
	}

	klog.V(2).Infof("Checking if qingcloud load balancer already exists: %s", loadBalancerName)
	loadBalancer, err := qc.getLoadBalancerByName(loadBalancerName)
	if err != nil {
		return nil, fmt.Errorf("Error checking if qingcloud load balancer already exists: %v", err)
	}

	if loadBalancer != nil {
		klog.V(1).Infof("LB '%s' is existed in this k8s cluster, will compare LB settng with related attributes in service spec, if anything is changed, , will update this LB ", *loadBalancer.LoadBalancerID)
		qyEips, qyPrivateIps, qyEipIDs := qc.getLoadBalancerNetConfig(loadBalancer)
		// check lisener: balance mode and port, add/update/delete listener
		qyLbListeners, err := qc.getLoadBalancerListeners(*loadBalancer.LoadBalancerID)
		if err != nil {
			klog.Error(err)
			return nil, err
		}
		comparedLoadBalancer, err := qc.compareSpecAndLoadBalancer(service, loadBalancer)
		if err != nil {
			return nil, fmt.Errorf("Error comparing LB settng with related attributes in service spec: %v", err)
		}
		comparedListeners := qc.compareSpecAndLoadBalancerListeners(qyLbListeners, k8sTCPPorts, balanceMode)

		switch comparedLoadBalancer {
		case "skip":
			switch comparedListeners {
			case "skip":
				status := &v1.LoadBalancerStatus{}
				if len(qyEips) > 0 {
					for _, ip := range qyEips {
						status.Ingress = append(status.Ingress, v1.LoadBalancerIngress{IP: ip})
					}
				} else {
					for _, ip := range qyPrivateIps {
						status.Ingress = append(status.Ingress, v1.LoadBalancerIngress{IP: ip})
					}
				}
				return status, nil
			case "update":
				err := qc.updateLoadBalancerListenersFromServiceSpec(*loadBalancer.LoadBalancerID, qyLbListeners, k8sTCPPorts, k8sNodePorts, balanceMode, instances)
				if err != nil {
					klog.Error(err)
					return nil, err
				}
				klog.V(1).Info("Update loadbalance because of service spec change")
				status, err := qc.updateLoadBalancer(loadBalancer)
				if err != nil {
					klog.Errorf("Couldn't update loadBalancer '%s'", *loadBalancer.LoadBalancerID)
					return nil, err
				}
				return status, nil
			}
		case "delete":
			err := qc.deleteLoadBalancerAndSecurityGrp(*loadBalancer.LoadBalancerID, loadBalancer.SecurityGroupID)
			if err != nil {
				klog.Error(err)
				return nil, err
			}
		case "update":
			// check lb type, if changing lb type to smaller value, qingcloud require to stop LB and then apply change.
			if loadBalancerType != *loadBalancer.LoadBalancerType {
				klog.V(1).Infof("Resize lb type because current lb type '%d' is different with the one in k8s servie spec '%d'", *loadBalancer.LoadBalancerType, loadBalancerType)
				err := qc.resizeLoadBalancer(*loadBalancer.LoadBalancerID, loadBalancerType, *loadBalancer.LoadBalancerType)
				if err != nil {
					klog.Error(err)
					return nil, err
				}
			}
			// check eip and vxnet
			// if eip is assigned in spec, will check if current lb was previously assigned with vxnet, if yes, delete this LB and recreate it later, otherwise, just dissociate old eip and assiciate new one
			// if vxnet is asigned i spec, and difference with previous setting, just delete this LB and recreate it later
			if hasEip {
				k8sLoadBalancerEipIds := strings.Split(lbEipIds, ",")
				for _, k8sEipID := range k8sLoadBalancerEipIds {
					if stringIndex(qyEipIDs, k8sEipID) < 0 {
						klog.V(1).Infof("Associate new EIP '%s' to LB '%s'", k8sEipID, *loadBalancer.LoadBalancerID)
						err := qc.associateEipToLoadBalancer(*loadBalancer.LoadBalancerID, k8sEipID)
						if err != nil {
							klog.Error(err)
							return nil, err
						}

					}
				}
				for _, qyEipID := range qyEipIDs {
					if stringIndex(k8sLoadBalancerEipIds, qyEipID) < 0 {
						klog.V(1).Infof("dissociate EIP '%s' from LB '%s'", qyEipID, *loadBalancer.LoadBalancerID)
						err := qc.dissociateEipFromLoadBalancer(*loadBalancer.LoadBalancerID, qyEipID)
						if err != nil {
							klog.Error(err)
							return nil, err
						}

					}
				}
			}

			// if len(qyLbListeners) == 0 {
			// 	err := fmt.Errorf("No listners under this load balancer '%s'", *loadBalancer.LoadBalancerID)
			// 	klog.Error(err)
			// 	return nil, err
			// }
			if comparedListeners == "update" {
				err := qc.updateLoadBalancerListenersFromServiceSpec(*loadBalancer.LoadBalancerID, qyLbListeners, k8sTCPPorts, k8sNodePorts, balanceMode, instances)
				if err != nil {
					klog.Error(err)
					return nil, err
				}
			}
			klog.V(1).Info("Update loadbalance because of service spec change")
			status, err := qc.updateLoadBalancer(loadBalancer)
			if err != nil {
				klog.Errorf("Couldn't update loadBalancer '%s'", *loadBalancer.LoadBalancerID)
				return nil, err
			}
			return status, nil
		}
	}
	klog.Infof("Create a new loadBalancer '%s' in zone '%s' after deleting the previous one", loadBalancerName, qc.zone)
	loadBalancerID, err := qc.createLoadBalancerFromServiceSpec(ctx, clusterName, service)
	if err != nil {
		klog.Error(err)
		return nil, err
	}
	// For every port(qingcloud only support tcp), we need a listener.
	for i, port := range k8sTCPPorts {
		_, err := qc.createLoadBalancerListenerWithBackends(loadBalancerID, port, k8sNodePorts[i], balanceMode, instances)
		if err != nil {
			klog.Errorf("Couldn't create loadBalancerListener with backends")
			klog.Error(err)
			return nil, err
		}
	}
	loadBalancer, err = qc.getLoadBalancerByName(loadBalancerName)
	if err != nil {
		return nil, fmt.Errorf("Error getting lb object after recreating it: %v", err)
	}
	status, err := qc.updateLoadBalancer(loadBalancer)
	if err != nil {
		klog.Errorf("Couldn't update loadBalancer '%v'", loadBalancerID)
		return nil, err
	}
	return status, nil
}

// UpdateLoadBalancer updates hosts under the specified load balancer.
func (qc *QingCloud) UpdateLoadBalancer(ctx context.Context, clusterName string, service *v1.Service, nodes []*v1.Node) error {
	loadBalancerName := qc.GetLoadBalancerName(ctx, clusterName, service)
	klog.V(3).Infof("UpdateLoadBalancer(%v, %v, %v)", clusterName, loadBalancerName, nodes)

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
		instanceID := qc.NodeNameToInstanceID(types.NodeName(node.Name))
		expected.Insert(instanceID)
	}

	listenerBackendPorts := map[string]int32{}
	instanceBackendIDs := map[string][]string{}
	loadBalancerListeners, err := qc.getLoadBalancerListeners(*loadBalancer.LoadBalancerID)
	if err != nil {
		klog.Errorf("Couldn't get loadBalancer '%v' err: %v", loadBalancerName, err)
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

	klog.V(3).Infof("For the loadBalancer, expected instances: %v, actual instances: %v, need to remove instances: %v, need to add instances: %v", expected, actual, removeInstances, addInstances)

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
					Port:                    qcservice.Int(int(port)),
				}
			}
			err := qc.addLoadBalancerBackends(listenerID, backends)
			if err != nil {
				klog.Errorf("Couldn't add backend servers '%v' to loadBalancer '%s': %v, err: %s", instances, loadBalancerName, loadBalancer, err.Error())
				return err
			}
			needUpdate = true
		}

		klog.V(1).Infof("Instances '%v' added to loadBalancer %s", instances, loadBalancerName)
	}

	if len(removeInstances) > 0 {
		instances := removeInstances.List()

		backendIDs := make([]string, 0, len(instances))
		for _, instance := range instances {
			backendIDs = append(backendIDs, instanceBackendIDs[instance]...)
		}
		err := qc.deleteLoadBalancerBackends(backendIDs)
		if err != nil {
			klog.Errorf("Couldn't remove backend servers '%v' from loadBalancer '%s': %v, err: %s", instances, loadBalancerName, loadBalancer, err.Error())
			return err
		}
		needUpdate = true
		klog.V(1).Infof("Instances '%v' removed from loadBalancer %s", instances, loadBalancerName)
	}

	if needUpdate {
		klog.V(1).Info("Enforce the loadBalancer update backends config")
		_, err = qc.updateLoadBalancer(loadBalancer)
		if err != nil {
			klog.Errorf("Couldn't update loadBalancer '%s': err: %s", loadBalancerName, err.Error())
			return err
		}
	}

	klog.V(3).Info("Skip update loadBalancer backends")
	return nil
}


