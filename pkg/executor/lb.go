package executor

import (
	"time"

	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/errors"
	qcclient "github.com/yunify/qingcloud-sdk-go/client"
	qcservice "github.com/yunify/qingcloud-sdk-go/service"
	"k8s.io/klog"
)

var _ QingCloudLoadBalancerExecutor = &qingCloudLoadBalanceExecutor{}

const (
	waitInterval         = 10 * time.Second
	operationWaitTimeout = 180 * time.Second
	pageLimt             = 100
)

type qingCloudLoadBalanceExecutor struct {
	lbapi  *qcservice.LoadBalancerService
	jobapi *qcservice.JobService
	tagapi *qcservice.TagService
	tagIDs []string
	addTag bool
	owner  string
}

func newServerErrorOfLoadBalancer(name, method string, e error) error {
	return errors.NewCommonServerError(ResourceNameLoadBalancer, name, method, e.Error())
}

func NewQingCloudLoadBalanceExecutor(owner string, lbapi *qcservice.LoadBalancerService, jobapi *qcservice.JobService, tagapi *qcservice.TagService) QingCloudLoadBalancerExecutor {
	return &qingCloudLoadBalanceExecutor{
		lbapi:  lbapi,
		jobapi: jobapi,
		tagapi: tagapi,
		owner:  owner,
	}
}

func (q *qingCloudLoadBalanceExecutor) EnableTagService(tagIds []string) {
	if len(tagIds) > 0 {
		q.addTag = true
		q.tagIDs = tagIds
	}
}

func (q *qingCloudLoadBalanceExecutor) GetLoadBalancerByName(name string) (*qcservice.LoadBalancer, error) {
	status := []*string{qcservice.String("pending"), qcservice.String("active"), qcservice.String("stopped")}
	output, err := q.lbapi.DescribeLoadBalancers(&qcservice.DescribeLoadBalancersInput{
		Status:     status,
		SearchWord: &name,
		Owner:      &q.owner,
	})
	if err != nil {
		return nil, newServerErrorOfLoadBalancer(name, "GetLoadBalancerByName", err)
	}
	for _, lb := range output.LoadBalancerSet {
		if lb.LoadBalancerName != nil && *lb.LoadBalancerName == name {
			return lb, nil
		}
	}
	return nil, errors.NewResourceNotFoundError(ResourceNameLoadBalancer, name)
}

func (q *qingCloudLoadBalanceExecutor) GetLoadBalancerByID(id string) (*qcservice.LoadBalancer, error) {
	output, err := q.lbapi.DescribeLoadBalancers(&qcservice.DescribeLoadBalancersInput{
		LoadBalancers: []*string{&id},
	})
	if err != nil {
		return nil, newServerErrorOfLoadBalancer(id, "GetLoadBalancerByID", err)
	}

	if len(output.LoadBalancerSet) == 0 {
		return nil, errors.NewResourceNotFoundError(ResourceNameLoadBalancer, id)
	}
	lb := output.LoadBalancerSet[0]
	if *lb.Status == qcclient.LoadBalancerStatusCeased || *lb.Status == qcclient.LoadBalancerStatusDeleted {
		return nil, errors.NewResourceNotFoundError(ResourceNameLoadBalancer, id, " is deleting")
	}
	return lb, nil
}

func (q *qingCloudLoadBalanceExecutor) Start(id string) error {
	klog.V(2).Infof("Starting loadBalancer '%s'", id)
	output, err := q.lbapi.StartLoadBalancers(&qcservice.StartLoadBalancersInput{
		LoadBalancers: []*string{&id},
	})
	if err != nil {
		klog.Errorln("Failed to start loadbalancer")
		return newServerErrorOfLoadBalancer(id, "Start", err)
	}
	return qcclient.WaitJob(q.jobapi, *output.JobID, operationWaitTimeout, waitInterval)
}

func (q *qingCloudLoadBalanceExecutor) Stop(id string) error {
	klog.V(2).Infof("Stopping loadBalancer '%s'", id)
	output, err := q.lbapi.StopLoadBalancers(&qcservice.StopLoadBalancersInput{
		LoadBalancers: []*string{&id},
	})
	if err != nil {
		klog.Errorln("Failed to stop loadbalancer")
		return newServerErrorOfLoadBalancer(id, "Stop", err)
	}
	return qcclient.WaitJob(q.jobapi, *output.JobID, operationWaitTimeout, waitInterval)
}

func (q *qingCloudLoadBalanceExecutor) Create(input *qcservice.CreateLoadBalancerInput) (*qcservice.LoadBalancer, error) {
	klog.V(2).Infof("Creating LB: %+v", *input)
	name := *input.LoadBalancerName
	output, err := q.lbapi.CreateLoadBalancer(input)
	if err != nil {
		return nil, newServerErrorOfLoadBalancer(name, "Create", err)
	}
	klog.V(2).Infof("Waiting for Lb %s starting", name)
	err = q.waitLoadBalancerActive(*output.LoadBalancerID)
	if err != nil {
		klog.Errorf("LoadBalancer %s start failed", *output.LoadBalancerID)
		return nil, newServerErrorOfLoadBalancer(name, "waitLoadBalancerActive", err)
	}
	klog.V(2).Infof("Lb %s is successfully started", name)
	lb, err := q.GetLoadBalancerByID(*output.LoadBalancerID)
	if err != nil {
		return nil, newServerErrorOfLoadBalancer(name, "GetLoadBalancerByID", err)
	}
	if q.addTag {
		err = AddTagsToResource(q.tagapi, q.tagIDs, *output.LoadBalancerID, "loadbalancer")
		if err != nil {
			klog.Errorf("Failed to add tag to loadBalancer %s, err: %s", *output.LoadBalancerID, err.Error())
		}
	}
	return lb, nil
}

func (q *qingCloudLoadBalanceExecutor) Resize(id string, newtype int) error {
	klog.V(2).Infof("Detect lb size changed, begin to resize the lb %s", id)
	err := q.Stop(id)
	if err != nil {
		klog.Errorf("Failed to stop lb %s when try to resize", id)
		return newServerErrorOfLoadBalancer(id, "Stop", err)
	}
	klog.V(2).Infof("Resizing the lb %s", id)
	output, err := q.lbapi.ResizeLoadBalancers(&qcservice.ResizeLoadBalancersInput{
		LoadBalancerType: &newtype,
		LoadBalancers:    []*string{&id},
	})
	if err != nil {
		return newServerErrorOfLoadBalancer(id, "Resize", err)
	}
	err = qcclient.WaitJob(q.jobapi, *output.JobID, operationWaitTimeout, waitInterval)
	if err != nil {
		klog.Errorf("Failed to waiting for lb resizing done")
		return newServerErrorOfLoadBalancer(id, "waitLoadBalancerResize", err)
	}
	return q.Start(id)
}

func (q *qingCloudLoadBalanceExecutor) Modify(input *qcservice.ModifyLoadBalancerAttributesInput) error {
	output, err := q.lbapi.ModifyLoadBalancerAttributes(input)
	if err != nil {
		return newServerErrorOfLoadBalancer(*input.LoadBalancerName, "Modify", err)
	}
	if *output.RetCode != 0 {
		return errors.NewCommonServerError(ResourceNameLoadBalancer, *input.LoadBalancerName, "Modify", *output.Message)
	}
	return nil
}

func (q *qingCloudLoadBalanceExecutor) AssociateEip(id string, eips ...string) error {
	if len(eips) == 0 {
		return nil
	}
	klog.V(2).Infof("Starting to associate Eip %s to loadBalancer '%s'", eips, id)
	output, err := q.lbapi.AssociateEIPsToLoadBalancer(&qcservice.AssociateEIPsToLoadBalancerInput{
		EIPs:         qcservice.StringSlice(eips),
		LoadBalancer: &id,
	})
	if err != nil {
		return newServerErrorOfLoadBalancer(id, "AssociateEip", err)
	}
	return qcclient.WaitJob(q.jobapi, *output.JobID, operationWaitTimeout, waitInterval)
}

func (q *qingCloudLoadBalanceExecutor) DissociateEip(id string, eips ...string) error {
	if len(eips) == 0 {
		return nil
	}
	klog.V(2).Infof("Starting to dissociate Eip %s to loadBalancer '%s'", eips, id)
	output, err := q.lbapi.DissociateEIPsFromLoadBalancer(&qcservice.DissociateEIPsFromLoadBalancerInput{
		EIPs:         qcservice.StringSlice(eips),
		LoadBalancer: &id,
	})
	if err != nil {
		return newServerErrorOfLoadBalancer(id, "DissociateEip", err)
	}
	return qcclient.WaitJob(q.jobapi, *output.JobID, operationWaitTimeout, waitInterval)
}

func (q *qingCloudLoadBalanceExecutor) waitLoadBalancerActive(id string) error {
	_, err := qcclient.WaitLoadBalancerStatus(q.lbapi, id, qcclient.LoadBalancerStatusActive, operationWaitTimeout, waitInterval)
	return err
}

func (q *qingCloudLoadBalanceExecutor) Confirm(id string) error {
	output, err := q.lbapi.UpdateLoadBalancers(&qcservice.UpdateLoadBalancersInput{
		LoadBalancers: []*string{&id},
	})
	if err != nil {
		return newServerErrorOfLoadBalancer(id, "Confirm", err)
	}
	klog.V(2).Infof("Waiting for updates of lb %s taking effects", id)
	err = qcclient.WaitJob(q.jobapi, *output.JobID, operationWaitTimeout, waitInterval)
	if err != nil {
		return newServerErrorOfLoadBalancer(id, "Wait Confirm Done", err)
	}
	return q.waitLoadBalancerActive(id)
}

func (q *qingCloudLoadBalanceExecutor) Delete(id string) error {
	output, err := q.lbapi.DeleteLoadBalancers(&qcservice.DeleteLoadBalancersInput{LoadBalancers: []*string{&id}})
	if err != nil {
		return newServerErrorOfLoadBalancer(id, "Delete", err)
	}
	err = qcclient.WaitJob(q.jobapi, *output.JobID, operationWaitTimeout, waitInterval)
	if err != nil {
		return newServerErrorOfLoadBalancer(id, "Wait Deletion Done", err)
	}
	return nil
}

func (q *qingCloudLoadBalanceExecutor) GetLBAPI() *qcservice.LoadBalancerService {
	return q.lbapi
}
