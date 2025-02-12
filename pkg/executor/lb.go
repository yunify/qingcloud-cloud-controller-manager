package executor

import (
	"fmt"
	"strings"

	"github.com/davecgh/go-spew/spew"
	qcclient "github.com/yunify/qingcloud-sdk-go/client"
	qcservice "github.com/yunify/qingcloud-sdk-go/service"
	"github.com/yunify/qingcloud-sdk-go/utils"
	"k8s.io/klog/v2"

	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/apis"
	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/errors"
)

const (
	QingCloudLBIPPrefix = "198.19"
	DefaultNodeNum      = 2
	DefaultMode         = 1
	DefaultLBType       = 0
)

func convertLoadBalancerStatus(lb *qcservice.LoadBalancer) apis.LoadBalancerStatus {
	var (
		resultVIP      []string
		resultListener []apis.LoadBalancerListener
		createdEIP     []*string
	)

	for _, privateIP := range lb.PrivateIPs {
		//The IP of this segment is load balancing specific.
		if !strings.HasPrefix(*privateIP, QingCloudLBIPPrefix) {
			resultVIP = append(resultVIP, *privateIP)
		} else {
			//VIPs tend to be placed first, so if the first one doesn't match, we're out.
			//The vip is actually in the vxnet field here, but the go sdk doesn't implement this field, so I've tricked it here.
			break
		}
	}

	for _, eip := range lb.Cluster {
		//iaas bug?  slice contain nil
		if eip.EIPID == nil || *eip.EIPID == "" {
			klog.V(4).Infof("invalid lb eip %s", spew.Sdump(lb))
			continue
		}
		resultVIP = append(resultVIP, *eip.EIPAddr)
		if strings.Compare(*eip.EIPName, AllocateEIPName) == 0 {
			createdEIP = append(createdEIP, eip.EIPID)
		}
	}

	for _, listener := range lb.Listeners {
		tmp := convertLoadBalancerListener([]*qcservice.LoadBalancerListener{listener})
		resultListener = append(resultListener, *tmp[0])
	}

	return apis.LoadBalancerStatus{
		LoadBalancerID:        lb.LoadBalancerID,
		VIP:                   resultVIP,
		LoadBalancerListeners: resultListener,
		CreatedEIPs:           createdEIP,
	}
}

func convertLoadBalancer(lb *qcservice.LoadBalancer) *apis.LoadBalancer {
	var eipsID []*string
	for _, eip := range lb.Cluster {
		if *eip.EIPID != "" {
			eipsID = append(eipsID, eip.EIPID)
		}
	}
	return &apis.LoadBalancer{
		Spec: apis.LoadBalancerSpec{
			LoadBalancerName: lb.LoadBalancerName,
			LoadBalancerType: lb.LoadBalancerType,
			NodeCount:        lb.NodeCount,
			VxNetID:          lb.VxNetID,
			PrivateIPs:       lb.PrivateIPs,
			SecurityGroups:   lb.SecurityGroupID,
			EIPs:             eipsID,
		},
		Status: convertLoadBalancerStatus(lb),
	}
}

func (q *QingCloudClient) GetLoadBalancerByName(name string) (*apis.LoadBalancer, error) {
	status := []*string{
		qcservice.String("pending"),
		qcservice.String("active"),
		qcservice.String("stopped"),
	}
	input := &qcservice.DescribeLoadBalancersInput{
		Status:     status,
		SearchWord: &name,
		Owner:      &q.Config.UserID,
		//> 0 时，会额外返回监听器的信息
		//    >= 2 时，会返回集群健康检查信息
		Verbose: qcservice.Int(2),
	}
	output, err := q.LBService.DescribeLoadBalancers(input)
	if err != nil && strings.Contains(err.Error(), "QingCloud Error: Code (1300)") {
		klog.V(4).Infof("cannot found lb by name, err=%s, input=%s, output=%s", spew.Sdump(err), spew.Sdump(input), spew.Sdump(output))
		return nil, errors.NewResourceNotFoundError(ResourceNameLoadBalancer, name)
	} else if err != nil {
		klog.V(4).Infof("get lb by name error, err=%s, input=%s, output=%s", spew.Sdump(err), spew.Sdump(input), spew.Sdump(output))
		return nil, fmt.Errorf("get lb by name error, err=%s", err)
	}
	for _, lb := range output.LoadBalancerSet {
		if lb.LoadBalancerName != nil && *lb.LoadBalancerName == name {
			return convertLoadBalancer(lb), nil
		}
	}
	return nil, errors.NewResourceNotFoundError(ResourceNameLoadBalancer, name)
}

func (q *QingCloudClient) GetLoadBalancerByID(id string) (*apis.LoadBalancer, error) {
	input := &qcservice.DescribeLoadBalancersInput{
		LoadBalancers: []*string{&id},
		Verbose:       qcservice.Int(2),
	}
	output, err := q.LBService.DescribeLoadBalancers(input)
	if err != nil && strings.Contains(err.Error(), "QingCloud Error: Code (1300)") {
		klog.V(4).Infof("cannot found lb by id, err=%s, input=%s, output=%s", spew.Sdump(err), spew.Sdump(input), spew.Sdump(output))
		return nil, errors.NewResourceNotFoundError(ResourceNameLoadBalancer, id)
	} else if err != nil {
		klog.V(4).Infof("get lb by id error, err=%s, input=%s, output=%s", spew.Sdump(err), spew.Sdump(input), spew.Sdump(output))
		return nil, fmt.Errorf("get lb by id error, err=%s", err)
	}

	if len(output.LoadBalancerSet) > 0 {
		lb := output.LoadBalancerSet[0]
		return convertLoadBalancer(lb), nil
	} else {
		return nil, errors.NewResourceNotFoundError(ResourceNameLoadBalancer, id)
	}
}

func (q *QingCloudClient) fillLBDefaultFileds(input *qcservice.CreateLoadBalancerInput) {
	if input.SecurityGroup == nil {
		input.SecurityGroup = q.sg.Status.SecurityGroupID
	}

	if input.NodeCount == nil {
		input.NodeCount = qcservice.Int(DefaultNodeNum)
	}

	if input.Mode == nil {
		input.Mode = qcservice.Int(DefaultMode)
	}

	if input.LoadBalancerType == nil {
		input.LoadBalancerType = qcservice.Int(DefaultLBType)
	}
}

func (q *QingCloudClient) CreateLB(input *apis.LoadBalancer) (*apis.LoadBalancer, error) {
	if input.Spec.VxNetID == nil && len(input.Spec.EIPs) <= 0 {
		klog.V(4).Infof("need vxnet or eip, input=%s", spew.Sdump(input))
		return nil, fmt.Errorf("create lb input invalid, need vxnet or eip")
	}

	inputLB := &qcservice.CreateLoadBalancerInput{
		EIPs:             input.Spec.EIPs,
		LoadBalancerName: input.Spec.LoadBalancerName,
		LoadBalancerType: input.Spec.LoadBalancerType,
		NodeCount:        input.Spec.NodeCount,
		VxNet:            input.Spec.VxNetID,
		SecurityGroup:    input.Spec.SecurityGroups,
	}
	if len(input.Spec.PrivateIPs) > 0 {
		inputLB.PrivateIP = input.Spec.PrivateIPs[0]
	}
	q.fillLBDefaultFileds(inputLB)
	output, err := q.LBService.CreateLoadBalancer(inputLB)
	if err != nil || *output.RetCode != 0 {
		klog.V(4).Infof("failed to create lb, err=%s, input=%s, output=%s", spew.Sdump(err), spew.Sdump(inputLB), spew.Sdump(output))
		if err != nil {
			return nil, fmt.Errorf("failed to create lb, err=%v", err)
		}
		return nil, fmt.Errorf("failed to create lb, code=%d, msg=%s", *output.RetCode, *output.Message)
	}

	var lbID = *output.LoadBalancerID
	lb, err := qcclient.WaitLoadBalancerStatus(q.LBService, lbID, qcclient.LoadBalancerStatusActive, operationWaitTimeout, waitInterval)
	if err != nil {
		return nil, fmt.Errorf("failed to wait lb %s to active, err=%v", lbID, err)
	}

	err = q.attachTagsToResources([]*string{&lbID}, "loadbalancer")
	if err != nil {
		klog.Errorf("Failed to attach tag to loadBalancer %s, err: %v", lbID, err)
	}

	return convertLoadBalancer(lb), nil
}

// need update lb
func (q *QingCloudClient) ModifyLB(conf *apis.LoadBalancer) error {
	input := &qcservice.ModifyLoadBalancerAttributesInput{
		LoadBalancer: conf.Status.LoadBalancerID,
		NodeCount:    conf.Spec.NodeCount,
	}
	if len(conf.Spec.PrivateIPs) > 0 {
		input.PrivateIP = conf.Spec.PrivateIPs[0]
	}
	output, err := q.LBService.ModifyLoadBalancerAttributes(input)
	if err != nil || *output.RetCode != 0 {
		klog.V(4).Infof("failed to modify lb attr, err=%s, input=%s, output=%s", spew.Sdump(err), spew.Sdump(input), spew.Sdump(output))
		if err != nil {
			return fmt.Errorf("failed to modify lb attr, err=%v", err)
		}
		return fmt.Errorf("failed to modify lb attr, code=%d, msg=%s", *output.RetCode, *output.Message)
	}

	//need apply modify
	return nil
}

func (q *QingCloudClient) UpdateLB(id *string) error {
	lb, err := q.GetLoadBalancerByID(*id)
	if err != nil {
		return fmt.Errorf("failed get lb %s when update lb, err=%s", *id, err)
	}
	if lb.Status.IsApplied != nil && *lb.Status.IsApplied != 0 {
		return nil
	}

	updateInput := &qcservice.UpdateLoadBalancersInput{
		LoadBalancers: []*string{id},
	}
	output, err := q.LBService.UpdateLoadBalancers(updateInput)
	if err != nil || *output.RetCode != 0 {
		klog.V(4).Infof("failed to update lb attr, err=%s, input=%s, output=%s", spew.Sdump(err), spew.Sdump(updateInput), spew.Sdump(output))
		if err != nil {
			return fmt.Errorf("failed to update lb attr, err=%v", err)
		}
		return fmt.Errorf("failed to update lb attr, code=%d, msg=%s", *output.RetCode, *output.Message)
	}
	err = qcclient.WaitJob(q.jobService, *output.JobID, operationWaitTimeout, waitInterval)
	if err != nil {
		return fmt.Errorf("lb %s delete job not completed", *id)
	}

	return nil
}

// need update before delete
func (q *QingCloudClient) DeleteLB(id *string) error {
	var (
		err    error
		output *qcservice.DeleteLoadBalancersOutput
		quit   bool
	)

	err = utils.WaitForSpecificOrError(func() (bool, error) {
		output, err = q.LBService.DeleteLoadBalancers(&qcservice.DeleteLoadBalancersInput{
			LoadBalancers: []*string{id},
		})

		if err != nil {
			if strings.Contains(err.Error(), "QingCloud Error: Code (2100)") {
				quit = true
				return true, nil
			}

			if !strings.Contains(err.Error(), "QingCloud Error: Code (1400)") {
				klog.V(4).Infof("failed to delete lb %s, err=%s, output=%s", *id, spew.Sdump(err), spew.Sdump(output))
				return false, fmt.Errorf("failed to delete lb %s, err=%v", *id, err)
			}

			return false, nil
		} else {
			return true, nil
		}
	}, operationWaitTimeout, waitInterval)

	if quit {
		return nil
	}

	if output != nil && err == nil {
		if *output.RetCode != 0 {
			return fmt.Errorf("failed to delete lb %s, code=%d, message=%s", *id, *output.RetCode, *output.Message)
		}

		err = qcclient.WaitJob(q.jobService, *output.JobID, operationWaitTimeout, waitInterval)
		if err != nil {
			return fmt.Errorf("lb %s delete job not completed, err: %v", *id, err)
		}
	}

	return nil
}

func (q *QingCloudClient) AssociateEIPsToLB(id *string, eips []*string) error {
	var err error
	var output *qcservice.AssociateEIPsToLoadBalancerOutput

	if len(eips) == 0 {
		return nil
	}
	eipValueSlice := qcservice.StringValueSlice(eips)
	output, err = q.LBService.AssociateEIPsToLoadBalancer(&qcservice.AssociateEIPsToLoadBalancerInput{
		EIPs:         eips,
		LoadBalancer: id,
	})
	if err != nil {
		return fmt.Errorf("associate eip %v to lb %s error: %v", eipValueSlice, *id, err)
	}

	if output != nil {
		if *output.RetCode != 0 {
			return fmt.Errorf("associate eip %v to lb %s failed, code=%d, message=%s", eipValueSlice, *id, *output.RetCode, *output.Message)
		}

		err = qcclient.WaitJob(q.jobService, *output.JobID, operationWaitTimeout, waitInterval)
		if err != nil {
			return fmt.Errorf("associate eip %v to lb %s job not completed, err %v", eipValueSlice, *id, err)
		}
	}

	return nil
}
func (q *QingCloudClient) DissociateEIPsFromLB(id *string, eips []*string) error {
	var err error
	var output *qcservice.DissociateEIPsFromLoadBalancerOutput

	if len(eips) == 0 {
		return nil
	}
	eipValueSlice := qcservice.StringValueSlice(eips)
	output, err = q.LBService.DissociateEIPsFromLoadBalancer(&qcservice.DissociateEIPsFromLoadBalancerInput{
		EIPs:         eips,
		LoadBalancer: id,
	})
	if err != nil {
		return fmt.Errorf("dissociate eips %v from lb %s error: %v", eipValueSlice, *id, err)
	}

	if output != nil {
		if *output.RetCode != 0 {
			return fmt.Errorf("dissociate eip %v from lb %s failed, code=%d, message=%s", eipValueSlice, *id, *output.RetCode, *output.Message)
		}

		err = qcclient.WaitJob(q.jobService, *output.JobID, operationWaitTimeout, waitInterval)
		if err != nil {
			return fmt.Errorf("dissociate eip %v from lb %s job not completed, err %v", eipValueSlice, *id, err)
		}
	}

	return nil
}
