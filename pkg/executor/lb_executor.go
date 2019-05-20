package executor

import (
	"fmt"
	"strings"
	"time"

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
}

func NewQingCloudLoadBalanceExecutor(lbapi *qcservice.LoadBalancerService, jobapi *qcservice.JobService) QingCloudLoadBalancerExecutor {
	return &qingCloudLoadBalanceExecutor{
		lbapi:  lbapi,
		jobapi: jobapi,
	}
}
func (q *qingCloudLoadBalanceExecutor) GetLoadBalancerByName(name string) (*qcservice.LoadBalancer, error) {
	status := []*string{qcservice.String("pending"), qcservice.String("active"), qcservice.String("stopped")}
	output, err := q.lbapi.DescribeLoadBalancers(&qcservice.DescribeLoadBalancersInput{
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

func (q *qingCloudLoadBalanceExecutor) GetLoadBalancerByID(id string) (*qcservice.LoadBalancer, error) {
	output, err := q.lbapi.DescribeLoadBalancers(&qcservice.DescribeLoadBalancersInput{
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

func (q *qingCloudLoadBalanceExecutor) Start(id string) error {
	klog.V(2).Infof("Starting loadBalancer '%s'", id)
	output, err := q.lbapi.StartLoadBalancers(&qcservice.StartLoadBalancersInput{
		LoadBalancers: []*string{&id},
	})
	if err != nil {
		klog.Errorln("Failed to start loadbalancer")
		return err
	}
	return qcclient.WaitJob(q.jobapi, *output.JobID, operationWaitTimeout, waitInterval)
}

func (q *qingCloudLoadBalanceExecutor) Stop(id string) error {
	klog.V(2).Infof("Stopping loadBalancer '%s'", id)
	output, err := q.lbapi.StopLoadBalancers(&qcservice.StopLoadBalancersInput{
		LoadBalancers: []*string{&id},
	})
	if err != nil {
		klog.Errorln("Failed to start loadbalancer")
		return err
	}
	return qcclient.WaitJob(q.jobapi, *output.JobID, operationWaitTimeout, waitInterval)
}

func (q *qingCloudLoadBalanceExecutor) Create(input *qcservice.CreateLoadBalancerInput) (*qcservice.LoadBalancer, error) {
	output, err := q.lbapi.CreateLoadBalancer(input)
	if err != nil {
		return nil, err
	}
	klog.V(2).Infof("Waiting for Lb %s starting", *input.LoadBalancerName)
	err = q.waitLoadBalancerActive(*output.LoadBalancerID)
	if err != nil {
		klog.Errorf("LoadBalancer %s start failed", *output.LoadBalancerID)
		return nil, err
	}
	klog.V(2).Infof("Lb %s is successfully started", *input.LoadBalancerName)
	return q.GetLoadBalancerByID(*output.LoadBalancerID)
}

func (q *qingCloudLoadBalanceExecutor) Resize(id string, newtype int) error {
	klog.V(2).Infof("Detect lb size changed, begin to resize the lb %s", id)
	err := q.Stop(id)
	if err != nil {
		klog.Errorf("Failed to stop lb %s when try to resize", id)
	}
	klog.V(2).Infof("Resizing the lb %s", id)
	output, err := q.lbapi.ResizeLoadBalancers(&qcservice.ResizeLoadBalancersInput{
		LoadBalancerType: &newtype,
		LoadBalancers:    []*string{&id},
	})
	if err != nil {
		return err
	}
	err = qcclient.WaitJob(q.jobapi, *output.JobID, operationWaitTimeout, waitInterval)
	if err != nil {
		klog.Errorf("Failed to waiting for lb resizing done")
		return err
	}
	return q.Start(id)
}

func (q *qingCloudLoadBalanceExecutor) Modify(input *qcservice.ModifyLoadBalancerAttributesInput) error {
	output, err := q.lbapi.ModifyLoadBalancerAttributes(input)
	if err != nil {
		klog.Errorf("Couldn't update loadBalancer '%s'", *input.LoadBalancer)
		return err
	}
	if *output.RetCode != 0 {
		err := fmt.Errorf("Fail to update loadbalancer %s  because of '%s'", *input.LoadBalancer, *output.Message)
		return err
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
		klog.Errorf("Failed to add eip %s to lb %s", eips, id)
		return err
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
		klog.Errorf("Failed to add eip %s to lb %s", eips, id)
		return err
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
		klog.Errorf("Couldn't confirm updates on loadBalancer '%s'", id)
		return err
	}
	klog.V(2).Infof("Waiting for updates of lb %s taking effects", id)
	err = qcclient.WaitJob(q.jobapi, *output.JobID, operationWaitTimeout, waitInterval)
	if err != nil {
		return err
	}
	return q.waitLoadBalancerActive(id)
}

func (q *qingCloudLoadBalanceExecutor) Delete(id string) error {
	output, err := q.lbapi.DeleteLoadBalancers(&qcservice.DeleteLoadBalancersInput{LoadBalancers: []*string{&id}})
	if err != nil {
		klog.Errorf("Failed to start job to delete lb %s", id)
		return err
	}
	err = qcclient.WaitJob(q.jobapi, *output.JobID, operationWaitTimeout, waitInterval)
	if err != nil {
		klog.Errorf("Failed to excute deletion of lb %s", id)
		return err
	}
	return nil
}

func (q *qingCloudLoadBalanceExecutor) GetLBAPI() *qcservice.LoadBalancerService {
	return q.lbapi
}

func (q *qingCloudLoadBalanceExecutor) GetListenersOfLB(id, prefix string) ([]*qcservice.LoadBalancerListener, error) {
	resp, err := q.lbapi.DescribeLoadBalancerListeners(&qcservice.DescribeLoadBalancerListenersInput{
		LoadBalancer: &id,
		Verbose:      qcservice.Int(1),
		Limit:        qcservice.Int(pageLimt),
	})
	if err != nil {
		return nil, err
	}
	if prefix == "" {
		return resp.LoadBalancerListenerSet, nil
	}
	loadBalancerListeners := []*qcservice.LoadBalancerListener{}
	for _, l := range resp.LoadBalancerListenerSet {
		if strings.Contains(*l.LoadBalancerListenerName, prefix) {
			loadBalancerListeners = append(loadBalancerListeners, l)
		}
	}
	return loadBalancerListeners, nil
}

func (q *qingCloudLoadBalanceExecutor) DeleteListener(lsnid string) error {
	output, err := q.lbapi.DeleteLoadBalancerListeners(&qcservice.DeleteLoadBalancerListenersInput{
		LoadBalancerListeners: []*string{&lsnid},
	})
	if err != nil {
		return err
	}
	if *output.RetCode != 0 {
		err := fmt.Errorf("Fail to delete loadbalancer lisener '%s' because of '%s'", lsnid, *output.Message)
		return err
	}
	return nil
}

func (q *qingCloudLoadBalanceExecutor) CreateListener(input *qcservice.AddLoadBalancerListenersInput) (*qcservice.LoadBalancerListener, error) {
	output, err := q.lbapi.AddLoadBalancerListeners(input)
	if err != nil {
		klog.Errorf("Failed to create listeners %+v", input.Listeners)
		return nil, err
	}
	if *output.RetCode != 0 {
		err := fmt.Errorf("Fail to create liseners %s  because of '%s'", *input.LoadBalancer, *output.Message)
		return nil, err
	}
	return q.GetListenerByID(*output.LoadBalancerListeners[0])
}

func (q *qingCloudLoadBalanceExecutor) GetListenerByID(id string) (*qcservice.LoadBalancerListener, error) {
	output, err := q.lbapi.DescribeLoadBalancerListeners(&qcservice.DescribeLoadBalancerListenersInput{
		LoadBalancerListeners: []*string{&id},
	})
	if err != nil {
		return nil, err
	}
	if len(output.LoadBalancerListenerSet) == 0 {
		return nil, fmt.Errorf("Cannot find the specify listener which id is %s", id)
	}
	return output.LoadBalancerListenerSet[0], nil
}

func (q *qingCloudLoadBalanceExecutor) ModifyListener(id, balanceMode string) error {
	output, err := q.lbapi.ModifyLoadBalancerListenerAttributes(&qcservice.ModifyLoadBalancerListenerAttributesInput{
		LoadBalancerListener: &id,
		BalanceMode:          &balanceMode,
	})
	if err != nil {
		return err
	}
	if *output.RetCode != 0 {
		err := fmt.Errorf("Fail to modify balanceMode of loadbalancer lisener '%s' because of '%s'", id, *output.Message)
		return err
	}
	return nil
}

func (q *qingCloudLoadBalanceExecutor) DeleteBackends(ids ...string) error {
	if len(ids) == 0 {
		klog.Warningln("No backends to delete, pls check the inputs")
		return nil
	}
	output, err := q.lbapi.DeleteLoadBalancerBackends(&qcservice.DeleteLoadBalancerBackendsInput{
		LoadBalancerBackends: qcservice.StringSlice(ids),
	})
	if *output.RetCode != 0 {
		err := fmt.Errorf("Fail to delete backends of because of '%s'", *output.Message)
		return err
	}
	return err
}

func (q *qingCloudLoadBalanceExecutor) CreateBackends(input *qcservice.AddLoadBalancerBackendsInput) error {
	output, err := q.lbapi.AddLoadBalancerBackends(input)
	if err != nil {
		klog.Errorln("Failed to create backends")
		return err
	}
	if *output.RetCode != 0 {
		err := fmt.Errorf("Fail to create backends of listener %s because of '%s'", *input.LoadBalancerListener, *output.Message)
		return err
	}
	return nil
}

func (q *qingCloudLoadBalanceExecutor) ModifyBackend(id string, weight int, port int) error {
	input := &qcservice.ModifyLoadBalancerBackendAttributesInput{
		LoadBalancerBackend: &id,
		Port:                &port,
		Weight:              &weight,
	}
	output, err := q.lbapi.ModifyLoadBalancerBackendAttributes(input)
	if err != nil {
		klog.Errorf("Failed to modify attribute of backend %s", id)
		return err
	}
	if *output.RetCode != 0 {
		err := fmt.Errorf("Fail to modify attribute of backend '%s' because of '%s'", id, *output.Message)
		return err
	}
	return nil
}

func (q *qingCloudLoadBalanceExecutor) GetBackendsOfListener(id string) ([]*qcservice.LoadBalancerBackend, error) {
	input := &qcservice.DescribeLoadBalancerBackendsInput{
		LoadBalancerBackends: []*string{&id},
	}
	output, err := q.lbapi.DescribeLoadBalancerBackends(input)
	if err != nil {
		klog.Errorf("Failed to get Backends of listener %s from qingcloud", id)
		return nil, err
	}
	return output.LoadBalancerBackendSet, nil
}

func (q *qingCloudLoadBalanceExecutor) GetBackendByID(id string) (*qcservice.LoadBalancerBackend, error) {
	input := &qcservice.DescribeLoadBalancerBackendsInput{
		LoadBalancerBackends: []*string{&id},
	}
	output, err := q.lbapi.DescribeLoadBalancerBackends(input)
	if err != nil {
		klog.Errorf("Failed to get backend %s from qingcloud", id)
		return nil, err
	}
	if len(output.LoadBalancerBackendSet) < 1 {
		return nil, fmt.Errorf("Backend %s Not found", id)
	}
	return output.LoadBalancerBackendSet[0], nil
}
