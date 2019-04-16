package loadbalance

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	qcclient "github.com/yunify/qingcloud-sdk-go/client"
	qcservice "github.com/yunify/qingcloud-sdk-go/service"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/klog"
)

func (qc *QingCloud) getRealLoadBalancer(clusterName string, service *corev1.Service) (*qcservice.LoadBalancer, error) {
	loadBalancerName := GetLoadBalancerName(clusterName, service)
	return qc.getLoadBalancerByName(loadBalancerName)
}

func (qc *QingCloud) getLoadBalancerByName(name string) (*qcservice.LoadBalancer, error) {
	status := []*string{qcservice.String("pending"), qcservice.String("active"), qcservice.String("stopped")}
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

func (qc *QingCloud) getLoadBalancerByID(id string) (*qcservice.LoadBalancer, error) {
	output, err := qc.lbService.DescribeLoadBalancers(&qcservice.DescribeLoadBalancersInput{
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
	return qcclient.WaitJob(qc.jobService, *output.JobID, operationWaitTimeout, waitInterval)
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

func (qc *QingCloud) getLoadBalancerListeners(loadBalancerID, prefix string) ([]*qcservice.LoadBalancerListener, error) {
	loadBalancerListeners := []*qcservice.LoadBalancerListener{}
	resp, err := qc.lbService.DescribeLoadBalancerListeners(&qcservice.DescribeLoadBalancerListenersInput{
		LoadBalancer: &loadBalancerID,
		Verbose:      qcservice.Int(1),
		Limit:        qcservice.Int(pageLimt),
	})
	if err != nil {
		return nil, err
	}
	for _, l := range resp.LoadBalancerListenerSet {
		if prefix != "" {
			if strings.Contains(*l.LoadBalancerListenerName, prefix) {
				loadBalancerListeners = append(loadBalancerListeners, l)
			}
		} else {
			loadBalancerListeners = append(loadBalancerListeners, l)
		}
	}
	return loadBalancerListeners, nil
}

// enforce the loadBalancer config
func (qc *QingCloud) updateLoadBalancer(loadBalancer *qcservice.LoadBalancer) (*v1.LoadBalancerStatus, error) {
	output, err := qc.lbService.UpdateLoadBalancers(&qcservice.UpdateLoadBalancersInput{
		LoadBalancers: []*string{loadBalancer.LoadBalancerID},
	})
	if err != nil {
		klog.Errorf("Couldn't update loadBalancer '%s'", *loadBalancer.LoadBalancerID)
		return nil, err
	}
	qcclient.WaitJob(qc.jobService, *output.JobID, operationWaitTimeout, waitInterval)
	qyEips, qyPrivateIps, _, err := qc.waitLoadBalancerActive(*loadBalancer.LoadBalancerID, operationWaitTimeout)

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
	klog.Infof("Start loadBalancer '%v', ingress ip '%v'", *loadBalancer.LoadBalancerName, status.Ingress)

	return status, nil
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
	klog.V(4).Infof("AddSecurityGroupRules SecurityGroup: [%s], output: [%+v] ", *sgID, addRuleOutput)
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
		klog.Errorf("Error create loadBalancer TCP listener (LoadBalancerId:'%s', Port: '%v'): %v", loadBalancerID, port, err)
		return "", err
	}
	klog.Infof("Created LoadBalancerTCPListener (LoadBalancerId:'%s', Port: '%v', listenerID: '%s')", loadBalancerID, port, listenerID)

	backends := make([]*qcservice.LoadBalancerBackend, len(instances))
	for j, instance := range instances {
		//copy for get address.
		instanceID := instance
		backends[j] = &qcservice.LoadBalancerBackend{
			ResourceID:              &instanceID,
			LoadBalancerBackendName: &instanceID,
			Port:                    qcservice.Int(int(nodePort)),
		}
	}
	if len(backends) > 0 {
		err = qc.addLoadBalancerBackends(listenerID, backends)
		if err != nil {
			klog.Errorf("Couldn't add backend servers '%v' to loadBalancer with id '%v': %v", instances, loadBalancerID, err)
			return "", err
		}
		klog.V(3).Infof("Added backend servers '%v' to loadBalancer with id '%s'", instances, loadBalancerID)
	}

	return listenerID, nil
}

func (qc *QingCloud) resizeLoadBalancer(loadBalancerID string, newLoadBalancerType int, oldLoadBalancerType int) error {
	if newLoadBalancerType < oldLoadBalancerType {
		klog.V(1).Infof("Stop lb at first before resizing it because current lb type '%d' is bigger with the one in k8s servie spec '%d', ", oldLoadBalancerType, newLoadBalancerType)
		err := qc.stopLoadBalancer(loadBalancerID)
		if err != nil {
			klog.Error(err)
			return err
		}
	}
	klog.V(2).Infof("Starting to resize loadBalancer '%s'", loadBalancerID)
	output, err := qc.lbService.ResizeLoadBalancers(&qcservice.ResizeLoadBalancersInput{
		LoadBalancerType: &newLoadBalancerType,
		LoadBalancers:    []*string{qcservice.String(loadBalancerID)},
	})
	if err != nil {
		return err
	}
	qcclient.WaitJob(qc.jobService, *output.JobID, operationWaitTimeout, waitInterval)

	if newLoadBalancerType < oldLoadBalancerType {
		klog.V(1).Infof("Start lb now after resizing it to take effect because previous lb type '%d' is bigger with the one in k8s servie spec '%d', ", oldLoadBalancerType, newLoadBalancerType)
		err := qc.startLoadBalancer(loadBalancerID)
		if err != nil {
			klog.Error(err)
			return err
		}
	}
	return err
}

func (qc *QingCloud) associateEipToLoadBalancer(loadBalancerID string, eip string) error {
	klog.V(2).Infof("Starting to associate Eip %s to loadBalancer '%s'", eip, loadBalancerID)
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
	klog.V(2).Infof("Starting to dissociate Eip %s from loadBalancer '%s'", eip, loadBalancerID)
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
	klog.Infof("Starting to delete loadBalancer '%s' before creating", loadBalancerID)
	err := qc.deleteLoadBalancer(loadBalancerID)
	if err != nil {
		klog.V(1).Infof("Deleted loadBalancer '%s' error before creating: %v", loadBalancerID, err)
		return err
	}
	err = qc.waitLoadBalancerDelete(loadBalancerID, operationWaitTimeout)
	if err != nil {
		klog.Error(err)
		return err
	}
	err = qc.DeleteSecurityGroup(securityGroupID)
	if err != nil {
		klog.Errorf("Delete SecurityGroup '%v' err '%s' ", &securityGroupID, err)
	}
	klog.Infof("Done, deleted loadBalancer '%s'", loadBalancerID)
	return nil
}

func (qc *QingCloud) deleteLoadBalancerListener(loadBalancerListenerID string) error {
	klog.Infof("Deleting LoadBalancerTCPListener :'%s'", loadBalancerListenerID)
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

func (qc *QingCloud) modifyLoadBalancerListenerAttributes(loadBalancerListenerID string, balanceMode string) error {
	klog.Infof("Modifying balanceMode of LoadBalancerTCPListener :'%s'", loadBalancerListenerID)
	output, err := qc.lbService.ModifyLoadBalancerListenerAttributes(&qcservice.ModifyLoadBalancerListenerAttributesInput{
		LoadBalancerListener: &loadBalancerListenerID,
		BalanceMode:          &balanceMode,
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
	klog.V(2).Infof("Stopping loadBalancer '%s'", loadBalancerID)
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
	klog.V(2).Infof("Starting loadBalancer '%s'", loadBalancerID)
	output, err := qc.lbService.StartLoadBalancers(&qcservice.StartLoadBalancersInput{
		LoadBalancers: []*string{qcservice.String(loadBalancerID)},
	})
	if err != nil {
		return err
	}
	return qcclient.WaitJob(qc.jobService, *output.JobID, operationWaitTimeout, waitInterval)
}

// Check the attributes in service spec, compared with current LB setting
// return 'skip' means doing nothing
// return 'update' means update current LB
// return 'delete' means delete current LB
// return "add" means add a new listener
func (qc *QingCloud) compareSpecAndLoadBalancer(service *v1.Service, loadBalancer *qcservice.LoadBalancer) (string, error) {
	// get lb properties from k8s service spec
	lbType, hasType := service.Annotations[ServiceAnnotationLoadBalancerType]
	if lbType != "0" && lbType != "1" && lbType != "2" && lbType != "3" && lbType != "4" && lbType != "5" {
		lbType = "0"
	}
	loadBalancerType, _ := strconv.Atoi(lbType)

	lbEipIds, hasEip := service.Annotations[ServiceAnnotationLoadBalancerEipIds]
	vxnetId, hasVxnet := service.Annotations[ServiceAnnotationLoadBalancerVxnetId]

	_, _, qyEipIDs := qc.getLoadBalancerNetConfig(loadBalancer)

	result := "skip"
	if !hasEip && !hasVxnet {
		klog.V(1).Infof("No eip or vxnet is specified for this LB '%s', will keep this LB as it is", *loadBalancer.LoadBalancerID)
		//return "delete", nil
	}
	if hasType && loadBalancerType != *loadBalancer.LoadBalancerType {
		klog.V(1).Infof("LB type is changed")
		result = "update"
	}
	if hasEip {
		if *loadBalancer.VxNetID != "" && *loadBalancer.VxNetID != "vxnet-0" {
			klog.V(1).Infof("EIP is assigned to this LB '%s', which used to be assigned with vxnet '%s', need to delete this lb and recreate it", *loadBalancer.LoadBalancerID, *loadBalancer.VxNetID)
			return "delete", nil
		}
		k8sLoadBalancerEipIds := strings.Split(lbEipIds, ",")
		for _, k8sEipID := range k8sLoadBalancerEipIds {
			if stringIndex(qyEipIDs, k8sEipID) < 0 {
				klog.V(1).Infof("New EIP '%s' is assigned to LB '%s'", k8sEipID, *loadBalancer.LoadBalancerID)
				result = "update"
			}
		}
		for _, qyEipID := range qyEipIDs {
			if stringIndex(k8sLoadBalancerEipIds, qyEipID) < 0 {
				klog.V(1).Infof("EIP '%s' is dissociated from LB '%s'", qyEipID, *loadBalancer.LoadBalancerID)
				result = "update"
			}
		}
	}
	if hasVxnet {
		if vxnetId != *loadBalancer.VxNetID || len(qyEipIDs) > 0 {
			klog.V(1).Infof("Delete this LB '%s' because vxnet in service spec is different with previous setting, will recreate this LB later", *loadBalancer.LoadBalancerID)
			return "delete", nil
		}
	}
	return result, nil
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

func GetListenerFromArray(service *v1.Service, listeners []*qcservice.LoadBalancerListener) *qcservice.LoadBalancerListener {
	name := GetListenerPrefixName(service)
	for index, v := range listeners {
		if strings.Contains(*v.LoadBalancerListenerName, name) {
			return listeners[index]
		}
	}
	return nil
}
func (qc *QingCloud) compareSpecAndLoadBalancerListeners(service *v1.Service, listeners []*qcservice.LoadBalancerListener, k8sTCPPorts []int, balanceMode string) string {
	result := "skip"
	if len(listeners) <= 0 {
		return result
	}
	qyLbListenerPorts := []int{}
	qyListener := GetListenerFromArray(service, listeners)
	if qyListener == nil {
		return "add"
	}
	// sum all existing listeners' port on qingcloud
	qyLbListenerPorts = append(qyLbListenerPorts, *qyListener.ListenerPort)
	if intIndex(k8sTCPPorts, *qyListener.ListenerPort) < 0 {
		// this listener/port is already remove from spec, so just delete it
		klog.V(1).Infof("This LB listener '%s' is not available any more for this LB, so need to delete it", *qyListener.LoadBalancerListenerID)
		result = "update"
	}
	for _, k8sPort := range k8sTCPPorts {
		// this is the index to locate the listener on qingcloud
		qyLbListenerPos := intIndex(qyLbListenerPorts, k8sPort)
		if qyLbListenerPos >= 0 {
			// so port in spec matches existing listener's port, then check if balance mode is modified in spec, if yes, modify listener's attr
			if balanceMode != *listeners[qyLbListenerPos].BalanceMode {
				klog.V(1).Infof("balancemode is changed in service spec")
				result = "update"
			}
		} else {
			// so this is new port from spec, will create a new listener for it with backend nodes
			klog.V(1).Infof("New LB listener need to created because this port '%d' is newly added in service spec", k8sPort)
			result = "update"
		}

	}
	return result
}

func (qc *QingCloud) createLoadBalancerFromServiceSpec(ctx context.Context, clusterName string, service *v1.Service) (string, error) {
	// get lb properties from k8s service spec
	lbType := service.Annotations[ServiceAnnotationLoadBalancerType]
	if lbType != "0" && lbType != "1" && lbType != "2" && lbType != "3" && lbType != "4" && lbType != "5" {
		lbType = "0"
	}
	loadBalancerType, _ := strconv.Atoi(lbType)

	lbEipIds, hasEip := service.Annotations[ServiceAnnotationLoadBalancerEipIds]
	vxnetId, hasVxnet := service.Annotations[ServiceAnnotationLoadBalancerVxnetId]

	loadBalancerName := qc.GetLoadBalancerName(ctx, clusterName, service)
	var loadBalancerID string
	var err error
	if hasEip {
		if hasVxnet {
			err := fmt.Errorf("Both ServiceAnnotationLoadBalancerVxnetId and ServiceAnnotationLoadBalancerEipIds are set. Please set only one of them")
			klog.Error(err)
			return "", err
		}
		loadBalancerEipIds := strings.Split(lbEipIds, ",")
		loadBalancerID, err = qc.createLoadBalancerWithEips(loadBalancerName, loadBalancerType, loadBalancerEipIds)
		if err != nil {
			klog.Errorf("Error creating loadBalancer '%s': %v", loadBalancerName, err)
			return "", err
		}
	} else if hasVxnet {
		loadBalancerID, err = qc.createLoadBalancerWithVxnet(loadBalancerName, loadBalancerType, vxnetId)
		if err != nil {
			klog.Errorf("Error creating loadBalancer '%s': %v", loadBalancerName, err)
			return "", err
		}
	} else {
		// use default vxnet id in qingcloud.conf to create new load balancer because no eip or vxnet is specified in service spec
		if qc.defaultVxNetForLB != "" {
			klog.Infof("As no eip or vxnet is specified in service spec but set defaultVxNetForLB in qingcloud.config, so just create loadBalancer '%s' in zone '%s' with this default vxnet '%s", loadBalancerName, qc.zone, qc.defaultVxNetForLB)
			loadBalancerID, err = qc.createLoadBalancerWithVxnet(loadBalancerName, loadBalancerType, qc.defaultVxNetForLB)
			if err != nil {
				klog.Errorf("Error creating loadBalancer '%s': %v", loadBalancerName, err)
				return "", err
			}
		} else {
			err := fmt.Errorf("Both ServiceAnnotationLoadBalancerVxnetId and ServiceAnnotationLoadBalancerEipIds are not set. Please set only one of them")
			klog.Error(err)
			return "", err
		}
	}
	_, _, _, err = qc.waitLoadBalancerActive(loadBalancerID, operationWaitTimeout)
	if err != nil {
		klog.Error(err)
		return "", err
	}
	return loadBalancerID, nil
}
func (qc *QingCloud) updateLoadBalancerListenersFromServiceSpec(loadBalancerID string, listeners []*qcservice.LoadBalancerListener, k8sTCPPorts []int, k8sNodePorts []int, balanceMode string, instances []string) error {
	qyLbListenerPorts := []int{}
	for _, qyLbListerner := range listeners {
		// sum all existing listeners' port on qingcloud
		qyLbListenerPorts = append(qyLbListenerPorts, *qyLbListerner.ListenerPort)
		if intIndex(k8sTCPPorts, *qyLbListerner.ListenerPort) < 0 {
			// this listener/port is already remove from spec, so just delete it
			klog.V(1).Infof("This LB listener '%s' is not available any more for this LB, so just delete it", *qyLbListerner.LoadBalancerListenerID)
			err := qc.deleteLoadBalancerListener(*qyLbListerner.LoadBalancerListenerID)
			if err != nil {
				klog.Error(err)
				return err
			}

		}
	}
	for i, k8sPort := range k8sTCPPorts {
		// this is the index to locate the listener on qingcloud
		qyLbListenerPos := intIndex(qyLbListenerPorts, k8sPort)
		if qyLbListenerPos >= 0 {
			// so port in spec matches existing listener's port, then check if balance mode is modified in spec, if yes, modify listener's attr
			if balanceMode != *listeners[qyLbListenerPos].BalanceMode {
				klog.V(1).Infof("Update this LB listener because balanceMode is changed")
				err := qc.modifyLoadBalancerListenerAttributes(*listeners[qyLbListenerPos].LoadBalancerListenerID, balanceMode)
				if err != nil {
					klog.Error(err)
					return err
				}

			}
		} else {
			// so this is new port from spec, will create a new listener for it with backend nodes
			klog.V(1).Infof("Create new LB listener because this port '%d' is newly added in service spec", k8sPort)
			_, err := qc.createLoadBalancerListenerWithBackends(loadBalancerID, k8sPort, k8sNodePorts[i], balanceMode, instances)
			if err != nil {
				klog.Errorf("Couldn't create loadBalancerListener with backends")
				klog.Error(err)
				return err
			}

		}
	}
	return nil
}
func (qc *QingCloud) getLoadBalancerNetConfig(loadBalancer *qcservice.LoadBalancer) ([]string, []string, []string) {
	eips := []string{}
	eipIDs := []string{}
	privateIps := []string{}
	for _, eip := range loadBalancer.Cluster {
		if *eip.EIPID != "" {
			eipIDs = append(eipIDs, *eip.EIPID)
			eips = append(eips, *eip.EIPAddr)
		}
	}
	for _, pip := range loadBalancer.PrivateIPs {
		privateIps = append(privateIps, *pip)
	}
	return eips, privateIps, eipIDs
}
